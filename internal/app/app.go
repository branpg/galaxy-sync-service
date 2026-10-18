package app

import (
	"fmt"
	"os/exec"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/branpg/galaxy-sync-service/internal/config"
	apperrors "github.com/branpg/galaxy-sync-service/internal/errors"
	"github.com/branpg/galaxy-sync-service/internal/galaxy"
	"github.com/branpg/galaxy-sync-service/internal/service"
	"github.com/branpg/galaxy-sync-service/internal/sync"
)

func Run() {
	a := app.New()
	w := a.NewWindow("Galaxy Sync Service")
	w.Resize(fyne.NewSize(450, 450))

	cfg, _ := config.Load()

	// Status label
	statusLabel := widget.NewLabel("Ready")
	statusLabel.Wrapping = fyne.TextWrapWord

	// Create main menu buttons
	syncBtn := widget.NewButton("Sync Now", nil)
	debugBtn := widget.NewButton("Debug Database", nil)
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
				var userErr *apperrors.UserError
				if result.Error != nil {
					userErr = apperrors.Wrap(result.Error)
				} else {
					userErr = apperrors.New(result.Message, nil)
				}
				statusLabel.SetText(userErr.FormatForDisplay())
				
				if userErr.HasTechnicalDetails() {
					showErrorDialog(w, userErr)
				}
			}
			syncBtn.Enable()
		}()
	}

	// Debug button action
	debugBtn.OnTapped = func() {
		debugBtn.Disable()
		statusLabel.SetText("Reading database...")

		go func() {
			showDebugWindow(a, w, statusLabel)
			debugBtn.Enable()
		}()
	}

	// Install button action
	installBtn.OnTapped = func() {
		if service.RequiresAdmin() {
			showErrorDialog(w, apperrors.New("Administrator privileges required", fmt.Errorf("Please run as administrator to install the service")))
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
					showErrorDialog(w, apperrors.Wrap(err))
				} else {
					dialog.ShowInformation("Success", "Service installed successfully", w)
				}
				updateButtons()
			}, w)
	}

	// Uninstall button action
	uninstallBtn.OnTapped = func() {
		if service.RequiresAdmin() {
			showErrorDialog(w, apperrors.New("Administrator privileges required", fmt.Errorf("Please run as administrator to uninstall the service")))
			return
		}

		dialog.ShowConfirm("Uninstall Service", "Are you sure you want to uninstall the service?",
			func(confirmed bool) {
				if !confirmed {
					return
				}

				err := service.Uninstall()
				if err != nil {
					showErrorDialog(w, apperrors.Wrap(err))
				} else {
					dialog.ShowInformation("Success", "Service uninstalled successfully", w)
				}
				updateButtons()
			}, w)
	}

	// Configure button action
	configBtn.OnTapped = func() {
		showConfigWindow(a, w, cfg, statusLabel)
	}

	// Initial button state
	updateButtons()

	// Layout
	buttons := container.NewVBox(
		syncBtn,
		debugBtn,
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

// showErrorDialog shows a user-friendly error with option to save full log
func showErrorDialog(parent fyne.Window, err *apperrors.UserError) {
	if err == nil {
		return
	}

	// Create dialog content
	msgLabel := widget.NewLabel(err.FormatForDisplay())
	msgLabel.Wrapping = fyne.TextWrapWord

	var content fyne.CanvasObject

	if err.HasTechnicalDetails() {
		saveLogBtn := widget.NewButton("Save Error Log", func() {
			logPath, saveErr := err.SaveLog()
			if saveErr != nil {
				dialog.ShowError(fmt.Errorf("Could not save log: %v", saveErr), parent)
				return
			}
			dialog.ShowInformation("Log Saved", fmt.Sprintf("Error log saved to:\n%s", logPath), parent)
			
			// Open the folder containing the log
			openFileLocation(logPath)
		})

		content = container.NewVBox(
			msgLabel,
			widget.NewSeparator(),
			container.NewHBox(layout.NewSpacer(), saveLogBtn, layout.NewSpacer()),
		)
	} else {
		content = msgLabel
	}

	d := dialog.NewCustom("Error", "Close", content, parent)
	d.Resize(fyne.NewSize(400, 200))
	d.Show()
}

func showDebugWindow(a fyne.App, parent fyne.Window, mainStatus *widget.Label) {
	w := a.NewWindow("Debug - Database Contents")
	w.Resize(fyne.NewSize(800, 600))

	// Read library
	library, err := galaxy.ReadLibrary()

	var content fyne.CanvasObject

	if err != nil {
		userErr := apperrors.Wrap(err)
		mainStatus.SetText(userErr.FormatForDisplay())
		
		// Create error view with save log option
		msgLabel := widget.NewLabelWithStyle("Error reading database", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		errorLabel := widget.NewLabel(userErr.FormatForDisplay())
		errorLabel.Wrapping = fyne.TextWrapWord

		saveLogBtn := widget.NewButton("Save Error Log", func() {
			logPath, saveErr := userErr.SaveLog()
			if saveErr != nil {
				dialog.ShowError(fmt.Errorf("Could not save log: %v", saveErr), w)
				return
			}
			dialog.ShowInformation("Log Saved", fmt.Sprintf("Error log saved to:\n%s", logPath), w)
			openFileLocation(logPath)
		})

		content = container.NewVBox(
			msgLabel,
			errorLabel,
			widget.NewSeparator(),
			container.NewHBox(layout.NewSpacer(), saveLogBtn, layout.NewSpacer()),
		)
	} else {
		mainStatus.SetText(fmt.Sprintf("Found %d games", len(library.Games)))

		// Create table header
		header := container.NewGridWithColumns(5,
			widget.NewLabelWithStyle("ID", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Title", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Achievements", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Playtime", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabelWithStyle("Last Played", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		)

		// Create list of games
		gameList := widget.NewList(
			func() int {
				return len(library.Games)
			},
			func() fyne.CanvasObject {
				return container.NewGridWithColumns(5,
					widget.NewLabel("ID"),
					widget.NewLabel("Title"),
					widget.NewLabel("Ach"),
					widget.NewLabel("Time"),
					widget.NewLabel("Date"),
				)
			},
			func(i widget.ListItemID, o fyne.CanvasObject) {
				game := library.Games[i]
				grid := o.(*fyne.Container)

				grid.Objects[0].(*widget.Label).SetText(fmt.Sprintf("%d", game.ID))
				grid.Objects[1].(*widget.Label).SetText(truncateString(game.Title, 40))
				grid.Objects[2].(*widget.Label).SetText(fmt.Sprintf("%d/%d", game.AchievementsUnlocked, game.AchievementsTotal))

				hours := game.PlaytimeMinutes / 60
				mins := game.PlaytimeMinutes % 60
				grid.Objects[3].(*widget.Label).SetText(fmt.Sprintf("%dh %dm", hours, mins))

				if !game.LastPlayed.IsZero() {
					grid.Objects[4].(*widget.Label).SetText(game.LastPlayed.Format("2006-01-02"))
				} else {
					grid.Objects[4].(*widget.Label).SetText("-")
				}
			},
		)

		// Database path info
		dbPath, _ := galaxy.GetDatabasePath()
		pathLabel := widget.NewLabel(fmt.Sprintf("Database: %s", dbPath))
		pathLabel.Wrapping = fyne.TextWrapWord

		// Summary
		totalAchievements := 0
		unlockedAchievements := 0
		totalPlaytime := 0
		for _, g := range library.Games {
			totalAchievements += g.AchievementsTotal
			unlockedAchievements += g.AchievementsUnlocked
			totalPlaytime += g.PlaytimeMinutes
		}

		summaryLabel := widget.NewLabel(fmt.Sprintf(
			"Total: %d games | Achievements: %d/%d | Playtime: %d hours",
			len(library.Games),
			unlockedAchievements,
			totalAchievements,
			totalPlaytime/60,
		))

		content = container.NewBorder(
			container.NewVBox(
				pathLabel,
				summaryLabel,
				widget.NewSeparator(),
				header,
			),
			nil, nil, nil,
			gameList,
		)
	}

	closeBtn := widget.NewButton("Close", func() {
		w.Close()
	})

	w.SetContent(container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), closeBtn),
		nil, nil,
		content,
	))

	w.Show()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func showConfigWindow(a fyne.App, parent fyne.Window, cfg *config.Config, mainStatus *widget.Label) {
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
				userErr := apperrors.Wrap(result.Error)
				showErrorDialog(w, userErr)
			}
		})

		if err != nil {
			showErrorDialog(w, apperrors.Wrap(err))
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
			showErrorDialog(w, apperrors.New("Please enter a URL", nil))
			return
		}

		err := sync.TestEndpoint(url)
		if err != nil {
			showErrorDialog(w, apperrors.Wrap(err))
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
			showErrorDialog(w, apperrors.Wrap(err))
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

func openFileLocation(path string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}

	cmd.Start()
}
