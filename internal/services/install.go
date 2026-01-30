package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pink-tools/pink-otel"
	"github.com/pink-tools/pink-orchestrator/internal/config"
	"github.com/pink-tools/pink-orchestrator/internal/registry"
)

func Install(name string, progress func(string)) error {
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

	if err := os.MkdirAll(config.ServiceDir(name), 0755); err != nil {
		return fmt.Errorf("failed to create service directory: %w", err)
	}

	releaseURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", svc.Repo, config.BinaryName(name))
	binaryPath := config.ServiceBinary(name)

	if err := downloadFile(releaseURL, binaryPath, progress); err != nil {
		return fmt.Errorf("failed to download binary: %w", err)
	}

	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	for _, asset := range svc.ExtraAssets {
		progress(fmt.Sprintf("Downloading %s...", asset.Path))
		assetPath := filepath.Join(config.ServiceDir(name), asset.Path)
		if err := downloadFile(asset.URL, assetPath, progress); err != nil {
			return fmt.Errorf("failed to download asset %s: %w", asset.Path, err)
		}
	}

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

	createSymlink(name, progress)

	installClaudeMd(svc, progress)

	// Verify binary works before saving version
	if err := verifyBinary(binaryPath); err != nil {
		otel.Error(context.Background(), "binary verification failed", otel.Attr{"service", name}, otel.Attr{"binary", binaryPath}, otel.Attr{"error", err.Error()})
		return fmt.Errorf("binary verification failed: %w", err)
	}

	// Save version AFTER verification
	if version, err := GetLatestVersion(svc.Repo); err == nil {
		SetInstalledVersion(name, version)
		progress(fmt.Sprintf("%s installed (release: %s)", name, version))
	} else {
		progress(fmt.Sprintf("%s installed", name))
	}

	return nil
}

// verifyBinary runs --version to check binary is executable and not corrupted
func verifyBinary(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
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
	if IsInstalled(name) {
		oldPath := binaryPath + ".old"
		os.Remove(oldPath) // cleanup from previous update
		if err := os.Rename(binaryPath, oldPath); err != nil {
			return fmt.Errorf("failed to move old binary (still locked?): %w", err)
		}
		defer os.Remove(oldPath) // cleanup after success
	}

	if err := Install(name, progress); err != nil {
		return err
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

	if runtime.GOOS != "windows" {
		linkPath := filepath.Join("/usr/local/bin", name)
		os.Remove(linkPath)
	}

	return os.Remove(config.ServiceBinary(name))
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

func createSymlink(name string, progress func(string)) {
	if runtime.GOOS == "windows" {
		return
	}

	binary := config.ServiceBinary(name)
	linkPath := filepath.Join("/usr/local/bin", name)

	os.Remove(linkPath)
	if err := os.Symlink(binary, linkPath); err != nil {
		progress(fmt.Sprintf("Warning: failed to create symlink: %v", err))
	}
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
				cmd = exec.Command("bash", "-c", dep.UnixScript)
			} else if dep.Brew != "" {
				cmd = exec.Command("brew", "install", dep.Brew)
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

func installClaudeMd(svc *registry.Service, progress func(string)) {
	if svc.ClaudeRoot {
		installClaudeRoot(svc, progress)
	} else {
		installClaudeService(svc, progress)
	}
}

func installClaudeRoot(svc *registry.Service, progress func(string)) {
	claudeDir := config.ClaudeDir()
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return
	}

	files := []string{"CLAUDE.md", "CODE.md", "PROJECTS.md", "MCP.md"}
	baseURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/.claude", svc.Repo)

	for _, file := range files {
		destPath := filepath.Join(claudeDir, file)
		if _, err := os.Stat(destPath); err == nil {
			continue
		}
		downloadFile(baseURL+"/"+file, destPath, progress)
	}

	// Install orchestrator docs (always bundled with agent)
	installOrchestratorDocs(progress)
}

func installOrchestratorDocs(progress func(string)) {
	claudeDir := config.ClaudeServiceDir("pink-orchestrator")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return
	}

	dest := config.ClaudeServiceMd("pink-orchestrator")
	if _, err := os.Stat(dest); err == nil {
		return
	}

	url := "https://raw.githubusercontent.com/pink-tools/pink-orchestrator/main/.claude/CLAUDE.md"
	downloadFile(url, dest, progress)
}

func installClaudeService(svc *registry.Service, progress func(string)) {
	claudeDir := config.ClaudeServiceDir(svc.Name)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return
	}

	claudeMdURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/.claude/CLAUDE.md", svc.Repo)
	claudeMdPath := config.ClaudeServiceMd(svc.Name)

	if err := downloadFile(claudeMdURL, claudeMdPath, progress); err != nil {
		return
	}

	updateProjectsMd(svc.Name)
}

func updateProjectsMd(name string) {
	projectsFile := config.ClaudeProjectsMd()
	refLine := fmt.Sprintf("@pink-tools/%s/CLAUDE.md", name)

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
