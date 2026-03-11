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
	"syscall"
)

// ExecPath removes the tray icon and replaces the current process with the
// binary at path. Used by autoInstall before the tray starts.
func ExecPath(path string) error {
	return execSyscall(path, os.Args[1:]...)
}

func execSyscall(path string, args ...string) error {
	C.removeTrayIcon()

	argv := append([]string{path}, args...)
	return syscall.Exec(path, argv, os.Environ())
}
