package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/branpg/galaxy-sync-service/internal/config"
)

const (
	BranpgBaseURL    = "https://todo.branpg.net"
	OAuthClientID    = "galaxy-sync-service"
	OAuthRedirectURI = "http://localhost:19847/callback"
)

type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type UserResponse struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

type OAuthResult struct {
	Success   bool
	Username  string
	Error     error
}

// StartOAuthFlow initiates the OAuth flow and returns the authorization URL
// It starts a local server to receive the callback
func StartOAuthFlow(cfg *config.Config, onComplete func(OAuthResult)) (string, error) {
	// Build authorization URL
	authURL := fmt.Sprintf("%s/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=sync",
		BranpgBaseURL,
		url.QueryEscape(OAuthClientID),
		url.QueryEscape(OAuthRedirectURI),
	)
	
	// Start local server to receive callback
	go startCallbackServer(cfg, onComplete)
	
	return authURL, nil
}

func startCallbackServer(cfg *config.Config, onComplete func(OAuthResult)) {
	listener, err := net.Listen("tcp", ":19847")
	if err != nil {
		onComplete(OAuthResult{Success: false, Error: err})
		return
	}
	
	server := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errorMsg := r.URL.Query().Get("error")
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><body><h1>Error</h1><p>%s</p><p>You can close this window.</p></body></html>", errorMsg)
			onComplete(OAuthResult{Success: false, Error: fmt.Errorf("authorization failed: %s", errorMsg)})
			go func() {
				time.Sleep(1 * time.Second)
				server.Shutdown(context.Background())
			}()
			return
		}
		
		// Exchange code for token
		result := exchangeCodeForToken(cfg, code)
		
		if result.Success {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><body><h1>Success!</h1><p>Connected as %s</p><p>You can close this window.</p></body></html>", result.Username)
		} else {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><body><h1>Error</h1><p>%v</p><p>You can close this window.</p></body></html>", result.Error)
		}
		
		onComplete(result)
		
		go func() {
			time.Sleep(1 * time.Second)
			server.Shutdown(context.Background())
		}()
	})
	
	server.Serve(listener)
}

func exchangeCodeForToken(cfg *config.Config, code string) OAuthResult {
	// Exchange authorization code for access token
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", OAuthClientID)
	data.Set("redirect_uri", OAuthRedirectURI)
	data.Set("code", code)
	
	req, err := http.NewRequest("POST", BranpgBaseURL+"/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return OAuthResult{Success: false, Error: err}
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return OAuthResult{Success: false, Error: err}
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OAuthResult{Success: false, Error: err}
	}
	
	if resp.StatusCode != 200 {
		return OAuthResult{Success: false, Error: fmt.Errorf("token exchange failed: %s", string(body))}
	}
	
	var tokenResp OAuthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return OAuthResult{Success: false, Error: err}
	}
	
	// Get user info
	username, err := getUserInfo(tokenResp.AccessToken)
	if err != nil {
		return OAuthResult{Success: false, Error: err}
	}
	
	// Save to config
	cfg.BranpgAccessToken = tokenResp.AccessToken
	cfg.BranpgRefreshToken = tokenResp.RefreshToken
	cfg.BranpgTokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	cfg.BranpgUsername = username
	cfg.BranpgConnectedAt = time.Now()
	cfg.Save()
	
	return OAuthResult{Success: true, Username: username}
}

func getUserInfo(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", BranpgBaseURL+"/api/user", nil)
	if err != nil {
		return "", err
	}
	
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to get user info: status %d", resp.StatusCode)
	}
	
	var user UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}
	
	return user.Name, nil
}

func RefreshToken(cfg *config.Config) error {
	if cfg.BranpgRefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}
	
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", OAuthClientID)
	data.Set("refresh_token", cfg.BranpgRefreshToken)
	
	req, err := http.NewRequest("POST", BranpgBaseURL+"/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token refresh failed: %s", string(body))
	}
	
	var tokenResp OAuthTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}
	
	cfg.BranpgAccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		cfg.BranpgRefreshToken = tokenResp.RefreshToken
	}
	cfg.BranpgTokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	cfg.Save()
	
	return nil
}
