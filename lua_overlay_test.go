//go:build !headless

package main

import (
	"context"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestOverlay_ResetState_PanicSafe(t *testing.T) {
	oldHook := luaOverlayRegisterModules
	t.Cleanup(func() { luaOverlayRegisterModules = oldHook })

	luaOverlayRegisterModules = func(se *ScriptEngine, L *lua.LState, ctx context.Context) {
		panic("register boom")
	}
	o := &LuaOverlay{scriptEngine: NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())}
	o.resetState()
	if o.L != nil {
		t.Fatal("LState should be nil after reset panic")
	}
	if len(o.output) == 0 || !strings.Contains(o.output[len(o.output)-1], "register boom") {
		t.Fatalf("missing reset panic output: %#v", o.output)
	}

	luaOverlayRegisterModules = oldHook
	o.resetState()
	if o.L == nil {
		t.Fatal("reset should recover on next attempt")
	}
	o.L.Close()
	o.L = nil
}
