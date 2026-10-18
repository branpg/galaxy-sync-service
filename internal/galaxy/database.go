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

// RawLibrary contains minimally processed data from GOG Galaxy database.
// Laravel handles all business logic (title lookup, status inference, etc.)
type RawLibrary struct {
	// OwnedIDs is the list of GOG game IDs the user owns
	OwnedIDs []int64 `json:"owned_ids"`

	// WishlistIDs is the list of GOG game IDs in the user's wishlist
	WishlistIDs []int64 `json:"wishlist_ids"`

	// Playtime maps game ID to minutes played
	Playtime map[int64]int `json:"playtime"`

	// Achievements maps game ID to achievement counts
	Achievements map[int64]AchievementData `json:"achievements"`

	// LastPlayed maps game ID to last played timestamp (RFC3339)
	LastPlayed map[int64]string `json:"last_played"`

	// SyncedAt is when this data was extracted
	SyncedAt time.Time `json:"synced_at"`
}

// AchievementData contains achievement counts for a game
type AchievementData struct {
	Unlocked int `json:"unlocked"`
	Total    int `json:"total"`
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

func ReadLibrary() (*RawLibrary, error) {
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

	library := &RawLibrary{
		OwnedIDs:     []int64{},
		WishlistIDs:  []int64{},
		Playtime:     make(map[int64]int),
		Achievements: make(map[int64]AchievementData),
		LastPlayed:   make(map[int64]string),
		SyncedAt:     time.Now().UTC(),
	}

	// Read owned game IDs from local database
	if err := readOwnedIDs(db, library); err != nil {
		return nil, err
	}

	if len(library.OwnedIDs) == 0 {
		tables := listTables(db)
		return nil, fmt.Errorf("no GOG games found in library. Available tables: %v", tables)
	}

	// Read playtime data
	readPlaytime(db, library)

	// Read achievement data
	readAchievements(db, library)

	// Read last played dates
	readLastPlayed(db, library)

	return library, nil
}

func readOwnedIDs(db *sql.DB, library *RawLibrary) error {
	query := `
		SELECT DISTINCT releaseKey
		FROM LibraryReleases
		WHERE releaseKey LIKE 'gog_%'
	`

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query owned games: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var releaseKey string
		if err := rows.Scan(&releaseKey); err != nil {
			continue
		}

		var id int64
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &id); err != nil {
			continue
		}

		library.OwnedIDs = append(library.OwnedIDs, id)
	}

	return nil
}

func readPlaytime(db *sql.DB, library *RawLibrary) {
	query := `
		SELECT releaseKey, minutesInGame
		FROM GameTimes
		WHERE releaseKey LIKE 'gog_%' AND minutesInGame > 0
	`

	rows, err := db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var releaseKey string
		var minutes int

		if err := rows.Scan(&releaseKey, &minutes); err != nil {
			continue
		}

		var id int64
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &id); err != nil {
			continue
		}

		library.Playtime[id] = minutes
	}
}

func readAchievements(db *sql.DB, library *RawLibrary) {
	query := `
		SELECT 
			gameReleaseKey,
			COUNT(*) as total,
			SUM(CASE WHEN isUnlocked = 1 THEN 1 ELSE 0 END) as unlocked
		FROM UserAchievements
		WHERE gameReleaseKey LIKE 'gog_%'
		GROUP BY gameReleaseKey
	`

	rows, err := db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var releaseKey string
		var total, unlocked sql.NullInt64

		if err := rows.Scan(&releaseKey, &total, &unlocked); err != nil {
			continue
		}

		var id int64
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &id); err != nil {
			continue
		}

		t, u := 0, 0
		if total.Valid {
			t = int(total.Int64)
		}
		if unlocked.Valid {
			u = int(unlocked.Int64)
		}

		if t > 0 {
			library.Achievements[id] = AchievementData{
				Unlocked: u,
				Total:    t,
			}
		}
	}
}

func readLastPlayed(db *sql.DB, library *RawLibrary) {
	query := `
		SELECT gameReleaseKey, lastPlayedDate
		FROM LastPlayedDates
		WHERE gameReleaseKey LIKE 'gog_%'
	`

	rows, err := db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var releaseKey string
		var lastPlayed string

		if err := rows.Scan(&releaseKey, &lastPlayed); err != nil {
			continue
		}

		var id int64
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &id); err != nil {
			continue
		}

		// Only include valid dates (not empty or sentinel values)
		if lastPlayed != "" && lastPlayed != "0001-01-01 00:00:00" {
			library.LastPlayed[id] = lastPlayed
		}
	}
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
