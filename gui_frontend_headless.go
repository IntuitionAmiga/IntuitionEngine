//go:build headless

package main

import "fmt"

type HeadlessFrontend struct {
	config    GUIConfig
	actions   *GUIActions
	video     *VideoChip
	lastError error
	visible   bool
}

var activeFrontend *HeadlessFrontend

func NewGTKFrontend(cpu EmulatorCPU, video *VideoChip, sound *SoundChip, psg *PSGPlayer) (GUIFrontend, error) {
	frontend := &HeadlessFrontend{
		actions: NewGUIActions(cpu, video, sound, psg),
		video:   video,
	}
	activeFrontend = frontend
	return frontend, nil
}

func NewFLTKFrontend(cpu EmulatorCPU, video *VideoChip, sound *SoundChip, psg *PSGPlayer) (GUIFrontend, error) {
	frontend := &HeadlessFrontend{
		actions: NewGUIActions(cpu, video, sound, psg),
		video:   video,
	}
	activeFrontend = frontend
	return frontend, nil
}

func (f *HeadlessFrontend) Initialize(config GUIConfig) error {
	f.config = config
	return nil
}

func (f *HeadlessFrontend) Show() error {
	f.visible = true
	return nil
}

func (f *HeadlessFrontend) Close() error {
	f.visible = false
	return nil
}

func (f *HeadlessFrontend) IsVisible() bool {
	return f.visible
}

func (f *HeadlessFrontend) SendEvent(event GUIEvent) error {
	switch event.Type {
	case EventStartEmulation:
		if err := f.actions.LoadProgram(event.Data.(string)); err != nil {
			f.lastError = err
			return fmt.Errorf("start emulation failed: %v", err)
		}
	case EventStopEmulation:
		return nil
	case EventReset:
		f.actions.Reset()
	}
	return nil
}

func (f *HeadlessFrontend) UpdateState(state EmulatorState) error {
	return nil
}

func (f *HeadlessFrontend) GetLastError() error {
	return f.lastError
}
