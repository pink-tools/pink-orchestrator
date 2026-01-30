package tray

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/getlantern/systray"
	"github.com/pink-tools/pink-otel"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
	"github.com/pink-tools/pink-orchestrator/internal/services"
)

type serviceMenu struct {
	name       string
	isDaemon   bool
	menuItem   *systray.MenuItem
	mStatus    *systray.MenuItem
	mError     *systray.MenuItem
	mUpdate    *systray.MenuItem
	mInstall   *systray.MenuItem
	mCheck     *systray.MenuItem
	mStart     *systray.MenuItem
	mStop      *systray.MenuItem
	mRestart   *systray.MenuItem
	mEnv       *systray.MenuItem
	mUninstall *systray.MenuItem
}

type Tray struct {
	serviceMenus []*serviceMenu
}

func New() *Tray {
	return &Tray{}
}

func (t *Tray) Run() {
	systray.Run(t.onReady, t.onExit)
}

func (t *Tray) onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("")
	systray.SetTooltip("Pink Orchestrator")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		systray.Quit()
	}()

	services.SetStatusCallback(t.updateMenus)

	t.buildMenu()
	services.RestoreState()
	t.updateMenus()
}

func (t *Tray) onExit() {
	otel.Info(context.Background(), "shutting down")
	services.SaveState()
	services.Shutdown()
	otel.Info(context.Background(), "stopped")
	os.Exit(0)
}

func (t *Tray) buildMenu() {
	svcs, err := registry.ListServices()
	if err != nil {
		mError := systray.AddMenuItem("Failed to load registry", "")
		mError.Disable()
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Quit", "")
		go func() {
			<-mQuit.ClickedCh
			systray.Quit()
		}()
		return
	}

	for _, svc := range svcs {
		if svc.Type == "daemon" {
			sm := t.addServiceMenu(svc.Name)
			t.serviceMenus = append(t.serviceMenus, sm)
		}
	}

	systray.AddSeparator()

	for _, svc := range svcs {
		if svc.Type != "daemon" {
			sm := t.addServiceMenu(svc.Name)
			t.serviceMenus = append(t.serviceMenus, sm)
		}
	}

	systray.AddSeparator()

	mStartAll := systray.AddMenuItem("Start All", "")
	mStopAll := systray.AddMenuItem("Stop All", "")

	go func() {
		for range mStartAll.ClickedCh {
			go t.startAllServices()
		}
	}()

	go func() {
		for range mStopAll.ClickedCh {
			go t.stopAllServices()
		}
	}()

	systray.AddSeparator()

	mUpdateAll := systray.AddMenuItem("Update All Services", "")
	mUpdateOrch := systray.AddMenuItem("Update Orchestrator", "")

	go func() {
		for range mUpdateAll.ClickedCh {
			go t.updateAllServices()
		}
	}()

	go func() {
		for range mUpdateOrch.ClickedCh {
			go t.updateOrchestrator()
		}
	}()

	mQuit := systray.AddMenuItem("Quit", "")
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

func (t *Tray) updateMenus() {
	for _, sm := range t.serviceMenus {
		t.updateServiceMenu(sm)
	}
}

func (t *Tray) updateServiceMenu(sm *serviceMenu) {
	status := services.GetStatus(sm.name)
	installing := services.IsInstalling(sm.name)

	hasError := services.GetLastError(sm.name) != ""

	var title string
	switch {
	case installing:
		title = fmt.Sprintf("⏳ %s", sm.name)
	case status.Status == services.StatusNotInstalled:
		title = fmt.Sprintf("⚠ %s", sm.name)
	case hasError:
		title = fmt.Sprintf("✕ %s", sm.name)
	case !sm.isDaemon:
		title = fmt.Sprintf("✓ %s", sm.name)
	case status.Status == services.StatusStopped:
		title = fmt.Sprintf("○ %s", sm.name)
	case status.Status == services.StatusRunning:
		title = fmt.Sprintf("● %s", sm.name)
	default:
		title = fmt.Sprintf("? %s", sm.name)
	}
	sm.menuItem.SetTitle(title)

	lastStatus := services.GetLastStatus(sm.name)
	if lastStatus == "" {
		lastStatus = "-"
	}
	sm.mStatus.SetTitle(fmt.Sprintf("Status: %s", truncate(lastStatus, 50)))

	lastError := services.GetLastError(sm.name)
	if lastError != "" {
		sm.mError.SetTitle(fmt.Sprintf("Error: %s", truncate(lastError, 50)))
		sm.mError.Show()
	} else {
		sm.mError.Hide()
	}

	if installing {
		sm.mUpdate.Hide()
		sm.mInstall.Show()
		sm.mInstall.Disable()
		sm.mCheck.Hide()
		sm.mStart.Hide()
		sm.mStop.Hide()
		sm.mRestart.Hide()
		sm.mEnv.Hide()
		sm.mUninstall.Hide()
	} else if status.Status == services.StatusNotInstalled {
		sm.mUpdate.Hide()
		sm.mInstall.Show()
		sm.mInstall.Enable()
		sm.mCheck.Hide()
		sm.mStart.Hide()
		sm.mStop.Hide()
		sm.mRestart.Hide()
		sm.mEnv.Hide()
		sm.mUninstall.Hide()
	} else if sm.isDaemon {
		sm.mCheck.Hide()
		sm.mInstall.Hide()
		sm.mUpdate.Show()
		sm.mEnv.Show()
		sm.mUninstall.Show()
		if status.Status == services.StatusRunning {
			sm.mStart.Hide()
			sm.mStop.Show()
			sm.mRestart.Show()
		} else {
			sm.mStart.Show()
			sm.mStop.Hide()
			sm.mRestart.Hide()
		}
	} else {
		sm.mInstall.Hide()
		sm.mCheck.Show()
		sm.mUpdate.Show()
		sm.mStart.Hide()
		sm.mStop.Hide()
		sm.mRestart.Hide()
		sm.mEnv.Show()
		sm.mUninstall.Show()
	}
}

func (t *Tray) addServiceMenu(name string) *serviceMenu {
	sm := &serviceMenu{name: name, isDaemon: registry.IsDaemon(name)}

	sm.menuItem = systray.AddMenuItem(name, "")

	sm.mInstall = sm.menuItem.AddSubMenuItem("Install", "")
	sm.mCheck = sm.menuItem.AddSubMenuItem("Check", "")
	sm.mStart = sm.menuItem.AddSubMenuItem("Start", "")
	sm.mStop = sm.menuItem.AddSubMenuItem("Stop", "")
	sm.mRestart = sm.menuItem.AddSubMenuItem("Restart", "")
	sm.mEnv = sm.menuItem.AddSubMenuItem("Edit .env", "")

	sm.menuItem.AddSubMenuItem("───────────", "").Disable()
	sm.mUpdate = sm.menuItem.AddSubMenuItem("Update", "")
	sm.mUninstall = sm.menuItem.AddSubMenuItem("Uninstall", "")

	sm.menuItem.AddSubMenuItem("───────────", "").Disable()
	sm.mStatus = sm.menuItem.AddSubMenuItem("Status: -", "")
	sm.mStatus.Disable()
	sm.mError = sm.menuItem.AddSubMenuItem("Error: -", "")
	sm.mError.Disable()

	go func() {
		for range sm.mUpdate.ClickedCh {
			go func() {
				otel.Info(context.Background(), "updating", otel.Attr{"service", name})
				services.Update(name, func(msg string) {
					otel.Info(context.Background(), msg, otel.Attr{"service", name})
					services.SetLastStatus(name, msg)
				})
			}()
		}
	}()

	go func() {
		for range sm.mInstall.ClickedCh {
			go func() {
				otel.Info(context.Background(), "installing", otel.Attr{"service", name})
				services.Install(name, func(msg string) {
					otel.Info(context.Background(), msg, otel.Attr{"service", name})
					services.SetLastStatus(name, msg)
				})
			}()
		}
	}()

	go func() {
		for range sm.mStart.ClickedCh {
			services.Start(name)
			t.updateMenus()
		}
	}()

	go func() {
		for range sm.mStop.ClickedCh {
			services.Stop(name)
			t.updateMenus()
		}
	}()

	go func() {
		for range sm.mRestart.ClickedCh {
			services.Restart(name)
			t.updateMenus()
		}
	}()

	go func() {
		for range sm.mEnv.ClickedCh {
			services.OpenEnvFile(name)
		}
	}()

	go func() {
		for range sm.mCheck.ClickedCh {
			version, err := services.Check(name)
			if err != nil {
				otel.Warn(context.Background(), "check failed", otel.Attr{"service", name}, otel.Attr{"error", err.Error()})
			} else {
				otel.Info(context.Background(), "version", otel.Attr{"service", name}, otel.Attr{"version", version})
				services.SetLastStatus(name, version)
			}
		}
	}()

	go func() {
		for range sm.mUninstall.ClickedCh {
			services.Uninstall(name)
			t.updateMenus()
		}
	}()

	return sm
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func (t *Tray) startAllServices() {
	otel.Info(context.Background(), "starting all services")

	for _, sm := range t.serviceMenus {
		if !sm.isDaemon {
			continue
		}
		status := services.GetStatus(sm.name)
		if status.Status == services.StatusNotInstalled || status.Status == services.StatusRunning {
			continue
		}
		otel.Info(context.Background(), "starting", otel.Attr{"service", sm.name})
		services.Start(sm.name)
	}
	t.updateMenus()
}

func (t *Tray) stopAllServices() {
	otel.Info(context.Background(), "stopping all services")

	for _, sm := range t.serviceMenus {
		if !sm.isDaemon {
			continue
		}
		status := services.GetStatus(sm.name)
		if status.Status != services.StatusRunning {
			continue
		}
		services.Stop(sm.name)
	}
	t.updateMenus()
}

func (t *Tray) updateAllServices() {
	otel.Info(context.Background(), "updating all services")

	svcs, err := registry.ListServices()
	if err != nil {
		otel.Error(context.Background(), "failed to list services", otel.Attr{"error", err.Error()})
		return
	}

	var updated, failed, skipped int
	for _, svc := range svcs {
		if !services.IsInstalled(svc.Name) {
			skipped++
			continue
		}

		otel.Info(context.Background(), "checking", otel.Attr{"service", svc.Name})
		services.SetLastStatus(svc.Name, "Checking for updates...")

		err := services.Update(svc.Name, func(msg string) {
			otel.Info(context.Background(), msg, otel.Attr{"service", svc.Name})
			services.SetLastStatus(svc.Name, msg)
		})

		if err != nil {
			otel.Error(context.Background(), "update failed", otel.Attr{"service", svc.Name}, otel.Attr{"error", err.Error()})
			failed++
		} else {
			updated++
		}
	}

	otel.Info(context.Background(), "update all complete", otel.Attr{"updated", updated}, otel.Attr{"failed", failed}, otel.Attr{"skipped", skipped})
}

func (t *Tray) updateOrchestrator() {
	otel.Info(context.Background(), "checking for orchestrator updates")

	hasUpdate, _, latest, err := services.CheckOrchestratorUpdate()
	if err != nil {
		otel.Error(context.Background(), "failed to check for updates", otel.Attr{"error", err.Error()})
		return
	}

	if !hasUpdate {
		otel.Info(context.Background(), "orchestrator is up to date")
		return
	}

	otel.Info(context.Background(), "updating orchestrator", otel.Attr{"version", latest})

	if err := services.SelfUpdate(latest, func(msg string) {
		otel.Info(context.Background(), msg)
	}); err != nil {
		otel.Error(context.Background(), "self-update failed", otel.Attr{"error", err.Error()})
		return
	}

	systray.Quit()
}
