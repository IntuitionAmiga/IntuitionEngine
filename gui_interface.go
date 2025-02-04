// gui_interface.go - GUI interface abstraction/glue for the Intuition Engine.

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

import (
	"fmt"
)

type GUIConfig struct {
	Width     int
	Height    int
	Title     string
	Resizable bool
	Theme     string
}

type GUIEventType int

const (
	EventQuit GUIEventType = iota
	EventLoadProgram
	EventStartEmulation
	EventStopEmulation
	EventReset
	EventKeyPress
	EventKeyRelease
)

type GUIEvent struct {
	Type GUIEventType
	Data interface{}
}

type EmulatorState struct {
	Running     bool
	ProgramPath string
	CPUState    *CPU
	FPS         float64
}

type GUIActions struct {
	cpu   *CPU
	video *VideoChip
	sound *SoundChip
}

type GUIFrontend interface {
	Initialize(config GUIConfig) error
	Show() error
	Close() error
	IsVisible() bool

	SendEvent(event GUIEvent) error
	UpdateState(state EmulatorState) error
	GetLastError() error
}

const (
	GUI_FRONTEND_FLTK = iota
	GUI_FRONTEND_GTK4
)

func NewGUIActions(cpu *CPU, video *VideoChip, sound *SoundChip) *GUIActions {
	return &GUIActions{
		cpu:   cpu,
		video: video,
		sound: sound,
	}
}

func (a *GUIActions) LoadProgram(filename string) error {
	// Reset the entire system to a clean state
	a.Reset()

	// Load and start the new program
	if err := a.cpu.LoadProgram(filename); err != nil {
		return fmt.Errorf("failed to load program: %v", err)
	}

	a.video.Start()
	a.sound.Start()
	go a.cpu.Execute()
	return nil
}

func (a *GUIActions) Reset() {
	a.cpu.Reset()
}

func (a *GUIActions) About() string {
	return `Intuition Engine
(c) 2024 - 2025 Zayn Otley

https://github.com/intuitionamiga/IntuitionEngine

A modern 32-bit reimagining of the Commodore, Atari and Sinclair 8-bit home computers.`
}

func (a *GUIActions) Debug() error {
	return fmt.Errorf("debugging not yet implemented")
}

func NewGUIFrontend(backend int, cpu *CPU, video *VideoChip, sound *SoundChip) (GUIFrontend, error) {
	switch backend {
	case GUI_FRONTEND_FLTK:
		return NewFLTKFrontend(cpu, video, sound)
	case GUI_FRONTEND_GTK4:
		return NewGTKFrontend(cpu, video, sound)
	}
	return nil, fmt.Errorf("unknown backend: %d", backend)
}
