package galaxy

import (
	"database/sql"
	"encoding/json"
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

// titleJSON is used to parse the JSON title from GamePieces
type titleJSON struct {
	Title string `json:"title"`
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
	// GOG Galaxy 2.0 schema:
	// - LibraryReleases: list of owned games (releaseKey)
	// - GamePieces: metadata including title (gamePieceTypeId=189 is title as JSON)
	// - GameTimes: playtime per game
	// - UserAchievements: achievement unlock status
	query := `
		SELECT DISTINCT
			lr.releaseKey,
			gp.value as titleJson,
			COALESCE(gt.minutesInGame, 0) as playtime
		FROM LibraryReleases lr
		LEFT JOIN GamePieces gp ON gp.releaseKey = lr.releaseKey AND gp.gamePieceTypeId = 189
		LEFT JOIN GameTimes gt ON gt.releaseKey = lr.releaseKey
		WHERE lr.releaseKey LIKE 'gog_%'
		ORDER BY gp.value
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query games: %w", err)
	}
	defer rows.Close()

	var games []Game
	for rows.Next() {
		var g Game
		var releaseKey string
		var titleJsonStr sql.NullString

		if err := rows.Scan(&releaseKey, &titleJsonStr, &g.PlaytimeMinutes); err != nil {
			continue
		}

		// Extract numeric ID from releaseKey (e.g., "gog_1234567890" -> 1234567890)
		if _, err := fmt.Sscanf(releaseKey, "gog_%d", &g.ID); err != nil {
			continue
		}

		// Parse title from JSON
		if titleJsonStr.Valid && titleJsonStr.String != "" {
			var tj titleJSON
			if err := json.Unmarshal([]byte(titleJsonStr.String), &tj); err == nil {
				g.Title = tj.Title
			}
		}
		if g.Title == "" {
			g.Title = releaseKey // Fallback to releaseKey if no title
		}

		// Get achievements for this game
		g.AchievementsTotal, g.AchievementsUnlocked = readAchievements(db, releaseKey)

		games = append(games, g)
	}

	if len(games) == 0 {
		// If no games found, list available tables for debugging
		tables := listTables(db)
		return nil, fmt.Errorf("no GOG games found in library. Available tables: %v", tables)
	}

	return games, nil
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

func readAchievements(db *sql.DB, releaseKey string) (total int, unlocked int) {
	// UserAchievements table has achievement unlock status
	// gameReleaseKey matches our releaseKey
	query := `
		SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN isUnlocked = 1 THEN 1 ELSE 0 END) as unlocked
		FROM UserAchievements
		WHERE gameReleaseKey = ?
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
