package sync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/branpg/galaxy-sync-service/internal/config"
	"github.com/branpg/galaxy-sync-service/internal/galaxy"
)

const (
	BranpgAPIURL = "https://todo.branpg.net/api/gog/sync"
)

type SyncResult struct {
	Success       bool
	Message       string
	GamesCount    int
	WishlistCount int
	HasChanges    bool
	Error         error
}

func CalculateHash(library *galaxy.RawLibrary) string {
	data, _ := json.Marshal(library)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func Execute(cfg *config.Config, forceSync bool) SyncResult {
	// Read library from Galaxy database
	library, err := galaxy.ReadLibrary()
	if err != nil {
		return SyncResult{
			Success: false,
			Message: "Failed to read GOG Galaxy database",
			Error:   err,
		}
	}

	// Try to fetch wishlist from GOG API using Galaxy's refresh token
	wishlistErr := fetchWishlist(library)
	if wishlistErr != nil {
		// Wishlist fetch failed, but we can still sync owned games
		// Log the error but continue
		fmt.Printf("Warning: Could not fetch wishlist: %v\n", wishlistErr)
	}

	// Calculate hash to detect changes
	currentHash := CalculateHash(library)
	hasChanges := currentHash != cfg.LastSyncHash

	if !forceSync && !hasChanges {
		return SyncResult{
			Success:       true,
			Message:       "No changes detected, skipping sync",
			GamesCount:    len(library.OwnedIDs),
			WishlistCount: len(library.WishlistIDs),
			HasChanges:    false,
		}
	}

	// Determine endpoint
	var endpoint string
	var headers map[string]string

	if cfg.ServiceType == config.ServiceTypeBranpg {
		if cfg.BranpgAccessToken == "" {
			return SyncResult{
				Success: false,
				Message: "Not connected to Branpg TODO. Please configure first.",
			}
		}
		endpoint = BranpgAPIURL
		headers = map[string]string{
			"Authorization": "Bearer " + cfg.BranpgAccessToken,
		}
	} else {
		if cfg.ThirdPartyURL == "" {
			return SyncResult{
				Success: false,
				Message: "Third party URL not configured",
			}
		}
		endpoint = cfg.ThirdPartyURL
		headers = map[string]string{}
	}

	// Send data
	err = sendLibrary(endpoint, headers, library)
	if err != nil {
		return SyncResult{
			Success:       false,
			Message:       "Failed to sync",
			GamesCount:    len(library.OwnedIDs),
			WishlistCount: len(library.WishlistIDs),
			HasChanges:    hasChanges,
			Error:         err,
		}
	}

	// Update config with new hash and sync time
	cfg.LastSyncHash = currentHash
	cfg.LastSyncAt = time.Now()
	cfg.Save()

	message := fmt.Sprintf("Successfully synced %d games", len(library.OwnedIDs))
	if len(library.WishlistIDs) > 0 {
		message = fmt.Sprintf("Successfully synced %d games and %d wishlist items", len(library.OwnedIDs), len(library.WishlistIDs))
	}

	return SyncResult{
		Success:       true,
		Message:       message,
		GamesCount:    len(library.OwnedIDs),
		WishlistCount: len(library.WishlistIDs),
		HasChanges:    hasChanges,
	}
}

// fetchWishlist attempts to fetch the wishlist from GOG API
func fetchWishlist(library *galaxy.RawLibrary) error {
	// Get refresh token from Windows Registry
	refreshToken, err := galaxy.GetRefreshToken()
	if err != nil {
		return fmt.Errorf("could not get refresh token: %w", err)
	}

	// Exchange refresh token for access token
	tokenResp, err := galaxy.ExchangeRefreshToken(refreshToken)
	if err != nil {
		return fmt.Errorf("could not exchange refresh token: %w", err)
	}

	// Fetch wishlist from GOG API
	wishlistIDs, err := galaxy.FetchWishlist(tokenResp.AccessToken)
	if err != nil {
		return fmt.Errorf("could not fetch wishlist: %w", err)
	}

	library.WishlistIDs = wishlistIDs
	return nil
}

func sendLibrary(endpoint string, headers map[string]string, library *galaxy.RawLibrary) error {
	data, err := json.Marshal(library)
	if err != nil {
		return fmt.Errorf("failed to serialize library: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GalaxySyncService/1.0")

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func TestEndpoint(url string) error {
	// Send empty library to test connectivity
	library := &galaxy.RawLibrary{
		OwnedIDs:     []int64{},
		WishlistIDs:  []int64{},
		Playtime:     make(map[int64]int),
		Achievements: make(map[int64]galaxy.AchievementData),
		LastPlayed:   make(map[int64]string),
		SyncedAt:     time.Now().UTC(),
	}

	return sendLibrary(url, map[string]string{}, library)
}
