//go:build linux

package systray

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Function pointers loaded via dlopen/dlsym
var (
	gtkInit                      func(argc *int, argv **uintptr)
	gtkMain                      func()
	gtkMainQuit                  func()
	gtkMenuNew                   func() uintptr
	gtkMenuItemNewWithLabel      func(label string) uintptr
	gtkCheckMenuItemNewWithLabel func(label string) uintptr
	gtkCheckMenuItemSetActive    func(item uintptr, active int)
	gtkSeparatorMenuItemNew      func() uintptr
	gtkMenuShellAppend           func(shell uintptr, child uintptr)
	gtkMenuItemSetLabel          func(item uintptr, label string)
	gtkMenuItemGetSubmenu        func(item uintptr) uintptr
	gtkMenuItemSetSubmenu        func(item uintptr, submenu uintptr)
	gtkWidgetShow                func(widget uintptr)
	gtkWidgetHide                func(widget uintptr)
	gtkWidgetSetSensitive        func(widget uintptr, sensitive int)
	gIdleAdd                     func(fn uintptr, data uintptr) uint
	gSignalConnectData           func(instance uintptr, signal string, handler uintptr, data uintptr, destroy uintptr, flags int) uint64
	gSignalHandlerBlock          func(instance uintptr, id uint64)
	gSignalHandlerUnblock        func(instance uintptr, id uint64)
	gBytesNewStatic              func(data uintptr, size int) uintptr
	gBytesGetData                func(bytes uintptr, size *int) uintptr
	gBytesUnref                  func(bytes uintptr)

	appIndicatorNew                  func(id string, iconName string, category int) uintptr
	appIndicatorSetStatus            func(indicator uintptr, status int)
	appIndicatorSetMenu              func(indicator uintptr, menu uintptr)
	appIndicatorSetIconFull          func(indicator uintptr, name string, desc string)
	appIndicatorSetAttentionIconFull func(indicator uintptr, name string, desc string)
	appIndicatorSetTitle             func(indicator uintptr, title string)
	appIndicatorSetLabel             func(indicator uintptr, label string, guide string)
)

const (
	appIndicatorCategoryApplicationStatus = 0
	appIndicatorStatusActive              = 1
	appIndicatorStatusPassive             = 2
)

var (
	libGTK       uintptr
	libIndicator uintptr

	guiAvailable bool
	guiOnce      sync.Once

	globalIndicator uintptr
	globalMenu      uintptr

	menuItemsMu sync.Mutex
	menuNodes   []*menuItemNode

	tempIconPath string
)

type menuItemNode struct {
	widget          uintptr
	id              int
	signalHandlerID uint64
}

// Available reports whether GUI libraries could be loaded.
func Available() bool {
	guiOnce.Do(loadLibs)
	return guiAvailable
}

func loadLibs() {
	gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY)
	if err != nil {
		return
	}

	// Try ayatana first, then legacy appindicator
	ind, err := purego.Dlopen("libayatana-appindicator3.so.1", purego.RTLD_LAZY)
	if err != nil {
		ind, err = purego.Dlopen("libappindicator3.so.1", purego.RTLD_LAZY)
		if err != nil {
			return
		}
	}

	libGTK = gtk
	libIndicator = ind

	// GTK functions
	purego.RegisterLibFunc(&gtkInit, gtk, "gtk_init")
	purego.RegisterLibFunc(&gtkMain, gtk, "gtk_main")
	purego.RegisterLibFunc(&gtkMainQuit, gtk, "gtk_main_quit")
	purego.RegisterLibFunc(&gtkMenuNew, gtk, "gtk_menu_new")
	purego.RegisterLibFunc(&gtkMenuItemNewWithLabel, gtk, "gtk_menu_item_new_with_label")
	purego.RegisterLibFunc(&gtkCheckMenuItemNewWithLabel, gtk, "gtk_check_menu_item_new_with_label")
	purego.RegisterLibFunc(&gtkCheckMenuItemSetActive, gtk, "gtk_check_menu_item_set_active")
	purego.RegisterLibFunc(&gtkSeparatorMenuItemNew, gtk, "gtk_separator_menu_item_new")
	purego.RegisterLibFunc(&gtkMenuShellAppend, gtk, "gtk_menu_shell_append")
	purego.RegisterLibFunc(&gtkMenuItemSetLabel, gtk, "gtk_menu_item_set_label")
	purego.RegisterLibFunc(&gtkMenuItemGetSubmenu, gtk, "gtk_menu_item_get_submenu")
	purego.RegisterLibFunc(&gtkMenuItemSetSubmenu, gtk, "gtk_menu_item_set_submenu")
	purego.RegisterLibFunc(&gtkWidgetShow, gtk, "gtk_widget_show")
	purego.RegisterLibFunc(&gtkWidgetHide, gtk, "gtk_widget_hide")
	purego.RegisterLibFunc(&gtkWidgetSetSensitive, gtk, "gtk_widget_set_sensitive")
	purego.RegisterLibFunc(&gIdleAdd, gtk, "g_idle_add")
	purego.RegisterLibFunc(&gSignalConnectData, gtk, "g_signal_connect_data")
	purego.RegisterLibFunc(&gSignalHandlerBlock, gtk, "g_signal_handler_block")
	purego.RegisterLibFunc(&gSignalHandlerUnblock, gtk, "g_signal_handler_unblock")
	purego.RegisterLibFunc(&gBytesNewStatic, gtk, "g_bytes_new_static")
	purego.RegisterLibFunc(&gBytesGetData, gtk, "g_bytes_get_data")
	purego.RegisterLibFunc(&gBytesUnref, gtk, "g_bytes_unref")

	// AppIndicator functions
	purego.RegisterLibFunc(&appIndicatorNew, ind, "app_indicator_new")
	purego.RegisterLibFunc(&appIndicatorSetStatus, ind, "app_indicator_set_status")
	purego.RegisterLibFunc(&appIndicatorSetMenu, ind, "app_indicator_set_menu")
	purego.RegisterLibFunc(&appIndicatorSetIconFull, ind, "app_indicator_set_icon_full")
	purego.RegisterLibFunc(&appIndicatorSetAttentionIconFull, ind, "app_indicator_set_attention_icon_full")
	purego.RegisterLibFunc(&appIndicatorSetTitle, ind, "app_indicator_set_title")
	purego.RegisterLibFunc(&appIndicatorSetLabel, ind, "app_indicator_set_label")

	guiAvailable = true
}

func findNode(id int) *menuItemNode {
	for _, n := range menuNodes {
		if n.id == id {
			return n
		}
	}
	return nil
}

func registerSystray() {
	if !Available() {
		systrayReady()
		return
	}
	gtkInit(nil, nil)
	globalIndicator = appIndicatorNew("systray", "", appIndicatorCategoryApplicationStatus)
	appIndicatorSetStatus(globalIndicator, appIndicatorStatusActive)
	globalMenu = gtkMenuNew()
	appIndicatorSetMenu(globalIndicator, globalMenu)
	systrayReady()
}

func nativeLoop() {
	if !guiAvailable {
		systrayExit()
		return
	}
	gtkMain()
	systrayExit()
}

func quit() {
	if !guiAvailable {
		return
	}
	gIdleAdd(purego.NewCallback(func(data uintptr) int {
		unlinkTempIcon()
		appIndicatorSetStatus(globalIndicator, appIndicatorStatusPassive)
		gtkMainQuit()
		return 0 // FALSE — don't call again
	}), 0)
}

func unlinkTempIcon() {
	if tempIconPath != "" {
		os.Remove(tempIconPath)
		tempIconPath = ""
	}
}

// SetIcon sets the systray icon.
func SetIcon(iconBytes []byte) {
	if !guiAvailable {
		return
	}
	// Write icon to temp file (AppIndicator requires file path)
	unlinkTempIcon()
	tmpdir := os.TempDir()
	f, err := os.CreateTemp(tmpdir, "systray_")
	if err != nil {
		return
	}
	if _, err := f.Write(iconBytes); err != nil {
		f.Close()
		os.Remove(f.Name())
		return
	}
	f.Close()
	tempIconPath = f.Name()

	appIndicatorSetIconFull(globalIndicator, tempIconPath, "")
	appIndicatorSetAttentionIconFull(globalIndicator, tempIconPath, "")
}

// SetTemplateIcon falls back to regular icon on Linux.
func SetTemplateIcon(templateIconBytes []byte, regularIconBytes []byte) {
	SetIcon(regularIconBytes)
}

// SetTitle sets the systray title.
func SetTitle(title string) {
	if !guiAvailable {
		return
	}
	appIndicatorSetTitle(globalIndicator, title)
	appIndicatorSetLabel(globalIndicator, title, "")
}

// SetTooltip is a no-op on Linux (AppIndicator doesn't support it).
func SetTooltip(tooltip string) {}

func (item *MenuItem) SetIcon(iconBytes []byte) {}

func (item *MenuItem) SetTemplateIcon(templateIconBytes []byte, regularIconBytes []byte) {}

func addOrUpdateMenuItem(item *MenuItem) {
	if !guiAvailable {
		return
	}
	menuItemsMu.Lock()
	defer menuItemsMu.Unlock()

	node := findNode(int(item.id))
	if node != nil {
		// Update existing
		gtkMenuItemSetLabel(node.widget, item.title)
		if item.isCheckable {
			gSignalHandlerBlock(node.widget, node.signalHandlerID)
			active := 0
			if item.checked {
				active = 1
			}
			gtkCheckMenuItemSetActive(node.widget, active)
			gSignalHandlerUnblock(node.widget, node.signalHandlerID)
		}
	} else {
		// Create new
		var widget uintptr
		if item.isCheckable {
			widget = gtkCheckMenuItemNewWithLabel(item.title)
			active := 0
			if item.checked {
				active = 1
			}
			gtkCheckMenuItemSetActive(widget, active)
		} else {
			widget = gtkMenuItemNewWithLabel(item.title)
		}

		// Store menu ID for callback
		idPtr := new(int)
		*idPtr = int(item.id)

		signalID := gSignalConnectData(widget, "activate",
			purego.NewCallback(func(widget uintptr, data uintptr) {
				id := *(*int)(unsafe.Pointer(data))
				systrayMenuItemSelected(uint32(id))
			}),
			uintptr(unsafe.Pointer(idPtr)), 0, 1) // 1 = G_CONNECT_SWAPPED

		parentID := uint32(0)
		if item.parent != nil {
			parentID = item.parent.id
		}

		if parentID == 0 {
			gtkMenuShellAppend(globalMenu, widget)
		} else {
			parentNode := findNode(int(parentID))
			if parentNode != nil {
				submenu := gtkMenuItemGetSubmenu(parentNode.widget)
				if submenu == 0 {
					submenu = gtkMenuNew()
					gtkMenuItemSetSubmenu(parentNode.widget, submenu)
				}
				gtkMenuShellAppend(submenu, widget)
			}
		}

		node = &menuItemNode{
			widget:          widget,
			id:              int(item.id),
			signalHandlerID: signalID,
		}
		menuNodes = append(menuNodes, node)
	}

	sensitive := 1
	if item.disabled {
		sensitive = 0
	}
	gtkWidgetSetSensitive(node.widget, sensitive)
	gtkWidgetShow(node.widget)
}

func addSeparator(id uint32) {
	if !guiAvailable {
		return
	}
	sep := gtkSeparatorMenuItemNew()
	gtkMenuShellAppend(globalMenu, sep)
	gtkWidgetShow(sep)
}

func hideMenuItem(item *MenuItem) {
	if !guiAvailable {
		return
	}
	menuItemsMu.Lock()
	defer menuItemsMu.Unlock()
	node := findNode(int(item.id))
	if node != nil {
		gtkWidgetHide(node.widget)
	}
}

func showMenuItem(item *MenuItem) {
	if !guiAvailable {
		return
	}
	menuItemsMu.Lock()
	defer menuItemsMu.Unlock()
	node := findNode(int(item.id))
	if node != nil {
		gtkWidgetShow(node.widget)
	}
}

func resetSubmenu(item *MenuItem) {
	if !guiAvailable {
		return
	}
	menuItemsMu.Lock()
	defer menuItemsMu.Unlock()
	node := findNode(int(item.id))
	if node != nil {
		newMenu := gtkMenuNew()
		gtkMenuItemSetSubmenu(node.widget, newMenu)
	}
}

// iconBytesToFilePath writes icon bytes to a temp file and returns its path.
func iconBytesToFilePath(iconBytes []byte) (string, error) {
	f, err := os.CreateTemp("", "systray_icon_")
	if err != nil {
		return "", fmt.Errorf("create temp icon: %w", err)
	}
	if _, err := f.Write(iconBytes); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("write temp icon: %w", err)
	}
	f.Close()
	return filepath.Abs(f.Name())
}
