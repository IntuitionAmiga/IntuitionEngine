//gui_frontend_gtk4.go - GUI frontend for the Intuition Engine using GTK4

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

/*
#cgo pkg-config: gtk4
#cgo CFLAGS: -Os -march=native -mtune=native -flto
#include <gtk/gtk.h>
#include <stdlib.h>
extern void gtk_create_window();
extern void gtk_show_window();
extern const char* gtk_get_selected_file();
extern int gtk_get_should_execute();
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
)

// GTKFrontend implements the GUIFrontend interface for GTK4
type GTKFrontend struct {
	config    GUIConfig
	actions   *GUIActions
	cpu       *CPU
	video     *VideoChip
	sound     *SoundChip
	lastError error
	visible   bool
	mutex     sync.Mutex
}

func NewGTKFrontend(cpu *CPU, video *VideoChip, sound *SoundChip) (GUIFrontend, error) {
	frontend := &GTKFrontend{
		actions: NewGUIActions(cpu, video, sound),
		cpu:     cpu,
		video:   video,
		sound:   sound,
	}
	activeFrontend = frontend // Store reference for callbacks
	return frontend, nil
}

func (f *GTKFrontend) Initialize(config GUIConfig) error {
	f.config = config
	C.gtk_create_window()
	return nil
}

func (f *GTKFrontend) Show() error {
	f.mutex.Lock()
	f.visible = true
	f.mutex.Unlock()

	go f.eventLoop()
	C.gtk_show_window()
	return nil
}

func (f *GTKFrontend) eventLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			shouldExec := int(C.gtk_get_should_execute())
			if shouldExec != 0 {
				filename := C.GoString(C.gtk_get_selected_file())
				fmt.Printf("Loading program: %s\n", filename)
				if err := f.actions.LoadProgram(filename); err != nil {
					fmt.Printf("Error loading program: %v\n", err)
					f.lastError = err
				}
			}
		}
	}
}

// Export functions that C code can call
//
//export do_reset
func do_reset() {
	if activeFrontend != nil {
		activeFrontend.actions.Reset()
	}
}

//export do_about
func do_about() *C.char {
	if activeFrontend != nil {
		result := C.CString(activeFrontend.actions.About())
		// Note: The C code must free this string when done
		return result
	}
	return nil
}

//export do_debug
func do_debug() {
	if activeFrontend != nil {
		if err := activeFrontend.actions.Debug(); err != nil {
			fmt.Printf("Debug error: %v\n", err)
		}
	}
}

// Keep one global reference to the active frontend for C callbacks
var activeFrontend *GTKFrontend

func (f *GTKFrontend) Close() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.visible = false
	return nil
}

func (f *GTKFrontend) IsVisible() bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.visible
}

func (f *GTKFrontend) SendEvent(event GUIEvent) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	switch event.Type {
	case EventQuit:
		return f.Close()
	case EventStartEmulation:
		if err := f.actions.LoadProgram(event.Data.(string)); err != nil {
			return fmt.Errorf("start emulation failed: %v", err)
		}
	case EventStopEmulation:
		f.video.Stop()
		f.sound.Stop()
	case EventReset:
		f.actions.Reset()
	}
	return nil
}

func (f *GTKFrontend) UpdateState(state EmulatorState) error {
	return nil
}

func (f *GTKFrontend) GetLastError() error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.lastError
}
