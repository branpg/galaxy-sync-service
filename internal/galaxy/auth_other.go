//go:build !windows

package galaxy

import "fmt"

// getRefreshTokenFromRegistry is a stub for non-Windows platforms
func getRefreshTokenFromRegistry() (string, error) {
	return "", fmt.Errorf("refresh token reading from registry is only supported on Windows")
}
