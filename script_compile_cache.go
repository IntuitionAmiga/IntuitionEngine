// script_compile_cache.go - parsed-proto cache for the IEScript engine.
//
// Running a script parses it: validateScript parses to reject a broken script
// before it starts, and the run then parses again inside DoString. A re-run of
// the same automation parses a third and fourth time. Parsing is pure work over
// the source text and its result, a FunctionProto, does not depend on the Lua
// state it will run in, so it can be cached and shared.
//
// The cache is keyed by the exact source text. It is opt-in with
// IE_SCRIPT_COMPILE_CACHE=1 until the parity gate is met, and only ever caches
// scripts that parse cleanly, so a syntax error is reported the same way with
// the cache on or off.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// compileScript returns a runnable proto for the script, parsing it only on a
// cache miss. The bool reports whether a parse actually happened, which the
// tests use to prove a second run skips compilation.
func (se *ScriptEngine) compileScript(script, name string) (*lua.FunctionProto, bool, error) {
	// Gopher-Lua embeds the script name in the compiled proto, so runtime stack
	// traces read it back. The cache key therefore covers the name as well as
	// the source: identical text under two names must not share one proto, or
	// the second run's diagnostics would report the first run's name.
	key := name + "\x00" + script
	if se.compileCacheOn {
		se.compileMu.Lock()
		if proto, ok := se.compileCache[key]; ok {
			se.compileMu.Unlock()
			return proto, false, nil
		}
		se.compileMu.Unlock()
	}

	proto, err := parseScriptProto(script, name)
	if err != nil {
		return nil, true, err
	}
	se.compileCount.Add(1)

	if se.compileCacheOn {
		se.compileMu.Lock()
		se.compileCache[key] = proto
		se.compileMu.Unlock()
	}
	return proto, true, nil
}

// parseScriptProto parses and compiles a script into a state-independent proto.
func parseScriptProto(script, name string) (*lua.FunctionProto, error) {
	chunk, err := parse.Parse(strings.NewReader(script), name)
	if err != nil {
		return nil, err
	}
	return lua.Compile(chunk, name)
}

// runCompiledScript executes a cached proto in the given state, the proto-based
// equivalent of L.DoString.
func runCompiledScript(L *lua.LState, proto *lua.FunctionProto) error {
	fn := L.NewFunctionFromProto(proto)
	L.Push(fn)
	return L.PCall(0, lua.MultRet, nil)
}

// CompileCount reports how many parses the engine has performed, for the cache
// tests.
func (se *ScriptEngine) CompileCount() int64 {
	return se.compileCount.Load()
}

// enableCompileCache turns the cache on outside the env path, for benchmarks and
// tests that construct the engine directly.
func (se *ScriptEngine) enableCompileCache() {
	se.compileCacheOn = true
	if se.compileCache == nil {
		se.compileCache = make(map[string]*lua.FunctionProto)
	}
}
