//go:build windows

package galaxy

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// getRefreshTokenFromRegistry reads the GOG refresh token from Windows Registry
func getRefreshTokenFromRegistry() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\GOG.com\Galaxy`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	token, _, err := key.GetStringValue("refreshToken")
	if err != nil {
		return "", fmt.Errorf("failed to read refresh token: %w", err)
	}

	if token == "" {
		return "", fmt.Errorf("refresh token is empty - please log in to GOG Galaxy")
	}

	return token, nil
}
