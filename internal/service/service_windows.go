//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/branpg/galaxy-sync-service/internal/config"
	"github.com/branpg/galaxy-sync-service/internal/sync"
)

const serviceName = "GalaxySyncService"
const serviceDescription = "Syncs GOG Galaxy library with external services"

type galaxySyncService struct{}

func (m *galaxySyncService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	
	cfg, _ := config.Load()
	ticker := time.NewTicker(time.Duration(cfg.SyncInterval) * time.Minute)
	defer ticker.Stop()
	
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	
	// Initial sync
	sync.Execute(cfg, false)
	
loop:
	for {
		select {
		case <-ticker.C:
			cfg, _ = config.Load() // Reload config in case it changed
			sync.Execute(cfg, false)
			
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				break loop
			default:
			}
		}
	}
	
	changes <- svc.Status{State: svc.StopPending}
	return
}

func Run() {
	err := svc.Run(serviceName, &galaxySyncService{})
	if err != nil {
		// Log error
	}
}

func Install(intervalMinutes int) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()
	
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}
	
	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName: "Galaxy Sync Service",
		Description: serviceDescription,
		StartType:   mgr.StartAutomatic,
	}, "--service")
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()
	
	// Save interval to config
	cfg, _ := config.Load()
	cfg.SyncInterval = intervalMinutes
	cfg.Save()
	
	// Start the service
	err = s.Start()
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}
	
	return nil
}

func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()
	
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found: %w", serviceName, err)
	}
	defer s.Close()
	
	// Stop the service first
	s.Control(svc.Stop)
	time.Sleep(2 * time.Second)
	
	err = s.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	
	return nil
}

func IsInstalled() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	s.Close()
	return true
}

func IsRunning() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()
	
	s, err := m.OpenService(serviceName)
	if err != nil {
		return false
	}
	defer s.Close()
	
	status, err := s.Query()
	if err != nil {
		return false
	}
	
	return status.State == svc.Running
}

func RequiresAdmin() bool {
	// Check if we can connect to service manager with write access
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err != nil
}
