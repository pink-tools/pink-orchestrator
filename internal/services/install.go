package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/pink-tools/pink-core"
	"github.com/pink-tools/pink-core/log"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

// SetupFunc renders a form from specJSON and returns (values, true) on save,
// or (nil, false) on cancel.
type SetupFunc func(specJSON []byte) (map[string]any, bool)

func Install(name string, progress func(string)) error {
	return InstallWithSetup(name, progress, nil)
}

func InstallWithSetup(name string, progress func(string), setupFunc SetupFunc) error {
	mu.Lock()
	if installingServices[name] {
		mu.Unlock()
		return fmt.Errorf("already installing")
	}
	installingServices[name] = true
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(installingServices, name)
		mu.Unlock()
		if onStatusUpdate != nil {
			onStatusUpdate()
		}
	}()

	svc, err := registry.GetService(name)
	if err != nil {
		return err
	}

	for _, dep := range svc.Dependencies {
		if !IsInstalled(dep) {
			progress(fmt.Sprintf("Installing dependency: %s", dep))
			if err := Install(dep, progress); err != nil {
				return fmt.Errorf("failed to install dependency %s: %w", dep, err)
			}
		}
	}

	if len(svc.SystemDeps) > 0 {
		if err := installSystemDeps(svc.SystemDeps, progress); err != nil {
			return fmt.Errorf("failed to install system deps: %w", err)
		}
	}

	progress(fmt.Sprintf("Downloading %s...", name))

	if err := os.MkdirAll(core.ServiceDir(name), 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}

	tag := svc.ReleaseTag
	if tag == "" {
		// Get actual latest release tag from GitHub API
		latestTag, err := GetLatestReleaseTag(svc.Repo)
		if err != nil {
			return fmt.Errorf("failed to get latest release: %w", err)
		}
		tag = latestTag
	}
	releaseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", svc.Repo, tag, config.BinaryName(name))
	binaryPath := config.ServiceBinary(name)

	if err := downloadFile(releaseURL, binaryPath, progress); err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}

	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	for _, asset := range svc.ExtraAssets {
		progress(fmt.Sprintf("Downloading %s...", asset.Path))
		assetPath := filepath.Join(core.ServiceDir(name), asset.Path)
		if err := downloadFile(asset.URL, assetPath, progress); err != nil {
			return fmt.Errorf("failed to download asset %s: %w", asset.Path, err)
		}
	}

	// Run install --describe to check if service has a setup form
	setupDone := false
	if setupFunc != nil {
		specJSON, err := describeInstallAction(binaryPath)
		if err == nil && len(specJSON) > 0 {
			values, ok := setupFunc(specJSON)
			if ok {
				if err := executeInstallAction(binaryPath, values); err != nil {
					os.RemoveAll(core.ServiceDir(name))
					return fmt.Errorf("install setup failed: %w", err)
				}
				setupDone = true
			}
		}
	}

	// Write .env from registry env_vars (skip if service wrote its own via install action)
	if !setupDone {
		envFile := config.ServiceEnvFile(name)
		if _, err := os.Stat(envFile); os.IsNotExist(err) {
			var envContent strings.Builder
			for _, ev := range svc.EnvVars {
				if ev.Default != "" {
					envContent.WriteString(fmt.Sprintf("%s=%s\n", ev.Name, ev.Default))
				} else {
					envContent.WriteString(fmt.Sprintf("# %s=\n", ev.Name))
				}
			}
			if err := os.WriteFile(envFile, []byte(envContent.String()), 0644); err != nil {
				return fmt.Errorf("failed to write .env file: %w", err)
			}
		}
	}

	installClaudeMd(svc, progress)

	// Verify binary works before saving version
	if err := verifyBinary(binaryPath); err != nil {
		log.Error(context.Background(), "binary verification failed", log.Attr{K: "service", V: name}, log.Attr{K: "binary", V: binaryPath}, log.Attr{K: "error", V: err.Error()})
		return fmt.Errorf("binary verification failed: %w", err)
	}

	// Chown service directory to real user (orchestrator runs as root)
	chownServiceDir(name)

	// Get version from binary for progress message
	if version := GetInstalledVersion(name); version != "" {
		progress(fmt.Sprintf("%s installed (%s)", name, version))
	} else {
		progress(fmt.Sprintf("%s installed", name))
	}

	return nil
}

func describeInstallAction(binaryPath string) ([]byte, error) {
	cmd := exec.Command(binaryPath, "install", "--describe")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

func executeInstallAction(binaryPath string, values map[string]any) error {
	data, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cmd := exec.Command(binaryPath, "install", "--config", string(data))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func chownServiceDir(name string) {
	if runtime.GOOS == "windows" || os.Getuid() != 0 {
		return
	}
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	// Chown parent dir (/Users/pink-tools/) so services can create subdirs
	os.Chown(core.PinkToolsDir(), uid, gid)

	// Chown service dir recursively
	dir := core.ServiceDir(name)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			os.Chown(path, uid, gid)
		}
		return nil
	})

	// Chown agent claude dir (~/pink-tools/.claude/)
	filepath.Walk(config.AgentClaudeDir(), func(path string, info os.FileInfo, err error) error {
		if err == nil {
			os.Chown(path, uid, gid)
		}
		return nil
	})

	// Chown service claude dir (~/pink-tools/<name>/.claude/)
	filepath.Walk(config.AgentClaudeServiceDir(name), func(path string, info os.FileInfo, err error) error {
		if err == nil {
			os.Chown(path, uid, gid)
		}
		return nil
	})
}

// verifyBinary runs --version to check binary is executable and not corrupted
func verifyBinary(path string) error {
	cmd := exec.Command(path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, string(output))
	}
	return nil
}

func Update(name string, progress func(string)) error {
	progress("Checking for updates...")
	hasUpdate, oldVersion, latest, err := CheckUpdate(name)
	if err != nil {
		return fmt.Errorf("failed to check update: %w", err)
	}
	if !hasUpdate {
		progress("Already up to date")
		return nil
	}

	wasRunning := GetStatus(name).Status == StatusRunning
	if wasRunning {
		progress("Stopping service...")
		if err := Stop(name); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
	}

	binaryPath := config.ServiceBinary(name)

	// Rename-first strategy: rename old binary before installing new
	// This works reliably on Windows even without sleep
	var oldPath string
	if IsInstalled(name) {
		oldPath = binaryPath + ".old"
		os.Remove(oldPath) // cleanup from previous update
		if err := os.Rename(binaryPath, oldPath); err != nil {
			return fmt.Errorf("failed to move old binary (still locked?): %w", err)
		}
	}

	if err := Install(name, progress); err != nil {
		// Restore old binary on failure
		if oldPath != "" {
			os.Rename(oldPath, binaryPath)
		}
		return err
	}

	// Cleanup old binary only on success
	if oldPath != "" {
		os.Remove(oldPath)
	}

	progress(fmt.Sprintf("Updated: %s → %s", oldVersion, latest))

	if wasRunning {
		progress("Restarting service...")
		if err := Start(name); err != nil {
			return fmt.Errorf("failed to restart service: %w", err)
		}
	}

	return nil
}

func Uninstall(name string) error {
	if err := Stop(name); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	os.RemoveAll(config.AgentClaudeServiceDir(name))
	removeFromProjectsMd(name)

	return os.Remove(config.ServiceBinary(name))
}

func removeFromProjectsMd(name string) {
	projectsFile := config.AgentClaudeProjectsMd()
	data, err := os.ReadFile(projectsFile)
	if err != nil {
		return
	}
	refLine := fmt.Sprintf("@../%s/.claude/CLAUDE.md", name)
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) != refLine {
			out = append(out, line)
		}
	}
	os.WriteFile(projectsFile, []byte(strings.Join(out, "\n")), 0644)
}

func Check(name string) (string, error) {
	if !IsInstalled(name) {
		return "", fmt.Errorf("not installed")
	}

	binary := config.ServiceBinary(name)
	cmd := exec.Command(binary, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("check failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func OpenEnvFile(name string) error {
	envFile := config.ServiceEnvFile(name)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-t", envFile)
	case "linux":
		cmd = exec.Command("xdg-open", envFile)
	case "windows":
		cmd = exec.Command("notepad", envFile)
	}

	return cmd.Start()
}

func downloadFile(url, dest string, progress func(string)) error {
	tmpFile := dest + ".tmp"

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d %s (url: %s)", resp.StatusCode, http.StatusText(resp.StatusCode), url)
	}

	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	total := resp.ContentLength
	var downloaded int64
	var lastPct int
	buf := make([]byte, 32*1024)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				out.Close()
				os.Remove(tmpFile)
				return writeErr
			}
			downloaded += int64(n)
			if total > 0 {
				pct := int(float64(downloaded) / float64(total) * 100)
				if pct >= lastPct+5 || pct == 100 {
					progress(fmt.Sprintf("%d%% (%s / %s)", pct, formatBytes(downloaded), formatBytes(total)))
					lastPct = pct
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			os.Remove(tmpFile)
			return readErr
		}
	}

	out.Close()

	if total > 0 && downloaded != total {
		os.Remove(tmpFile)
		return fmt.Errorf("incomplete download: got %d bytes, expected %d", downloaded, total)
	}

	if err := os.Rename(tmpFile, dest); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to finalize download: %w", err)
	}

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func installSystemDeps(deps []registry.SystemDep, progress func(string)) error {
	for _, dep := range deps {
		if isCommandAvailable(dep.Name) {
			continue
		}

		progress(fmt.Sprintf("Installing system dependency: %s", dep.Name))

		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			if dep.UnixScript != "" {
				cmd = userCommand("bash", "-c", dep.UnixScript)
			} else if dep.Brew != "" {
				if !isCommandAvailable("brew") {
					return fmt.Errorf("brew is not installed. Install from https://brew.sh")
				}
				cmd = userCommand("brew", "install", dep.Brew)
			} else {
				return fmt.Errorf("no install method for %s on darwin", dep.Name)
			}
		case "linux":
			if dep.UnixScript != "" {
				cmd = exec.Command("bash", "-c", dep.UnixScript)
			} else if dep.Apt != "" {
				cmd = exec.Command("sudo", "apt-get", "install", "-y", dep.Apt)
			} else {
				return fmt.Errorf("no install method for %s on linux", dep.Name)
			}
		case "windows":
			if dep.Winget != "" {
				if !isCommandAvailable("winget") {
					progress("Installing winget...")
					installWinget()
				}
				cmd = exec.Command("winget", "install",
					"--silent",
					"--disable-interactivity",
					"--accept-package-agreements",
					"--accept-source-agreements",
					"--no-upgrade",
					"--force",
					dep.Winget)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				cmd.Run() // ignore errors - either installed or not, continue
				continue
			} else if dep.WinScript != "" {
				cmd = exec.Command("powershell", "-NoProfile", "-Command", dep.WinScript)
			} else if dep.UnixScript != "" && isCommandAvailable("bash") {
				cmd = exec.Command("bash", "-c", dep.UnixScript)
			} else {
				return fmt.Errorf("no install method for %s on windows", dep.Name)
			}
		default:
			return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install %s: %w", dep.Name, err)
		}
	}
	return nil
}

func installWinget() {
	// Use asheroto/winget-install script - handles all dependencies automatically
	// Works on Windows 10/11 and Server 2019/2022
	// https://github.com/asheroto/winget-install
	script := `
$ProgressPreference = 'SilentlyContinue'
try {
    Invoke-RestMethod asheroto.com/winget | Invoke-Expression
} catch {}
`
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// userCommand wraps exec.Command to drop privileges when running as root.
// brew and unix_script must not run as root on macOS.
func userCommand(name string, args ...string) *exec.Cmd {
	if os.Getuid() == 0 {
		if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
			return exec.Command("sudo", append([]string{"-u", sudoUser, name}, args...)...)
		}
	}
	return exec.Command(name, args...)
}

func replaceTemplateVars(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	result := strings.ReplaceAll(string(data), "{{PINK_TOOLS}}", strings.TrimSuffix(core.PinkToolsDir(), string(filepath.Separator)))
	os.WriteFile(path, []byte(result), 0644)
}

func installClaudeMd(svc *registry.Service, progress func(string)) {
	if svc.ClaudeRoot {
		installClaudeRoot(svc, progress)
	} else {
		installClaudeService(svc, progress)
	}
}

func installClaudeRoot(svc *registry.Service, progress func(string)) {
	claudeDir := config.AgentClaudeDir()
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		log.Error(context.Background(), "create claude dir failed", log.Attr{K: "error", V: err.Error()})
		return
	}

	baseURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/.claude", svc.Repo)

	// Always overwrite CLAUDE.md, CODE.md, MCP.md
	for _, file := range []string{"CLAUDE.md", "CODE.md", "MCP.md"} {
		destPath := filepath.Join(claudeDir, file)
		if err := downloadFile(baseURL+"/"+file, destPath, progress); err != nil {
			log.Error(context.Background(), "download claude file failed", log.Attr{K: "file", V: file}, log.Attr{K: "error", V: err.Error()})
			continue
		}
		replaceTemplateVars(destPath)
	}

	// PROJECTS.md: download fresh template, then merge with discovered services
	installProjectsMd(svc.Repo, progress)

	// Install orchestrator docs (always bundled with agent)
	installOrchestratorDocs(progress)
}

// installProjectsMd downloads fresh PROJECTS.md template, then scans for installed
// services with .claude/CLAUDE.md and ensures they're referenced.
func installProjectsMd(repo string, progress func(string)) {
	destPath := config.AgentClaudeProjectsMd()
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/.claude/PROJECTS.md", repo)

	if err := downloadFile(url, destPath, progress); err != nil {
		log.Error(context.Background(), "download PROJECTS.md failed", log.Attr{K: "error", V: err.Error()})
		return
	}
	replaceTemplateVars(destPath)

	// Scan ~/pink-tools/*/.claude/CLAUDE.md for installed services
	entries, err := os.ReadDir(core.PinkToolsDir())
	if err != nil {
		return
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		return
	}
	text := string(content)

	var added bool
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		claudeMd := filepath.Join(core.PinkToolsDir(), name, ".claude", "CLAUDE.md")
		if _, err := os.Stat(claudeMd); err != nil {
			continue
		}
		refLine := fmt.Sprintf("@../%s/.claude/CLAUDE.md", name)
		if !strings.Contains(text, refLine) {
			text += refLine + "\n"
			added = true
		}
	}

	if added {
		os.WriteFile(destPath, []byte(text), 0644)
	}
}

func installOrchestratorDocs(progress func(string)) {
	claudeDir := config.AgentClaudeServiceDir("pink-orchestrator")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return
	}

	dest := config.AgentClaudeServiceMd("pink-orchestrator")
	url := "https://raw.githubusercontent.com/pink-tools/pink-orchestrator/main/.claude/CLAUDE.md"
	if err := downloadFile(url, dest, progress); err != nil {
		log.Error(context.Background(), "download orchestrator CLAUDE.md failed", log.Attr{K: "error", V: err.Error()})
		return
	}
	replaceTemplateVars(dest)
}

func installClaudeService(svc *registry.Service, progress func(string)) {
	claudeDir := config.AgentClaudeServiceDir(svc.Name)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		log.Error(context.Background(), "create service claude dir failed", log.Attr{K: "service", V: svc.Name}, log.Attr{K: "error", V: err.Error()})
		return
	}

	claudeMdURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/.claude/CLAUDE.md", svc.Repo)
	claudeMdPath := config.AgentClaudeServiceMd(svc.Name)

	if err := downloadFile(claudeMdURL, claudeMdPath, progress); err != nil {
		log.Error(context.Background(), "download service CLAUDE.md failed", log.Attr{K: "service", V: svc.Name}, log.Attr{K: "error", V: err.Error()})
		return
	}
	replaceTemplateVars(claudeMdPath)

	updateProjectsMd(svc.Name)
}

// NeedsSetup checks if a service with has_setup requires setup.
// Returns true if install --check reports ready: false.
func NeedsSetup(name string) bool {
	svc, err := registry.GetService(name)
	if err != nil || !svc.HasSetup {
		return false
	}
	if !IsInstalled(name) {
		return false
	}

	binary := config.ServiceBinary(name)
	cmd := exec.Command(binary, "install", "--check")
	output, err := cmd.Output()
	if err != nil {
		return true // can't check → assume needs setup
	}

	var info struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(output, &info); err != nil {
		return true
	}
	return !info.Ready
}

// HasSetup returns whether a service has the has_setup flag.
func HasSetup(name string) bool {
	svc, err := registry.GetService(name)
	if err != nil {
		return false
	}
	return svc.HasSetup
}

// RunSetupTerminal opens a terminal window running the service's install command.
func RunSetupTerminal(name string) error {
	binary := config.ServiceBinary(name)

	switch runtime.GOOS {
	case "darwin":
		// Write a temp script that runs the install command
		script := fmt.Sprintf("#!/bin/bash\n%s install\necho\necho 'Setup complete. Press any key to close.'\nread -n1\n", binary)
		tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s-setup.sh", name))
		if err := os.WriteFile(tmpFile, []byte(script), 0755); err != nil {
			return fmt.Errorf("write setup script: %w", err)
		}
		return exec.Command("open", "-a", "Terminal.app", tmpFile).Start()

	case "linux":
		return exec.Command("x-terminal-emulator", "-e", binary, "install").Start()

	case "windows":
		return exec.Command("cmd", "/c", "start", "cmd", "/k", binary, "install").Start()

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func updateProjectsMd(name string) {
	projectsFile := config.AgentClaudeProjectsMd()
	refLine := fmt.Sprintf("@../%s/.claude/CLAUDE.md", name)

	content, err := os.ReadFile(projectsFile)
	if err != nil {
		content = []byte("# Installed Services\n\n")
	}

	if strings.Contains(string(content), refLine) {
		return
	}

	f, err := os.OpenFile(projectsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if len(content) == 0 {
		f.WriteString("# Installed Services\n\n")
	}
	f.WriteString(refLine + "\n")
}
