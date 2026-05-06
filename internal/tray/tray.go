package tray

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/pink-tools/pink-orchestrator/internal/systray"
	"github.com/pink-tools/pink-core"
	"github.com/pink-tools/pink-core/log"
	"github.com/pink-tools/pink-orchestrator/internal/dialog"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
	"github.com/pink-tools/pink-orchestrator/internal/services"
)

type actionItem struct {
	name     string // action name (e.g. "settings")
	menuItem *systray.MenuItem
}

type serviceMenu struct {
	name       string
	isDaemon   bool
	hasSetup   bool
	menuItem   *systray.MenuItem
	mStatus    *systray.MenuItem
	mError     *systray.MenuItem
	mUpdate    *systray.MenuItem
	mDownload  *systray.MenuItem
	mSetup     *systray.MenuItem
	mCheck     *systray.MenuItem
	mStart     *systray.MenuItem
	mStop      *systray.MenuItem
	mRestart   *systray.MenuItem
	mEnv       *systray.MenuItem
	mRemove    *systray.MenuItem
	actions    []actionItem
}

type Tray struct {
	serviceMenus []*serviceMenu
}

func New() *Tray {
	return &Tray{}
}

func (t *Tray) Run() {
	systray.Run(t.onReady, t.onExit)

	if services.PendingRestart() && runtime.GOOS == "windows" {
		log.Info(context.Background(), "restarting after update")
		services.ReleaseLock()
		os.Exit(0)
	}
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
	log.Info(context.Background(), "shutting down")
	services.SaveState()
	services.Shutdown()
	log.Info(context.Background(), "stopped")
}

func (t *Tray) buildMenu() {
	svcs, err := registry.ListServices()
	if err != nil {
		log.Error(context.Background(), "registry load failed in buildMenu", log.Attr{K: "error", V: err.Error()})
		mError := systray.AddMenuItem(fmt.Sprintf("Registry: %v", err), "")
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
	downloading := services.IsDownloading(sm.name)

	hasError := services.GetLastError(sm.name) != ""

	var title string
	switch {
	case downloading:
		title = fmt.Sprintf("⏳ %s", sm.name)
	case status.Status == services.StatusNotDownloaded:
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

	showActions := !downloading && status.Status != services.StatusNotDownloaded
	for _, a := range sm.actions {
		if showActions {
			a.menuItem.Show()
		} else {
			a.menuItem.Hide()
		}
	}

	if downloading {
		sm.mUpdate.Hide()
		sm.mDownload.Show()
		sm.mDownload.Disable()
		sm.mSetup.Hide()
		sm.mCheck.Hide()
		sm.mStart.Hide()
		sm.mStop.Hide()
		sm.mRestart.Hide()
		sm.mEnv.Hide()
		sm.mRemove.Hide()
	} else if status.Status == services.StatusNotDownloaded {
		sm.mUpdate.Hide()
		sm.mDownload.Show()
		sm.mDownload.Enable()
		sm.mSetup.Hide()
		sm.mCheck.Hide()
		sm.mStart.Hide()
		sm.mStop.Hide()
		sm.mRestart.Hide()
		sm.mEnv.Hide()
		sm.mRemove.Hide()
	} else if sm.isDaemon {
		sm.mCheck.Hide()
		sm.mDownload.Hide()
		sm.mUpdate.Show()
		sm.mEnv.Show()
		sm.mRemove.Show()
		if sm.hasSetup && services.NeedsSetup(sm.name) {
			sm.mSetup.Show()
		} else {
			sm.mSetup.Hide()
		}
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
		sm.mDownload.Hide()
		sm.mSetup.Hide()
		sm.mCheck.Show()
		sm.mUpdate.Show()
		sm.mStart.Hide()
		sm.mStop.Hide()
		sm.mRestart.Hide()
		sm.mEnv.Show()
		sm.mRemove.Show()
	}
}

func (t *Tray) addServiceMenu(name string) *serviceMenu {
	sm := &serviceMenu{name: name, isDaemon: registry.IsDaemon(name), hasSetup: services.HasSetup(name)}
	sm.menuItem = systray.AddMenuItem(name, "")
	t.populateServiceMenu(sm)
	return sm
}

func (t *Tray) rebuildServiceMenu(sm *serviceMenu) {
	sm.menuItem.ResetSubmenu()
	sm.actions = nil
	t.populateServiceMenu(sm)
	t.updateServiceMenu(sm)
}

func (t *Tray) populateServiceMenu(sm *serviceMenu) {
	name := sm.name

	sm.mDownload = sm.menuItem.AddSubMenuItem("Download", "")
	sm.mSetup = sm.menuItem.AddSubMenuItem("Setup", "")
	sm.mCheck = sm.menuItem.AddSubMenuItem("Check", "")
	sm.mStart = sm.menuItem.AddSubMenuItem("Start", "")
	sm.mStop = sm.menuItem.AddSubMenuItem("Stop", "")
	sm.mRestart = sm.menuItem.AddSubMenuItem("Restart", "")
	sm.mEnv = sm.menuItem.AddSubMenuItem("Edit .env", "")

	if services.IsDownloaded(name) {
		for _, a := range services.GetActions(name) {
			item := sm.menuItem.AddSubMenuItem(a.Label, "")
			sm.actions = append(sm.actions, actionItem{name: a.Name, menuItem: item})
			go func(actionName string) {
				for range item.ClickedCh {
					go t.handleAction(name, actionName)
				}
			}(a.Name)
		}
	}

	sm.menuItem.AddSubMenuItem("───────────", "").Disable()
	sm.mUpdate = sm.menuItem.AddSubMenuItem("Update", "")
	sm.mRemove = sm.menuItem.AddSubMenuItem("Remove", "")

	sm.menuItem.AddSubMenuItem("───────────", "").Disable()
	sm.mStatus = sm.menuItem.AddSubMenuItem("Status: -", "")
	sm.mStatus.Disable()
	sm.mError = sm.menuItem.AddSubMenuItem("Error: -", "")
	sm.mError.Disable()

	go func() {
		for range sm.mUpdate.ClickedCh {
			go func() {
				log.Info(context.Background(), "updating", log.Attr{K: "service", V: name})
				if err := services.Update(name, func(msg string) {
					log.Info(context.Background(), msg, log.Attr{K: "service", V: name})
					services.SetLastStatus(name, msg)
				}); err != nil {
					log.Error(context.Background(), "update failed", log.Attr{K: "service", V: name}, log.Attr{K: "error", V: err.Error()})
					services.SetLastError(name, err.Error())
				}
				t.rebuildServiceMenu(sm)
			}()
		}
	}()

	go func() {
		for range sm.mDownload.ClickedCh {
			go func() {
				log.Info(context.Background(), "downloading", log.Attr{K: "service", V: name})
				if err := services.DownloadWithSetup(name, func(msg string) {
					log.Info(context.Background(), msg, log.Attr{K: "service", V: name})
					services.SetLastStatus(name, msg)
				}, func(specJSON []byte) (map[string]any, bool) {
					return dialog.ShowForm(specJSON)
				}); err != nil {
					log.Error(context.Background(), "download failed", log.Attr{K: "service", V: name}, log.Attr{K: "error", V: err.Error()})
					services.SetLastError(name, err.Error())
				}
				t.rebuildServiceMenu(sm)
			}()
		}
	}()

	go func() {
		for range sm.mSetup.ClickedCh {
			go func() {
				log.Info(context.Background(), "running setup", log.Attr{K: "service", V: name})
				services.SetLastStatus(name, "Running setup...")
				if err := services.RunSetupTerminal(name); err != nil {
					log.Error(context.Background(), "setup failed", log.Attr{K: "service", V: name}, log.Attr{K: "error", V: err.Error()})
					services.SetLastStatus(name, fmt.Sprintf("Setup failed: %s", err))
				}
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
				log.Warn(context.Background(), "check failed", log.Attr{K: "service", V: name}, log.Attr{K: "error", V: err.Error()})
			} else {
				log.Info(context.Background(), "version", log.Attr{K: "service", V: name}, log.Attr{K: "version", V: version})
				services.SetLastStatus(name, version)
			}
		}
	}()

	go func() {
		for range sm.mRemove.ClickedCh {
			services.Remove(name)
			t.rebuildServiceMenu(sm)
		}
	}()
}

func (t *Tray) handleAction(serviceName, actionName string) {
	specJSON, err := services.DescribeAction(serviceName, actionName)
	if err != nil {
		log.Error(context.Background(), "describe action failed", log.Attr{K: "service", V: serviceName}, log.Attr{K: "action", V: actionName}, log.Attr{K: "error", V: err.Error()})
		services.SetLastStatus(serviceName, fmt.Sprintf("Failed: %s", err))
		return
	}

	values, ok := dialog.ShowForm(specJSON)
	if !ok {
		return
	}

	if err := services.ExecuteAction(serviceName, actionName, values); err != nil {
		log.Error(context.Background(), "execute action failed", log.Attr{K: "service", V: serviceName}, log.Attr{K: "action", V: actionName}, log.Attr{K: "error", V: err.Error()})
		services.SetLastStatus(serviceName, fmt.Sprintf("Failed: %s", err))
		return
	}

	// Tell running service to reload config via IPC
	status := services.GetStatus(serviceName)
	if status.Status == services.StatusRunning {
		resp, err := core.SendCommand(serviceName, "RELOAD")
		if err != nil || resp != "OK" {
			// Service doesn't support RELOAD — restart as fallback
			log.Info(context.Background(), "reload not supported, restarting", log.Attr{K: "service", V: serviceName})
			services.Restart(serviceName)
		}
		services.SetLastStatus(serviceName, "Settings applied")
	} else {
		services.SetLastStatus(serviceName, "Settings saved")
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func (t *Tray) startAllServices() {
	log.Info(context.Background(), "starting all services")

	for _, sm := range t.serviceMenus {
		if !sm.isDaemon {
			continue
		}
		status := services.GetStatus(sm.name)
		if status.Status == services.StatusNotDownloaded || status.Status == services.StatusRunning {
			continue
		}
		log.Info(context.Background(), "starting", log.Attr{K: "service", V: sm.name})
		services.Start(sm.name)
	}
	t.updateMenus()
}

func (t *Tray) stopAllServices() {
	log.Info(context.Background(), "stopping all services")

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
	log.Info(context.Background(), "updating all services")

	svcs, err := registry.ListServices()
	if err != nil {
		log.Error(context.Background(), "failed to list services", log.Attr{K: "error", V: err.Error()})
		return
	}

	var updated, failed, skipped int
	for _, svc := range svcs {
		if !services.IsDownloaded(svc.Name) {
			skipped++
			continue
		}

		log.Info(context.Background(), "checking", log.Attr{K: "service", V: svc.Name})
		services.SetLastStatus(svc.Name, "Checking for updates...")

		err := services.Update(svc.Name, func(msg string) {
			log.Info(context.Background(), msg, log.Attr{K: "service", V: svc.Name})
			services.SetLastStatus(svc.Name, msg)
		})

		if err != nil {
			log.Error(context.Background(), "update failed", log.Attr{K: "service", V: svc.Name}, log.Attr{K: "error", V: err.Error()})
			failed++
		} else {
			updated++
		}
	}

	log.Info(context.Background(), "update all complete", log.Attr{K: "updated", V: updated}, log.Attr{K: "failed", V: failed}, log.Attr{K: "skipped", V: skipped})
}

func (t *Tray) updateOrchestrator() {
	log.Info(context.Background(), "checking for orchestrator updates")

	hasUpdate, _, latest, err := services.CheckOrchestratorUpdate()
	if err != nil {
		log.Error(context.Background(), "failed to check for updates", log.Attr{K: "error", V: err.Error()})
		return
	}

	if !hasUpdate {
		log.Info(context.Background(), "orchestrator is up to date")
		return
	}

	log.Info(context.Background(), "updating orchestrator", log.Attr{K: "version", V: latest})

	if err := services.SelfUpdate(latest, func(msg string) {
		log.Info(context.Background(), msg)
	}); err != nil {
		log.Error(context.Background(), "self-update failed", log.Attr{K: "error", V: err.Error()})
		return
	}

	if services.PendingRestart() && runtime.GOOS == "windows" {
		log.Info(context.Background(), "restarting after update")
		services.SaveState()
		services.Shutdown()
		services.ReleaseLock()
		os.Exit(0)
	}
}
