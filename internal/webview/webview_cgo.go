//go:build darwin || windows

package webview

import (
	upstream "github.com/webview/webview_go"
)

type cgoWebView struct {
	w upstream.WebView
}

func newWebView(debug bool) WebView {
	w := upstream.New(debug)
	if w == nil {
		return nil
	}
	return &cgoWebView{w: w}
}

func (v *cgoWebView) Run()                          { v.w.Run() }
func (v *cgoWebView) Terminate()                    { v.w.Terminate() }
func (v *cgoWebView) Destroy()                      { v.w.Destroy() }
func (v *cgoWebView) SetTitle(title string)         { v.w.SetTitle(title) }
func (v *cgoWebView) SetHtml(html string)           { v.w.SetHtml(html) }
func (v *cgoWebView) Bind(name string, f interface{}) error { return v.w.Bind(name, f) }

func (v *cgoWebView) SetSize(w int, h int, hint Hint) {
	v.w.SetSize(w, h, upstream.Hint(hint))
}
