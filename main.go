package main

import (
	"os"

	"github.com/branpg/galaxy-sync-service/internal/app"
	"github.com/branpg/galaxy-sync-service/internal/service"
)

func main() {
	// Check if running as Windows service
	if len(os.Args) > 1 && os.Args[1] == "--service" {
		service.Run()
		return
	}

	// Run GUI application
	app.Run()
}
