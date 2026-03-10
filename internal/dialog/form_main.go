package dialog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/ncruces/zenity"
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

	w.Bind("openFile", func(extensions string) string {
		return openFileDialog(extensions)
	})

	w.SetHtml(formHTML(specJSON))
	w.Run()

	if saved {
		data, err := json.Marshal(values)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dialog: failed to marshal values: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(data)
	}
}

func openFileDialog(extensions string) string {
	opts := []zenity.Option{zenity.Title("Choose file")}
	if extensions != "" {
		var patterns []string
		for _, ext := range strings.Split(extensions, ",") {
			ext = strings.TrimSpace(ext)
			if ext != "" {
				patterns = append(patterns, "*"+ext)
			}
		}
		if len(patterns) > 0 {
			opts = append(opts, zenity.FileFilters{{Name: "Files", Patterns: patterns}})
		}
	}
	path, err := zenity.SelectFile(opts...)
	if err != nil {
		return ""
	}
	return path
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
