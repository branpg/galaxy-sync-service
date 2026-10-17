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
	// Query to get GOG games with their details
	// The schema may vary between Galaxy versions, so we handle errors gracefully
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
	
	rows, err := db.Query(query)
	if err != nil {
		// Try alternative query structure
		return readGamesAlternative(db)
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

func readGamesAlternative(db *sql.DB) ([]Game, error) {
	// Alternative query for different Galaxy DB versions
	query := `
		SELECT 
			releaseKey,
			title
		FROM InstalledProducts
		WHERE releaseKey LIKE 'gog_%'
		UNION
		SELECT 
			releaseKey,
			title  
		FROM LicensedProducts
		WHERE releaseKey LIKE 'gog_%'
	`
	
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query games: %w", err)
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
