package galaxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

// GOG Galaxy public OAuth credentials (same used by the official client)
// These are intentionally public - GOG uses them for the Galaxy client
const (
	GogClientID     = "46899977096215655"
	GogClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
	GogTokenURL     = "https://auth.gog.com/token"
)

// TokenResponse represents the OAuth token response from GOG
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	UserID       string `json:"user_id"`
	SessionID    string `json:"session_id"`
}

// GetRefreshToken reads the GOG refresh token from the Windows Registry
// GOG Galaxy stores it at: HKEY_CURRENT_USER\SOFTWARE\GOG.com\Galaxy\refreshToken
func GetRefreshToken() (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("refresh token reading is only supported on Windows")
	}

	return getRefreshTokenFromRegistry()
}

// ExchangeRefreshToken exchanges a refresh token for an access token
func ExchangeRefreshToken(refreshToken string) (*TokenResponse, error) {
	params := url.Values{}
	params.Set("client_id", GogClientID)
	params.Set("client_secret", GogClientSecret)
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)

	resp, err := http.Get(GogTokenURL + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("failed to exchange refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var token TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if token.AccessToken == "" {
		return nil, fmt.Errorf("received empty access token")
	}

	return &token, nil
}

// WishlistResponse represents the wishlist API response
type WishlistResponse struct {
	Wishlist map[string]bool `json:"wishlist"`
	Checksum string          `json:"checksum"`
}

// FetchWishlist fetches the user's wishlist from GOG API
func FetchWishlist(accessToken string) ([]int64, error) {
	req, err := http.NewRequest("GET", "https://embed.gog.com/user/wishlist.json", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create wishlist request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "GalaxySyncService/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wishlist request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wishlist request returned status %d", resp.StatusCode)
	}

	var wishlistResp WishlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&wishlistResp); err != nil {
		return nil, fmt.Errorf("failed to decode wishlist response: %w", err)
	}

	var wishlistIDs []int64
	for idStr := range wishlistResp.Wishlist {
		var id int64
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			wishlistIDs = append(wishlistIDs, id)
		}
	}

	return wishlistIDs, nil
}
