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
			fmt.Fprintf(w, callbackErrorHTML("authorization failed", errorMsg))
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
			fmt.Fprintf(w, callbackSuccessHTML(result.Username))
		} else {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, callbackErrorHTML("token exchange failed", result.Error.Error()))
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


func callbackSuccessHTML(username string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Conectado - Galaxy Sync Service</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            min-height: 100vh;
            background: #030712;
            display: flex;
            align-items: center;
            justify-content: center;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            padding: 1rem;
        }
        .card {
            background: #111827;
            border-radius: 1rem;
            border: 1px solid #1f2937;
            max-width: 400px;
            width: 100%%;
            overflow: hidden;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }
        .header {
            background: linear-gradient(135deg, #7c3aed 0%%, #a855f7 50%%, #c084fc 100%%);
            padding: 2rem;
            text-align: center;
        }
        .icon {
            width: 64px;
            height: 64px;
            background: rgba(255,255,255,0.2);
            border-radius: 50%%;
            margin: 0 auto 1rem;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .icon svg { width: 32px; height: 32px; color: white; }
        .header h1 { color: white; font-size: 1.5rem; font-weight: 700; }
        .content { padding: 1.5rem; text-align: center; }
        .username {
            background: #1f2937;
            border-radius: 0.5rem;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .username-label { color: #9ca3af; font-size: 0.875rem; margin-bottom: 0.25rem; }
        .username-value { color: white; font-size: 1.125rem; font-weight: 600; }
        .message { color: #9ca3af; font-size: 0.875rem; }
        .close-hint {
            color: #6b7280;
            font-size: 0.75rem;
            margin-top: 1rem;
            padding-top: 1rem;
            border-top: 1px solid #1f2937;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="header">
            <div class="icon">
                <svg fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
            </div>
            <h1>¡Conectado!</h1>
        </div>
        <div class="content">
            <div class="username">
                <div class="username-label">Conectado como</div>
                <div class="username-value">%s</div>
            </div>
            <p class="message">Tu biblioteca de GOG Galaxy se sincronizará automáticamente.</p>
            <p class="close-hint">Puedes cerrar esta ventana</p>
        </div>
    </div>
</body>
</html>`, username)
}

func callbackErrorHTML(title, message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Error - Galaxy Sync Service</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            min-height: 100vh;
            background: #030712;
            display: flex;
            align-items: center;
            justify-content: center;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            padding: 1rem;
        }
        .card {
            background: #111827;
            border-radius: 1rem;
            border: 1px solid #1f2937;
            max-width: 400px;
            width: 100%%;
            overflow: hidden;
            box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
        }
        .header {
            background: linear-gradient(135deg, #e11d48 0%%, #f43f5e 100%%);
            padding: 2rem;
            text-align: center;
        }
        .icon {
            width: 64px;
            height: 64px;
            background: rgba(255,255,255,0.2);
            border-radius: 50%%;
            margin: 0 auto 1rem;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .icon svg { width: 32px; height: 32px; color: white; }
        .header h1 { color: white; font-size: 1.5rem; font-weight: 700; }
        .content { padding: 1.5rem; text-align: center; }
        .error-box {
            background: #1f2937;
            border-radius: 0.5rem;
            padding: 1rem;
            margin-bottom: 1rem;
            border-left: 3px solid #f43f5e;
        }
        .error-title { color: #f87171; font-size: 0.875rem; font-weight: 600; margin-bottom: 0.25rem; }
        .error-message { color: #9ca3af; font-size: 0.875rem; word-break: break-word; }
        .close-hint {
            color: #6b7280;
            font-size: 0.75rem;
            margin-top: 1rem;
            padding-top: 1rem;
            border-top: 1px solid #1f2937;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="header">
            <div class="icon">
                <svg fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
            </div>
            <h1>Error</h1>
        </div>
        <div class="content">
            <div class="error-box">
                <div class="error-title">%s</div>
                <div class="error-message">%s</div>
            </div>
            <p class="close-hint">Puedes cerrar esta ventana</p>
        </div>
    </div>
</body>
</html>`, title, message)
}
