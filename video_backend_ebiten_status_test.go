//go:build !headless

package main

import "testing"

func TestRuntimeCPUStatusTokensIESPlacementAndState(t *testing.T) {
	tokens := runtimeCPUStatusTokens(runtimeStatusSnapshot{})
	iesIdx := statusTokenIndex(tokens, "IES")
	if iesIdx < 0 {
		t.Fatal("CPU status tokens missing IES")
	}
	if iesIdx < 2 || tokens[iesIdx-2].name != "6502" || tokens[iesIdx-1].name != "|" {
		t.Fatalf("IES placement = tokens[%d], want after 6502 separator; tokens=%v", iesIdx, statusTokenNames(tokens))
	}
	if tokens[iesIdx].enabled {
		t.Fatal("IES enabled without script engine")
	}

	idleScript := &ScriptEngine{}
	tokens = runtimeCPUStatusTokens(runtimeStatusSnapshot{scriptEngine: idleScript})
	iesIdx = statusTokenIndex(tokens, "IES")
	if tokens[iesIdx].enabled {
		t.Fatal("IES enabled while script engine is idle")
	}

	runningScript := &ScriptEngine{}
	runningScript.running.Store(true)
	tokens = runtimeCPUStatusTokens(runtimeStatusSnapshot{scriptEngine: runningScript})
	iesIdx = statusTokenIndex(tokens, "IES")
	if !tokens[iesIdx].enabled {
		t.Fatal("IES disabled while script engine is running")
	}
}

func statusTokenIndex(tokens []statusToken, name string) int {
	for i, token := range tokens {
		if token.name == name {
			return i
		}
	}
	return -1
}

func statusTokenNames(tokens []statusToken) []string {
	names := make([]string, len(tokens))
	for i, token := range tokens {
		names[i] = token.name
	}
	return names
}
