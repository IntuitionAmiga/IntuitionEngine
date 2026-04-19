//go:build !headless

package main

import (
	"os/exec"
	"sync"

	"github.com/intuitionamiga/IntuitionEngine/internal/clipboard"
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
				_ = clipboard.WriteText(data)
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
			data, _ := clipboard.ReadText()
			return data
		},
	)
}
