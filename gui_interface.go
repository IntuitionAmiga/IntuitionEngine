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

(c) 2024 - 2026 Zayn Otley
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
	CPUState    EmulatorCPU
	FPS         float64
}

type GUIActions struct {
	cpu   EmulatorCPU
	video *VideoChip
	sound *SoundChip
	psg   *PSGPlayer
}

type EmulatorCPU interface {
	LoadProgram(filename string) error
	Reset()
	Execute()
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

func NewGUIActions(cpu EmulatorCPU, video *VideoChip, sound *SoundChip, psg *PSGPlayer) *GUIActions {
	return &GUIActions{
		cpu:   cpu,
		video: video,
		sound: sound,
		psg:   psg,
	}
}

func (a *GUIActions) LoadProgram(filename string) error {
	// Reset the entire system to a clean state
	a.Reset()

	// Load and start the new program
	if a.cpu == nil {
		return fmt.Errorf("CPU mode is not available in this session")
	}
	if err := a.cpu.LoadProgram(filename); err != nil {
		return fmt.Errorf("failed to load program: %v", err)
	}

	a.video.Start()
	a.sound.Start()
	go a.cpu.Execute()
	return nil
}

func (a *GUIActions) LoadPSG(filename string) error {
	if a.psg == nil {
		return fmt.Errorf("PSG playback is not available")
	}
	if a.cpu != nil {
		return fmt.Errorf("PSG playback is disabled while CPU mode is active")
	}
	a.psg.Stop()
	if err := a.psg.Load(filename); err != nil {
		return err
	}
	a.sound.Start()
	a.psg.Play()
	return nil
}

func (a *GUIActions) LoadFile(filename string) error {
	if isPSGExtension(filename) {
		return a.LoadPSG(filename)
	}
	return a.LoadProgram(filename)
}

func (a *GUIActions) Reset() {
	if a.cpu != nil {
		a.cpu.Reset()
	}
}

func (a *GUIActions) About() string {
	return `Intuition Engine
(c) 2024 - 2026 Zayn Otley

https://github.com/intuitionamiga/IntuitionEngine

A modern 32-bit reimagining of the Commodore, Atari and Sinclair 8-bit home computers.`
}

func (a *GUIActions) Debug() error {
	return fmt.Errorf("debugging not yet implemented")
}

func NewGUIFrontend(backend int, cpu EmulatorCPU, video *VideoChip, sound *SoundChip, psg *PSGPlayer) (GUIFrontend, error) {
	switch backend {
	case GUI_FRONTEND_FLTK:
		return NewFLTKFrontend(cpu, video, sound, psg)
	case GUI_FRONTEND_GTK4:
		return NewGTKFrontend(cpu, video, sound, psg)
	}
	return nil, fmt.Errorf("unknown backend: %d", backend)
}
