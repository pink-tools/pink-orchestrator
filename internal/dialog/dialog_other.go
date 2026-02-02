//go:build !darwin

package dialog

// Show returns "cancel" on non-macOS platforms.
// Services will fall back to CLI prompt.
func Show(req Request) string {
	return "cancel"
}
