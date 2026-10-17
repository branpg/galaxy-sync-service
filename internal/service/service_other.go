//go:build !windows

package service

import "fmt"

func Run() {
	fmt.Println("Windows service is not supported on this platform")
}

func Install(intervalMinutes int) error {
	return fmt.Errorf("windows service is not supported on this platform")
}

func Uninstall() error {
	return fmt.Errorf("windows service is not supported on this platform")
}

func IsInstalled() bool {
	return false
}

func IsRunning() bool {
	return false
}

func RequiresAdmin() bool {
	return false
}
