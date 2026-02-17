//go:build !headless

package main

import (
	"os/exec"
	"sync"

	"golang.design/x/clipboard"
)

var (
	termClipOnce sync.Once
	termClipOK   bool
)

// readPrimarySelection reads the X11 PRIMARY selection via xsel or xclip.
// PRIMARY is what gets populated when you highlight text in another app.
func readPrimarySelection() []byte {
	if out, err := exec.Command("xsel", "-p", "-o").Output(); err == nil && len(out) > 0 {
		return out
	}
	if out, err := exec.Command("xclip", "-selection", "primary", "-o").Output(); err == nil && len(out) > 0 {
		return out
	}
	return nil
}

func initTerminalClipboard(vt *VideoTerminal) {
	vt.SetClipboardHandlers(
		func(data []byte) {
			termClipOnce.Do(func() { termClipOK = clipboard.Init() == nil })
			if termClipOK {
				clipboard.Write(clipboard.FmtText, data)
			}
		},
		func() []byte {
			// Try X11 PRIMARY selection first (text highlighted by mouse in other apps)
			if primary := readPrimarySelection(); len(primary) > 0 {
				return primary
			}
			// Fall back to CLIPBOARD (text copied via Ctrl+C)
			termClipOnce.Do(func() { termClipOK = clipboard.Init() == nil })
			if !termClipOK {
				return nil
			}
			return clipboard.Read(clipboard.FmtText)
		},
	)
}
