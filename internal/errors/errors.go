package errors

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UserError represents an error with a user-friendly message and technical details
type UserError struct {
	UserMessage string // Short message for the user
	Technical   string // Full technical error for logs
	Timestamp   time.Time
}

func (e *UserError) Error() string {
	return e.UserMessage
}

// New creates a UserError from a technical error
func New(userMessage string, technicalErr error) *UserError {
	technical := ""
	if technicalErr != nil {
		technical = technicalErr.Error()
	}
	return &UserError{
		UserMessage: userMessage,
		Technical:   technical,
		Timestamp:   time.Now(),
	}
}

// Wrap wraps an error with a user-friendly message
func Wrap(err error) *UserError {
	if err == nil {
		return nil
	}

	// Check if it's already a UserError
	if ue, ok := err.(*UserError); ok {
		return ue
	}

	errStr := err.Error()

	// Map common errors to user-friendly messages
	userMsg := categorizeError(errStr)

	return &UserError{
		UserMessage: userMsg,
		Technical:   errStr,
		Timestamp:   time.Now(),
	}
}

func categorizeError(errStr string) string {
	errLower := strings.ToLower(errStr)

	// Database errors
	if strings.Contains(errLower, "database") || strings.Contains(errLower, "sqlite") {
		if strings.Contains(errLower, "not found") {
			return "GOG Galaxy database not found. Make sure GOG Galaxy is installed."
		}
		if strings.Contains(errLower, "locked") || strings.Contains(errLower, "i/o error") {
			return "Cannot read database. Try closing GOG Galaxy and retry."
		}
		return "Error reading GOG Galaxy database."
	}

	// Network errors
	if strings.Contains(errLower, "connection refused") {
		return "Cannot connect to server. Check your internet connection."
	}
	if strings.Contains(errLower, "timeout") {
		return "Connection timed out. The server may be busy."
	}
	if strings.Contains(errLower, "no such host") || strings.Contains(errLower, "dns") {
		return "Cannot reach server. Check your internet connection."
	}

	// Auth errors
	if strings.Contains(errLower, "401") || strings.Contains(errLower, "unauthorized") {
		return "Authentication failed. Please reconnect your account."
	}
	if strings.Contains(errLower, "403") || strings.Contains(errLower, "forbidden") {
		return "Access denied. Please reconnect your account."
	}
	if strings.Contains(errLower, "token") && strings.Contains(errLower, "expired") {
		return "Session expired. Please reconnect your account."
	}

	// Server errors
	if strings.Contains(errLower, "500") || strings.Contains(errLower, "internal server") {
		return "Server error. Please try again later."
	}
	if strings.Contains(errLower, "502") || strings.Contains(errLower, "503") || strings.Contains(errLower, "504") {
		return "Server temporarily unavailable. Please try again later."
	}

	// File errors
	if strings.Contains(errLower, "permission denied") {
		return "Permission denied. Try running as administrator."
	}
	if strings.Contains(errLower, "file not found") || strings.Contains(errLower, "no such file") {
		return "Required file not found."
	}

	// Default
	return "An unexpected error occurred."
}

// SaveLog saves the full error log to a file and returns the path
func (e *UserError) SaveLog() (string, error) {
	// Get user's temp directory
	tempDir := os.TempDir()
	filename := fmt.Sprintf("galaxy-sync-error-%s.log", e.Timestamp.Format("2006-01-02-150405"))
	logPath := filepath.Join(tempDir, filename)

	content := fmt.Sprintf(`Galaxy Sync Service - Error Log
================================
Timestamp: %s
User Message: %s

Technical Details:
%s
`,
		e.Timestamp.Format("2006-01-02 15:04:05"),
		e.UserMessage,
		e.Technical,
	)

	err := os.WriteFile(logPath, []byte(content), 0644)
	if err != nil {
		return "", err
	}

	return logPath, nil
}

// FormatForDisplay returns a short message suitable for UI display
func (e *UserError) FormatForDisplay() string {
	return e.UserMessage
}

// HasTechnicalDetails returns true if there are technical details available
func (e *UserError) HasTechnicalDetails() bool {
	return e.Technical != "" && e.Technical != e.UserMessage
}
