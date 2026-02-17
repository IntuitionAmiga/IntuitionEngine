//go:build !headless

package main

import (
	"strings"
	"testing"
)

func TestLuaOverlay_ActivateDeactivate(t *testing.T) {
	o := NewLuaOverlay(nil)
	if o.IsActive() {
		t.Fatalf("overlay should start inactive")
	}
	o.Toggle()
	if !o.IsActive() {
		t.Fatalf("overlay should be active after toggle")
	}
	o.Toggle()
	if o.IsActive() {
		t.Fatalf("overlay should be inactive after second toggle")
	}
}

func TestLuaOverlay_ExecuteLine(t *testing.T) {
	o := NewLuaOverlay(nil)
	o.submitInputLine("return 2+2")
	found := false
	for _, line := range o.output {
		if strings.Contains(line, "4") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected REPL output to contain 4, got: %v", o.output)
	}
}

func TestLuaOverlay_MultilineDetection(t *testing.T) {
	o := NewLuaOverlay(nil)
	o.submitInputLine("function foo()")
	if !o.multiline {
		t.Fatalf("expected multiline mode after incomplete function declaration")
	}
	o.submitInputLine("return 1")
	if !o.multiline {
		t.Fatalf("expected multiline mode to continue before end")
	}
	o.submitInputLine("end")
	if o.multiline {
		t.Fatalf("expected multiline mode to end after complete chunk")
	}
}

func TestLuaOverlay_History(t *testing.T) {
	o := NewLuaOverlay(nil)
	o.submitInputLine("a=1")
	o.submitInputLine("a=2")
	if len(o.history) != 2 {
		t.Fatalf("history length=%d, want 2", len(o.history))
	}
	if o.history[0] != "a=1" || o.history[1] != "a=2" {
		t.Fatalf("unexpected history contents: %v", o.history)
	}
}
