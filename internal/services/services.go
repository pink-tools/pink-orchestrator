package services

import (
	"os"
	"sync"

	"github.com/pink-tools/pink-orchestrator/internal/config"
)

type Status string

const (
	StatusNotDownloaded Status = "not_downloaded"
	StatusStopped      Status = "stopped"
	StatusRunning      Status = "running"
	StatusError        Status = "error"
)

type ServiceState struct {
	Status     Status `json:"status"`
	LastStatus string `json:"last_status"`
	LastError  string `json:"last_error"`
	PID        int    `json:"pid,omitempty"`
}

type processInfo struct {
	process *os.Process
	done    chan struct{}
}

var (
	mu                 sync.RWMutex
	serviceLogs        = make(map[string]*ServiceState)
	runningProcesses   = make(map[string]*processInfo)
	downloadingServices = make(map[string]bool)
	onStatusUpdate     func()
)

func SetStatusCallback(cb func()) {
	mu.Lock()
	onStatusUpdate = cb
	mu.Unlock()
}

func notifyStatusUpdate() {
	mu.RLock()
	cb := onStatusUpdate
	mu.RUnlock()
	if cb != nil {
		cb()
	}
}

func GetStatus(name string) ServiceState {
	state := ServiceState{Status: StatusNotDownloaded}

	binary := config.ServiceBinary(name)
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		return state
	}

	state.Status = StatusStopped

	mu.RLock()
	info, ok := runningProcesses[name]
	if ok {
		select {
		case <-info.done:
			// Process exited
		default:
			// Process still running
			state.Status = StatusRunning
			state.PID = info.process.Pid
		}
	}
	mu.RUnlock()

	return state
}

func IsDownloaded(name string) bool {
	binary := config.ServiceBinary(name)
	_, err := os.Stat(binary)
	return err == nil
}

func IsDownloading(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return downloadingServices[name]
}

func updateServiceLog(name, line string, isError bool) {
	mu.Lock()
	if serviceLogs[name] == nil {
		serviceLogs[name] = &ServiceState{}
	}

	if isError {
		serviceLogs[name].LastError = line
	} else {
		serviceLogs[name].LastStatus = line
	}
	mu.Unlock()

	notifyStatusUpdate()
}

func SetLastStatus(name, status string) {
	mu.Lock()
	if serviceLogs[name] == nil {
		serviceLogs[name] = &ServiceState{}
	}
	serviceLogs[name].LastStatus = status
	mu.Unlock()

	notifyStatusUpdate()
}

func GetLastStatus(name string) string {
	mu.RLock()
	defer mu.RUnlock()
	if state := serviceLogs[name]; state != nil {
		return state.LastStatus
	}
	return ""
}

func GetLastError(name string) string {
	mu.RLock()
	defer mu.RUnlock()
	if state := serviceLogs[name]; state != nil {
		return state.LastError
	}
	return ""
}

func ClearLastError(name string) {
	mu.Lock()
	if serviceLogs[name] != nil {
		serviceLogs[name].LastError = ""
	}
	mu.Unlock()

	notifyStatusUpdate()
}
