package galaxy

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"
)

type Game struct {
	ID                   int64     `json:"id"`
	Title                string    `json:"title"`
	AchievementsTotal    int       `json:"achievements_total"`
	AchievementsUnlocked int       `json:"achievements_unlocked"`
	PlaytimeMinutes      int       `json:"playtime_minutes"`
	LastPlayed           time.Time `json:"last_played,omitempty"`
}

type Library struct {
	Games    []Game    `json:"games"`
	SyncedAt time.Time `json:"synced_at"`
}

func GetDatabasePath() (string, error) {
	var basePath string
	
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("ProgramData")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		basePath = filepath.Join(programData, "GOG.com", "Galaxy", "storage", "galaxy-2.0.db")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		basePath = filepath.Join(home, "Library", "Application Support", "GOG.com", "Galaxy", "storage", "galaxy-2.0.db")
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return "", fmt.Errorf("GOG Galaxy database not found at: %s", basePath)
	}
	
	return basePath, nil
}

func ReadLibrary() (*Library, error) {
	dbPath, err := GetDatabasePath()
	if err != nil {
		return nil, err
	}
	
	// Open as read-only to avoid locking issues with Galaxy
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()
	
	games, err := readGames(db)
	if err != nil {
		return nil, err
	}
	
	return &Library{
		Games:    games,
		SyncedAt: time.Now().UTC(),
	}, nil
}

func readGames(db *sql.DB) ([]Game, error) {
	// Try multiple query strategies based on what tables exist
	// GOG Galaxy schema varies between versions
	
	// Strategy 1: LibraryReleases table (newer Galaxy versions)
	games, err := readGamesFromLibraryReleases(db)
	if err == nil && len(games) > 0 {
		return games, nil
	}
	
	// Strategy 2: Products table with GameTimes
	games, err = readGamesFromProducts(db)
	if err == nil && len(games) > 0 {
		return games, nil
	}
	
	// Strategy 3: InstalledProducts/LicensedProducts union
	games, err = readGamesFromInstalledProducts(db)
	if err == nil && len(games) > 0 {
		return games, nil
	}
	
	// Strategy 4: Try to discover available tables and report
	tables := listTables(db)
	if len(tables) == 0 {
		return nil, fmt.Errorf("failed to query games: database appears empty or inaccessible")
	}
	
	return nil, fmt.Errorf("failed to query games: no compatible table found. Available tables: %v", tables)
}

func listTables(db *sql.DB) []string {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()
	
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}
	return tables
}

func readGamesFromLibraryReleases(db *sql.DB) ([]Game, error) {
	query := `
		SELECT DISTINCT
			lr.releaseKey,
			COALESCE(pd.title, lr.releaseKey) as title,
			COALESCE(gt.minutesInGame, 0) as playtime,
			gt.lastSessionEnd
		FROM LibraryReleases lr
		LEFT JOIN ProductsDetails pd ON pd.releaseKey = lr.releaseKey
		LEFT JOIN GameTimes gt ON gt.releaseKey = lr.releaseKey
		WHERE lr.releaseKey LIKE 'gog_%'
		ORDER BY title
	`
	
	return queryGamesWithPlaytime(db, query)
}

func readGamesFromProducts(db *sql.DB) ([]Game, error) {
	query := `
		SELECT 
			p.releaseKey,
			p.title,
			COALESCE(gt.minutesInGame, 0) as playtime,
			gt.lastSessionEnd
		FROM Products p
		LEFT JOIN GameTimes gt ON gt.releaseKey = p.releaseKey
		WHERE p.releaseKey LIKE 'gog_%'
		ORDER BY p.title
	`
	
	return queryGamesWithPlaytime(db, query)
}

func queryGamesWithPlaytime(db *sql.DB, query string) ([]Game, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var games []Game
	for rows.Next() {
		var g Game
		var releaseKey string
		var lastPlayed sql.NullInt64
		
		if err := rows.Scan(&releaseKey, &g.Title, &g.PlaytimeMinutes, &lastPlayed); err != nil {
			continue
		}
		
		// Extract numeric ID from releaseKey (e.g., "gog_1234567890" -> 1234567890)
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &g.ID); err != nil {
			continue
		}
		
		if lastPlayed.Valid {
			g.LastPlayed = time.Unix(lastPlayed.Int64, 0).UTC()
		}
		
		// Get achievements for this game
		achievements, unlocked := readAchievements(db, releaseKey)
		g.AchievementsTotal = achievements
		g.AchievementsUnlocked = unlocked
		
		games = append(games, g)
	}
	
	return games, nil
}

func readGamesFromInstalledProducts(db *sql.DB) ([]Game, error) {
	query := `
		SELECT releaseKey, title FROM InstalledProducts WHERE releaseKey LIKE 'gog_%'
		UNION
		SELECT releaseKey, title FROM LicensedProducts WHERE releaseKey LIKE 'gog_%'
	`
	
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var games []Game
	seen := make(map[int64]bool)
	
	for rows.Next() {
		var g Game
		var releaseKey string
		
		if err := rows.Scan(&releaseKey, &g.Title); err != nil {
			continue
		}
		
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &g.ID); err != nil {
			continue
		}
		
		if seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		
		achievements, unlocked := readAchievements(db, releaseKey)
		g.AchievementsTotal = achievements
		g.AchievementsUnlocked = unlocked
		
		games = append(games, g)
	}
	
	return games, nil
}

func readAchievements(db *sql.DB, releaseKey string) (total int, unlocked int) {
	// Query achievements table
	query := `
		SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN unlocked = 1 THEN 1 ELSE 0 END) as unlocked
		FROM Achievements
		WHERE releaseKey = ?
	`
	
	var t, u sql.NullInt64
	err := db.QueryRow(query, releaseKey).Scan(&t, &u)
	if err != nil {
		return 0, 0
	}
	
	if t.Valid {
		total = int(t.Int64)
	}
	if u.Valid {
		unlocked = int(u.Int64)
	}
	
	return total, unlocked
}
