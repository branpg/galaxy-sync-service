package app

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/branpg/galaxy-sync-service/internal/config"
	"github.com/branpg/galaxy-sync-service/internal/service"
	"github.com/branpg/galaxy-sync-service/internal/sync"
)

func Run() {
	a := app.New()
	w := a.NewWindow("Galaxy Sync Service")
	w.Resize(fyne.NewSize(450, 400))
	
	cfg, _ := config.Load()
	
	// Status label
	statusLabel := widget.NewLabel("Ready")
	statusLabel.Wrapping = fyne.TextWrapWord
	
	// Create main menu buttons
	syncBtn := widget.NewButton("Sync Now", nil)
	installBtn := widget.NewButton("Install Service", nil)
	uninstallBtn := widget.NewButton("Uninstall Service", nil)
	configBtn := widget.NewButton("Configure", nil)
	
	// Update button states based on service status
	updateButtons := func() {
		if service.IsInstalled() {
			installBtn.Disable()
			uninstallBtn.Enable()
			if service.IsRunning() {
				statusLabel.SetText("Service is running")
			} else {
				statusLabel.SetText("Service is installed but not running")
			}
		} else {
			installBtn.Enable()
			uninstallBtn.Disable()
			statusLabel.SetText("Service not installed")
		}
	}
	
	// Sync button action
	syncBtn.OnTapped = func() {
		syncBtn.Disable()
		statusLabel.SetText("Syncing...")
		
		go func() {
			result := sync.Execute(cfg, true) // Force sync
			
			if result.Success {
				statusLabel.SetText(result.Message)
			} else {
				msg := result.Message
				if result.Error != nil {
					msg = fmt.Sprintf("%s: %v", msg, result.Error)
				}
				statusLabel.SetText(msg)
			}
			syncBtn.Enable()
		}()
	}
	
	// Install button action
	installBtn.OnTapped = func() {
		if service.RequiresAdmin() {
			dialog.ShowError(fmt.Errorf("Administrator privileges required.\nPlease run as administrator."), w)
			return
		}
		
		// Ask for sync interval
		intervalEntry := widget.NewEntry()
		intervalEntry.SetText("10")
		
		dialog.ShowForm("Install Service", "Install", "Cancel",
			[]*widget.FormItem{
				widget.NewFormItem("Sync interval (minutes)", intervalEntry),
			},
			func(confirmed bool) {
				if !confirmed {
					return
				}
				
				var interval int
				fmt.Sscanf(intervalEntry.Text, "%d", &interval)
				if interval < 1 {
					interval = 10
				}
				
				err := service.Install(interval)
				if err != nil {
					dialog.ShowError(err, w)
				} else {
					dialog.ShowInformation("Success", "Service installed successfully", w)
				}
				updateButtons()
			}, w)
	}
	
	// Uninstall button action
	uninstallBtn.OnTapped = func() {
		if service.RequiresAdmin() {
			dialog.ShowError(fmt.Errorf("Administrator privileges required.\nPlease run as administrator."), w)
			return
		}
		
		dialog.ShowConfirm("Uninstall Service", "Are you sure you want to uninstall the service?",
			func(confirmed bool) {
				if !confirmed {
					return
				}
				
				err := service.Uninstall()
				if err != nil {
					dialog.ShowError(err, w)
				} else {
					dialog.ShowInformation("Success", "Service uninstalled successfully", w)
				}
				updateButtons()
			}, w)
	}
	
	// Configure button action
	configBtn.OnTapped = func() {
		showConfigWindow(a, cfg, statusLabel)
	}
	
	// Initial button state
	updateButtons()
	
	// Layout
	buttons := container.NewVBox(
		syncBtn,
		widget.NewSeparator(),
		installBtn,
		uninstallBtn,
		widget.NewSeparator(),
		configBtn,
	)
	
	content := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Galaxy Sync Service", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
		),
		container.NewVBox(
			widget.NewSeparator(),
			statusLabel,
		),
		nil, nil,
		container.NewCenter(buttons),
	)
	
	w.SetContent(content)
	w.ShowAndRun()
}

func showConfigWindow(a fyne.App, cfg *config.Config, mainStatus *widget.Label) {
	w := a.NewWindow("Configuration")
	w.Resize(fyne.NewSize(500, 350))
	
	// Service type selector
	serviceType := widget.NewRadioGroup([]string{"Branpg TODO", "Third-party service"}, nil)
	if cfg.ServiceType == config.ServiceTypeThirdParty {
		serviceType.SetSelected("Third-party service")
	} else {
		serviceType.SetSelected("Branpg TODO")
	}
	
	// Branpg TODO section
	branpgStatus := widget.NewLabel("")
	connectBtn := widget.NewButton("Connect", nil)
	disconnectBtn := widget.NewButton("Disconnect", nil)
	
	updateBranpgUI := func() {
		if cfg.BranpgAccessToken != "" {
			branpgStatus.SetText(fmt.Sprintf("Connected as %s since %s",
				cfg.BranpgUsername,
				cfg.BranpgConnectedAt.Format("2006-01-02 15:04")))
			connectBtn.Hide()
			disconnectBtn.Show()
		} else {
			branpgStatus.SetText("Not connected")
			connectBtn.Show()
			disconnectBtn.Hide()
		}
	}
	
	connectBtn.OnTapped = func() {
		authURL, err := sync.StartOAuthFlow(cfg, func(result sync.OAuthResult) {
			if result.Success {
				updateBranpgUI()
				mainStatus.SetText(fmt.Sprintf("Connected as %s", result.Username))
			} else {
				dialog.ShowError(result.Error, w)
			}
		})
		
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		
		// Open browser
		openBrowser(authURL)
		branpgStatus.SetText("Waiting for authorization...")
	}
	
	disconnectBtn.OnTapped = func() {
		dialog.ShowConfirm("Disconnect", "Are you sure you want to disconnect?",
			func(confirmed bool) {
				if confirmed {
					cfg.Disconnect()
					cfg.Save()
					updateBranpgUI()
					mainStatus.SetText("Disconnected")
				}
			}, w)
	}
	
	branpgContainer := container.NewVBox(
		branpgStatus,
		container.NewHBox(connectBtn, disconnectBtn),
	)
	
	// Third-party section
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/api/gog/sync")
	urlEntry.SetText(cfg.ThirdPartyURL)
	
	testBtn := widget.NewButton("Test Connection", func() {
		url := urlEntry.Text
		if url == "" {
			dialog.ShowError(fmt.Errorf("Please enter a URL"), w)
			return
		}
		
		err := sync.TestEndpoint(url)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Connection failed: %v", err), w)
		} else {
			dialog.ShowInformation("Success", "Connection successful!", w)
		}
	})
	
	thirdPartyContainer := container.NewVBox(
		widget.NewLabel("API URL:"),
		urlEntry,
		testBtn,
	)
	
	// Show/hide sections based on selection
	branpgCard := widget.NewCard("Branpg TODO", "", branpgContainer)
	thirdPartyCard := widget.NewCard("Third-party Service", "", thirdPartyContainer)
	
	updateVisibility := func() {
		if serviceType.Selected == "Branpg TODO" {
			branpgCard.Show()
			thirdPartyCard.Hide()
		} else {
			branpgCard.Hide()
			thirdPartyCard.Show()
		}
	}
	
	serviceType.OnChanged = func(s string) {
		updateVisibility()
	}
	
	// Initial UI state
	updateBranpgUI()
	updateVisibility()
	
	// Save button
	saveBtn := widget.NewButton("Save", func() {
		if serviceType.Selected == "Branpg TODO" {
			cfg.ServiceType = config.ServiceTypeBranpg
		} else {
			cfg.ServiceType = config.ServiceTypeThirdParty
			cfg.ThirdPartyURL = urlEntry.Text
		}
		
		err := cfg.Save()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		
		mainStatus.SetText("Configuration saved")
		w.Close()
	})
	
	cancelBtn := widget.NewButton("Cancel", func() {
		w.Close()
	})
	
	content := container.NewVBox(
		widget.NewLabel("Sync with:"),
		serviceType,
		widget.NewSeparator(),
		branpgCard,
		thirdPartyCard,
		layout.NewSpacer(),
		container.NewHBox(layout.NewSpacer(), cancelBtn, saveBtn),
	)
	
	w.SetContent(container.NewPadded(content))
	w.Show()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	
	cmd.Start()
}
