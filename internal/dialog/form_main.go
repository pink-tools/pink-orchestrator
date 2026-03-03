package dialog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/pink-tools/pink-orchestrator/internal/player"
	webview "github.com/webview/webview_go"
)

// FormMain is the entry point for the --dialog subprocess.
// Reads FormSpec JSON from stdin, shows webview, outputs result JSON to stdout.
// Must be called from the main goroutine (webview needs the main OS thread).
func FormMain() {
	runtime.LockOSThread()

	specJSON, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dialog: failed to read stdin: %v\n", err)
		os.Exit(1)
	}

	var values map[string]any
	saved := false

	w := webview.New(false)
	defer w.Destroy()

	w.SetTitle("Settings")
	w.SetSize(500, dialogHeight(specJSON), webview.HintNone)

	w.Bind("onSave", func(jsonString string) {
		if err := json.Unmarshal([]byte(jsonString), &values); err == nil {
			saved = true
		}
		w.Terminate()
	})

	w.Bind("onCancel", func() {
		w.Terminate()
	})

	pl, _ := player.New()
	if pl != nil {
		defer pl.Close()
	}

	w.Bind("playSound", func(path string, volume float64) {
		if pl != nil {
			pl.Play(path, volume)
		}
	})

	w.Bind("openFile", func() string {
		return openFileDialog()
	})

	w.SetHtml(formHTML(specJSON))
	w.Run()

	if saved {
		data, _ := json.Marshal(values)
		os.Stdout.Write(data)
	}
}

func openFileDialog() string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("osascript", "-e", `POSIX path of (choose file of type {"public.audio"} with prompt "Choose sound file")`)
	case "linux":
		cmd = exec.Command("zenity", "--file-selection", "--file-filter=Audio files|*.wav *.aiff *.aif *.mp3 *.ogg *.flac")
	case "windows":
		cmd = exec.Command("powershell", "-NoProfile", "-Command",
			`Add-Type -AssemblyName System.Windows.Forms; $d = New-Object System.Windows.Forms.OpenFileDialog; $d.Filter = 'Audio files|*.wav;*.mp3;*.aiff;*.flac'; if ($d.ShowDialog() -eq 'OK') { $d.FileName }`)
	default:
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func dialogHeight(specJSON []byte) int {
	var spec struct {
		Fields []json.RawMessage `json:"fields"`
	}
	json.Unmarshal(specJSON, &spec)

	h := 130 + len(spec.Fields)*70
	if h < 400 {
		h = 400
	}
	if h > 900 {
		h = 900
	}
	return h
}
