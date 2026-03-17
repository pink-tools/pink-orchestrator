// Package webview provides a minimal cross-platform webview.
// On macOS/Windows it wraps the CGO-based webview library.
// On Linux it uses purego to dlopen GTK/WebKit2GTK at runtime,
// so the binary runs on headless servers without GUI libraries.
package webview

// Hint configures window sizing.
type Hint int

const (
	HintNone  Hint = 0
	HintFixed Hint = 3
)

// WebView is the interface for a webview window.
type WebView interface {
	Run()
	Terminate()
	Destroy()
	SetTitle(title string)
	SetSize(w int, h int, hint Hint)
	SetHtml(html string)
	Bind(name string, f interface{}) error
}

// New creates a new webview. Returns nil if GUI is not available.
func New(debug bool) WebView {
	return newWebView(debug)
}
