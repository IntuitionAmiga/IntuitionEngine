package main

import "testing"

type recordingAudioOutput struct{ starts, stops, closes int }

func (o *recordingAudioOutput) Start()          { o.starts++ }
func (o *recordingAudioOutput) Stop()           { o.stops++ }
func (o *recordingAudioOutput) Close()          { o.closes++ }
func (o *recordingAudioOutput) IsStarted() bool { return o.starts > o.stops }

func TestSoundChipStopClosesOutputBeforePlaybackStarts(t *testing.T) {
	out := &recordingAudioOutput{}
	chip := &SoundChip{output: out}
	chip.Stop()
	if out.stops != 0 || out.closes != 1 {
		t.Fatalf("never-started output lifecycle stop=%d close=%d, want 0,1", out.stops, out.closes)
	}
	chip.Stop()
	if out.closes != 2 {
		t.Fatalf("second Stop must keep closing terminal outputs, got %d closes", out.closes)
	}
}

func TestSoundChipStopStopsAndClosesStartedOutput(t *testing.T) {
	out := &recordingAudioOutput{}
	chip := &SoundChip{output: out}
	chip.enabled.Store(true)
	chip.Stop()
	if out.stops != 1 || out.closes != 1 {
		t.Fatalf("started output lifecycle stop=%d close=%d, want 1,1", out.stops, out.closes)
	}
}
