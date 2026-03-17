//go:build !linux

package systray

// Available reports whether GUI libraries are loaded.
// On macOS and Windows, GUI is always available.
func Available() bool { return true }
