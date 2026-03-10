//go:build darwin

package services

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

void removeTrayIcon() {
	// No app running (e.g. autoInstall path before tray starts) — nothing to clean up
	if (![NSApp isRunning]) {
		return;
	}

	void (^removeBlock)(void) = ^{
		id delegate = [NSApp delegate];
		if (!delegate) {
			return;
		}

		// systray stores the NSStatusItem as an ivar on the AppDelegate
		NSStatusItem *item = [delegate valueForKey:@"statusItem"];
		if (item) {
			[[NSStatusBar systemStatusBar] removeStatusItem:item];
		}
	};

	if ([NSThread isMainThread]) {
		removeBlock();
	} else {
		dispatch_sync(dispatch_get_main_queue(), removeBlock);
	}
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"syscall"
)

// ExecRestart removes the tray icon and replaces the current process with a
// new instance of the same binary. syscall.Exec preserves PID and terminal.
func ExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	return execSyscall(exe, os.Args[1:]...)
}

// ExecPath removes the tray icon and replaces the current process with the
// binary at path.
func ExecPath(path string) error {
	return execSyscall(path, os.Args[1:]...)
}

func execSyscall(path string, args ...string) error {
	C.removeTrayIcon()

	argv := append([]string{path}, args...)
	return syscall.Exec(path, argv, os.Environ())
}
