// gui_frontend_fltk.go - GUI frontend for Intuition Engine using FLTK

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
#cgo CXXFLAGS: -I/usr/include/FL -Ofast -march=native -mtune=native
#cgo LDFLAGS: -lfltk -lstdc++
#cgo CFLAGS: -w -Os -march=native -mtune=native -flto
#include <stdlib.h>
extern void create_window();
extern void show_window();
extern const char* get_selected_file();
extern int get_should_execute();
*/
import "C"
import (
	"fmt"
	"unsafe"
)

type FLTKFrontend struct {
	config    GUIConfig
	cpu       *CPU
	video     *VideoChip
	sound     *SoundChip
	actions   *GUIActions
	lastError error
	visible   bool
}

func NewFLTKFrontend(cpu *CPU, video *VideoChip, sound *SoundChip, psg *PSGPlayer) (GUIFrontend, error) {
	return &FLTKFrontend{
		cpu:     cpu,
		video:   video,
		sound:   sound,
		actions: NewGUIActions(cpu, video, sound, psg),
	}, nil
}

func (f *FLTKFrontend) Initialize(config GUIConfig) error {
	f.config = config
	C.create_window()
	return nil
}

func (f *FLTKFrontend) Close() error {
	f.visible = false
	return nil
}

func (f *FLTKFrontend) IsVisible() bool {
	return f.visible
}

func (f *FLTKFrontend) SendEvent(event GUIEvent) error {
	return nil
}

func (f *FLTKFrontend) UpdateState(state EmulatorState) error {
	return nil
}

func (f *FLTKFrontend) GetLastError() error {
	return f.lastError
}

func (f *FLTKFrontend) Show() error {
	go f.eventLoop()
	C.show_window() // Main thread runs FLTK
	return nil
}

func (f *FLTKFrontend) eventLoop() {
	for {
		shouldExec := C.get_should_execute()
		fmt.Printf("Should execute: %d\n", shouldExec)
		if shouldExec != 0 {
			filename := C.GoString(C.get_selected_file())
			fmt.Printf("Selected file: %s\n", filename)
			if err := f.actions.LoadFile(filename); err != nil {
				fmt.Printf("Error: %v\n", err)
				f.lastError = err
			}
			C.free(unsafe.Pointer(C.get_selected_file()))
		}
	}
}
