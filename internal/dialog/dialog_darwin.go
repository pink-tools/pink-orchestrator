//go:build darwin

package dialog

import (
	"fmt"
	"html"

	webview "github.com/webview/webview_go"
)

// Show displays a WebView dialog and returns "confirm" or "cancel"
func Show(req Request) string {
	result := "cancel"

	w := webview.New(false)
	defer w.Destroy()

	w.SetTitle(req.Title)
	w.SetSize(450, 320, webview.HintFixed)

	w.Bind("onConfirm", func() {
		result = "confirm"
		w.Terminate()
	})

	w.Bind("onCancel", func() {
		result = "cancel"
		w.Terminate()
	})

	htmlContent := generateHTML(req)
	w.SetHtml(htmlContent)
	w.Run()

	return result
}

func generateHTML(req Request) string {
	confirmBtn := ""
	if req.ConfirmButton != "" {
		confirmBtn = fmt.Sprintf(`<button class="confirm" onclick="onConfirm()">%s</button>`,
			html.EscapeString(req.ConfirmButton))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
	font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
	background: #1a1a2e;
	color: #eee;
	padding: 24px;
	display: flex;
	flex-direction: column;
	height: 100vh;
}
h1 {
	font-size: 18px;
	font-weight: 600;
	margin-bottom: 16px;
	color: #fff;
}
.message {
	flex: 1;
	font-size: 14px;
	line-height: 1.6;
	white-space: pre-wrap;
	color: #ccc;
	overflow-y: auto;
}
.buttons {
	display: flex;
	gap: 12px;
	margin-top: 20px;
	justify-content: flex-end;
}
button {
	padding: 10px 20px;
	border: none;
	border-radius: 6px;
	font-size: 14px;
	font-weight: 500;
	cursor: pointer;
	transition: opacity 0.2s;
}
button:hover { opacity: 0.9; }
.confirm {
	background: #e94560;
	color: white;
}
.cancel {
	background: #0f3460;
	color: white;
}
</style>
</head>
<body>
<h1>%s</h1>
<div class="message">%s</div>
<div class="buttons">
	%s
	<button class="cancel" onclick="onCancel()">%s</button>
</div>
</body>
</html>`,
		html.EscapeString(req.Title),
		html.EscapeString(req.Message),
		confirmBtn,
		html.EscapeString(req.CancelButton))
}
