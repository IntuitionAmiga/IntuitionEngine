//go:build !headless

package main

import "testing"

func newTestMonitorOverlay(t *testing.T) (*MonitorOverlay, *MachineMonitor) {
	t.Helper()
	bus := NewMachineBus()
	m := NewMachineMonitor(bus)
	o := NewMonitorOverlay(m)
	return o, m
}

func TestMonitorCut_OutputRowsOnly(t *testing.T) {
	o, m := newTestMonitorOverlay(t)
	m.mu.Lock()
	m.inputLine = []byte("HELLO")
	m.cursorPos = 5
	m.mu.Unlock()

	// Select only in output area (rows 1-5)
	o.selActive = true
	o.selAnchorCol = 0
	o.selAnchorRow = 1
	o.selEndCol = 10
	o.selEndRow = 5

	m.mu.Lock()
	o.handleMonitorCut(m)
	got := string(m.inputLine)
	m.mu.Unlock()
	if got != "HELLO" {
		t.Fatalf("expected inputLine unchanged when cut in output rows, got %q", got)
	}
}

func TestMonitorCut_PromptPrefix(t *testing.T) {
	o, m := newTestMonitorOverlay(t)
	m.mu.Lock()
	m.inputLine = []byte("HELLO")
	m.cursorPos = 5
	m.mu.Unlock()

	inputRow := overlayRows - 1
	// Select columns 0-1 on input row (the "> " prompt prefix)
	o.selActive = true
	o.selAnchorCol = 0
	o.selAnchorRow = inputRow
	o.selEndCol = 1
	o.selEndRow = inputRow

	m.mu.Lock()
	o.handleMonitorCut(m)
	got := string(m.inputLine)
	m.mu.Unlock()
	if got != "HELLO" {
		t.Fatalf("expected inputLine unchanged when cut in prompt prefix, got %q", got)
	}
}

func TestMonitorCut_SpanOutputAndInput(t *testing.T) {
	o, m := newTestMonitorOverlay(t)
	m.mu.Lock()
	m.inputLine = []byte("ABCDE")
	m.cursorPos = 5
	m.mu.Unlock()

	inputRow := overlayRows - 1
	// Select from output row 1 to input row col 4 (covers "ABC" of input: cols 2,3,4 = indices 0,1,2)
	o.selActive = true
	o.selAnchorCol = 0
	o.selAnchorRow = 1
	o.selEndCol = 4
	o.selEndRow = inputRow

	m.mu.Lock()
	o.handleMonitorCut(m)
	got := string(m.inputLine)
	pos := m.cursorPos
	m.mu.Unlock()
	if got != "DE" {
		t.Fatalf("expected 'DE' after cutting ABC, got %q", got)
	}
	if pos != 0 {
		t.Fatalf("expected cursorPos 0, got %d", pos)
	}
}

func TestMonitorCut_PastEndOfInput(t *testing.T) {
	o, m := newTestMonitorOverlay(t)
	m.mu.Lock()
	m.inputLine = []byte("HI")
	m.cursorPos = 2
	m.mu.Unlock()

	inputRow := overlayRows - 1
	// Select from col 2 to col 40 on input row (past end of "HI")
	o.selActive = true
	o.selAnchorCol = 2
	o.selAnchorRow = inputRow
	o.selEndCol = 40
	o.selEndRow = inputRow

	m.mu.Lock()
	o.handleMonitorCut(m)
	got := string(m.inputLine)
	pos := m.cursorPos
	m.mu.Unlock()
	if got != "" {
		t.Fatalf("expected empty inputLine after cutting all input, got %q", got)
	}
	if pos != 0 {
		t.Fatalf("expected cursorPos 0, got %d", pos)
	}
}

func TestMonitorSelection_IsInSelection(t *testing.T) {
	o := &MonitorOverlay{}
	o.selActive = true
	o.selAnchorCol = 2
	o.selAnchorRow = 1
	o.selEndCol = 5
	o.selEndRow = 3

	tests := []struct {
		col, row int
		want     bool
	}{
		{0, 0, false}, // above selection
		{2, 1, true},  // anchor
		{5, 1, true},  // first row, col 5
		{0, 2, true},  // middle row, col 0
		{79, 2, true}, // middle row, last col
		{0, 3, true},  // last row, col 0
		{5, 3, true},  // last row, endCol
		{6, 3, false}, // last row, past endCol
		{1, 1, false}, // first row, before startCol
		{0, 4, false}, // below selection
	}
	for _, tc := range tests {
		got := o.isInSelection(tc.col, tc.row)
		if got != tc.want {
			t.Errorf("isInSelection(%d,%d) = %v, want %v", tc.col, tc.row, got, tc.want)
		}
	}
}
