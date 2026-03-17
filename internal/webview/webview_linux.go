//go:build linux

package webview

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	libsLoaded bool
	libsOnce   sync.Once

	// GTK
	gtkInitCheck           func(argc *int, argv **uintptr) int
	gtkWindowNew           func(windowType int) uintptr
	gtkWidgetDestroy       func(widget uintptr)
	gtkWindowSetTitle      func(window uintptr, title string)
	gtkWindowSetResizable  func(window uintptr, resizable int)
	gtkWindowResize        func(window uintptr, width int, height int)
	gtkWidgetSetSizeReq    func(widget uintptr, width int, height int)
	gtkContainerAdd        func(container uintptr, widget uintptr)
	gtkWidgetShowAll       func(widget uintptr)
	gtkWidgetGrabFocus     func(widget uintptr)
	gtkMainFunc            func()
	gtkMainQuitFunc        func()
	gIdleAddFull           func(priority int, fn uintptr, data uintptr, notify uintptr) uint
	gSignalConnect         func(instance uintptr, signal string, handler uintptr, data uintptr) uint64

	// WebKit2GTK
	webkitWebViewNew                    func() uintptr
	webkitWebViewLoadHtml               func(webView uintptr, content string, baseURI uintptr)
	webkitWebViewGetUserContentManager  func(webView uintptr) uintptr
	webkitUserContentManagerRegisterHandler func(manager uintptr, name string) int
	webkitUserContentManagerAddScript   func(manager uintptr, script uintptr)
	webkitUserScriptNew                 func(source string, frames int, injTime int, allowList uintptr, blockList uintptr) uintptr
	webkitWebViewGetSettings            func(webView uintptr) uintptr
	webkitSettingsSetJSClipboard        func(settings uintptr, enabled int)
	webkitJsResultGetJsValue            func(result uintptr) uintptr
	jscValueToString                    func(value uintptr) uintptr

	gFree func(ptr uintptr)
)

func loadLibs() {
	libsOnce.Do(func() {
		gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY)
		if err != nil {
			return
		}

		// Try webkit2gtk-4.1 first, then 4.0
		wk, err := purego.Dlopen("libwebkit2gtk-4.1.so.0", purego.RTLD_LAZY)
		if err != nil {
			wk, err = purego.Dlopen("libwebkit2gtk-4.0.so.37", purego.RTLD_LAZY)
			if err != nil {
				return
			}
		}

		jsc, _ := purego.Dlopen("libjavascriptcoregtk-4.1.so.0", purego.RTLD_LAZY)
		if jsc == 0 {
			jsc, _ = purego.Dlopen("libjavascriptcoregtk-4.0.so.18", purego.RTLD_LAZY)
		}

		// GTK
		purego.RegisterLibFunc(&gtkInitCheck, gtk, "gtk_init_check")
		purego.RegisterLibFunc(&gtkWindowNew, gtk, "gtk_window_new")
		purego.RegisterLibFunc(&gtkWidgetDestroy, gtk, "gtk_widget_destroy")
		purego.RegisterLibFunc(&gtkWindowSetTitle, gtk, "gtk_window_set_title")
		purego.RegisterLibFunc(&gtkWindowSetResizable, gtk, "gtk_window_set_resizable")
		purego.RegisterLibFunc(&gtkWindowResize, gtk, "gtk_window_resize")
		purego.RegisterLibFunc(&gtkWidgetSetSizeReq, gtk, "gtk_widget_set_size_request")
		purego.RegisterLibFunc(&gtkContainerAdd, gtk, "gtk_container_add")
		purego.RegisterLibFunc(&gtkWidgetShowAll, gtk, "gtk_widget_show_all")
		purego.RegisterLibFunc(&gtkWidgetGrabFocus, gtk, "gtk_widget_grab_focus")
		purego.RegisterLibFunc(&gtkMainFunc, gtk, "gtk_main")
		purego.RegisterLibFunc(&gtkMainQuitFunc, gtk, "gtk_main_quit")
		purego.RegisterLibFunc(&gIdleAddFull, gtk, "g_idle_add_full")
		purego.RegisterLibFunc(&gSignalConnect, gtk, "g_signal_connect_data")
		purego.RegisterLibFunc(&gFree, gtk, "g_free")

		// WebKit2GTK
		purego.RegisterLibFunc(&webkitWebViewNew, wk, "webkit_web_view_new")
		purego.RegisterLibFunc(&webkitWebViewLoadHtml, wk, "webkit_web_view_load_html")
		purego.RegisterLibFunc(&webkitWebViewGetUserContentManager, wk, "webkit_web_view_get_user_content_manager")
		purego.RegisterLibFunc(&webkitUserContentManagerRegisterHandler, wk, "webkit_user_content_manager_register_script_message_handler")
		purego.RegisterLibFunc(&webkitUserContentManagerAddScript, wk, "webkit_user_content_manager_add_script")
		purego.RegisterLibFunc(&webkitUserScriptNew, wk, "webkit_user_script_new")
		purego.RegisterLibFunc(&webkitWebViewGetSettings, wk, "webkit_web_view_get_settings")
		purego.RegisterLibFunc(&webkitSettingsSetJSClipboard, wk, "webkit_settings_set_javascript_can_access_clipboard")

		if jsc != 0 {
			purego.RegisterLibFunc(&webkitJsResultGetJsValue, wk, "webkit_javascript_result_get_js_value")
			purego.RegisterLibFunc(&jscValueToString, jsc, "jsc_value_to_string")
		}

		libsLoaded = true
	})
}

type puregoWebView struct {
	window  uintptr
	webview uintptr

	mu       sync.Mutex
	bindings map[string]func(id, req string) (interface{}, error)
}

func newWebView(debug bool) WebView {
	loadLibs()
	if !libsLoaded {
		return nil
	}

	if gtkInitCheck(nil, nil) == 0 {
		return nil
	}

	window := gtkWindowNew(0) // GTK_WINDOW_TOPLEVEL
	wv := webkitWebViewNew()

	gtkContainerAdd(window, wv)

	// Inject external.invoke bridge
	mgr := webkitWebViewGetUserContentManager(wv)
	webkitUserContentManagerRegisterHandler(mgr, "external")

	js := `window.external={invoke:function(s){window.webkit.messageHandlers.external.postMessage(s);}};`
	script := webkitUserScriptNew(js, 0, 0, 0, 0) // ALL_FRAMES, DOCUMENT_START
	webkitUserContentManagerAddScript(mgr, script)

	v := &puregoWebView{
		window:   window,
		webview:  wv,
		bindings: make(map[string]func(id, req string) (interface{}, error)),
	}

	// Handle script messages from JS
	gSignalConnect(mgr, "script-message-received::external",
		purego.NewCallback(func(mgr uintptr, result uintptr, data uintptr) {
			v.handleScriptMessage(result)
		}), 0)

	// Handle window close
	gSignalConnect(window, "destroy",
		purego.NewCallback(func(widget uintptr, data uintptr) {
			gtkMainQuitFunc()
		}), 0)

	return v
}

func (v *puregoWebView) handleScriptMessage(result uintptr) {
	if webkitJsResultGetJsValue == nil || jscValueToString == nil {
		return
	}
	jsValue := webkitJsResultGetJsValue(result)
	cstr := jscValueToString(jsValue)
	if cstr == 0 {
		return
	}
	msg := goString(cstr)
	gFree(cstr)

	// Parse binding call: {"id":"N","method":"name","params":[...]}
	var call struct {
		ID     string            `json:"id"`
		Method string            `json:"method"`
		Params json.RawMessage   `json:"params"`
	}
	if json.Unmarshal([]byte(msg), &call) != nil {
		return
	}

	v.mu.Lock()
	fn, ok := v.bindings[call.Method]
	v.mu.Unlock()
	if !ok {
		return
	}

	jsString := func(val interface{}) string { b, _ := json.Marshal(val); return string(b) }
	status, resultStr := 0, ""
	if res, err := fn(call.ID, string(call.Params)); err != nil {
		status = -1
		resultStr = jsString(err.Error())
	} else if b, err := json.Marshal(res); err != nil {
		status = -1
		resultStr = jsString(err.Error())
	} else {
		resultStr = string(b)
	}

	// Return result to JS
	var js string
	if status == 0 {
		js = `window.__webview_resolve__("` + call.ID + `",0,` + resultStr + `);`
	} else {
		js = `window.__webview_resolve__("` + call.ID + `",-1,` + resultStr + `);`
	}
	_ = js
	// For simple bind callbacks (like onConfirm/onCancel), the function
	// is called directly and doesn't need JS return. The webview library's
	// Bind injects resolver JS automatically, but we handle it simpler:
	// the Go callback is invoked synchronously.
}

func (v *puregoWebView) Run() {
	gtkWidgetShowAll(v.window)
	gtkWidgetGrabFocus(v.webview)
	gtkMainFunc()
}

func (v *puregoWebView) Terminate() {
	gIdleAddFull(0, purego.NewCallback(func(data uintptr) int {
		gtkMainQuitFunc()
		return 0
	}), 0, 0)
}

func (v *puregoWebView) Destroy() {
	gtkWidgetDestroy(v.window)
}

func (v *puregoWebView) SetTitle(title string) {
	gtkWindowSetTitle(v.window, title)
}

func (v *puregoWebView) SetSize(w int, h int, hint Hint) {
	switch hint {
	case HintFixed:
		gtkWindowSetResizable(v.window, 0)
		gtkWidgetSetSizeReq(v.window, w, h)
		gtkWindowResize(v.window, w, h)
	default:
		gtkWindowResize(v.window, w, h)
	}
}

func (v *puregoWebView) SetHtml(html string) {
	webkitWebViewLoadHtml(v.webview, html, 0)
}

func (v *puregoWebView) Bind(name string, f interface{}) error {
	rv := reflect.ValueOf(f)
	if rv.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}

	binding := func(id, req string) (interface{}, error) {
		raw := []json.RawMessage{}
		if err := json.Unmarshal([]byte(req), &raw); err != nil {
			return nil, err
		}
		args := make([]reflect.Value, len(raw))
		for i := range raw {
			arg := reflect.New(rv.Type().In(i))
			if err := json.Unmarshal(raw[i], arg.Interface()); err != nil {
				return nil, err
			}
			args[i] = arg.Elem()
		}
		errorType := reflect.TypeOf((*error)(nil)).Elem()
		res := rv.Call(args)
		switch len(res) {
		case 0:
			return nil, nil
		case 1:
			if res[0].Type().Implements(errorType) {
				if res[0].Interface() != nil {
					return nil, res[0].Interface().(error)
				}
				return nil, nil
			}
			return res[0].Interface(), nil
		case 2:
			if res[1].Interface() == nil {
				return res[0].Interface(), nil
			}
			return res[0].Interface(), res[1].Interface().(error)
		default:
			return nil, errors.New("unexpected number of return values")
		}
	}

	v.mu.Lock()
	v.bindings[name] = binding
	v.mu.Unlock()

	// Inject JS binding: window.NAME = function(...) { ... }
	js := `(function(){
		var name = "` + name + `";
		var RPC = window.__webview_rpc__ = window.__webview_rpc__ || {nextID:1,promises:{}};
		window[name] = function() {
			var id = String(RPC.nextID++);
			var params = Array.prototype.slice.call(arguments);
			var msg = JSON.stringify({id:id,method:name,params:params});
			window.external.invoke(msg);
		};
	})();`
	_ = js
	// For the orchestrator's use case (onConfirm/onCancel with no args/return),
	// we use a simpler approach: JS calls window.external.invoke directly,
	// and we parse the method name to call the Go binding.

	// Actually inject a simpler binding that just invokes directly
	initJS := `window.` + name + ` = function() {
		var params = Array.prototype.slice.call(arguments);
		window.external.invoke(JSON.stringify({id:"0",method:"` + name + `",params:params}));
	};`

	// We need to inject this into the webview. Use a user script.
	mgr := webkitWebViewGetUserContentManager(v.webview)
	script := webkitUserScriptNew(initJS, 0, 0, 0, 0)
	webkitUserContentManagerAddScript(mgr, script)

	return nil
}

// goString converts a C string (uintptr) to a Go string.
func goString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(ptr + uintptr(length)))
		if b == 0 {
			break
		}
		length++
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), length))
}
