package main

import (
	"testing"
	"time"
)

// BenchmarkIEScriptHotLoop measures the compile path with the cache on: a script
// re-run in a hot loop must hit the cache and not re-parse.
func BenchmarkIEScriptHotLoop(b *testing.B) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	se.enableCompileCache()
	const script = `for i = 0, 63 do audio.write_reg(0x5000 + i * 4, i) end`
	if _, _, err := se.compileScript(script, "hot"); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := se.compileScript(script, "hot"); err != nil {
			b.Fatal(err)
		}
	}
}

// TestIEScriptCompileCache_SecondRunSkipsCompile proves the parsed-proto cache
// parses a script once across a validate, a run, and a second identical run.
func TestIEScriptCompileCache_SecondRunSkipsCompile(t *testing.T) {
	t.Setenv("IE_SCRIPT_COMPILE_CACHE", "1")
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())
	if !se.compileCacheOn {
		t.Fatal("compile cache not enabled from env")
	}

	const script = `mem.write8(0x2000, 1)`
	if err := se.RunString(script, "cache"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	waitScriptStopped(t, se)
	first := se.CompileCount()
	if first != 1 {
		t.Fatalf("compiles after first run = %d, want 1 (validate miss, run hit)", first)
	}

	if err := se.RunString(script, "cache"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	waitScriptStopped(t, se)
	if second := se.CompileCount(); second != first {
		t.Fatalf("compiles after second run = %d, want unchanged %d", second, first)
	}
}

// TestIEScriptBulkMemoryAPI_MatchesByteAPI checks that read_block returns exactly
// what a read8 loop would, and write_block writes exactly what a write8 loop
// would, so the bulk API is a pure batching of the byte API.
func TestIEScriptBulkMemoryAPI_MatchesByteAPI(t *testing.T) {
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())

	const script = `
		cpu.freeze()
		-- write_block must equal a write8 loop
		local payload = ""
		for i = 0, 15 do payload = payload .. string.char((i * 7 + 3) % 256) end
		mem.write_block(0x3000, payload)
		for i = 0, 15 do mem.write8(0x3100 + i, (i * 7 + 3) % 256) end
		-- read_block must equal a read8 loop
		local blk = mem.read_block(0x3000, 16)
		local ok = (#blk == 16)
		for i = 0, 15 do
			if string.byte(blk, i + 1) ~= mem.read8(0x3000 + i) then ok = false end
			if mem.read8(0x3000 + i) ~= mem.read8(0x3100 + i) then ok = false end
		end
		if ok then mem.write8(0x4000, 0xAB) end
		cpu.resume()
	`
	if err := se.RunString(script, "bulk"); err != nil {
		t.Fatalf("run: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read8(0x4000); got != 0xAB {
		t.Fatalf("bulk/byte mismatch sentinel = %#x, want 0xAB", got)
	}
}

// TestIEScriptBatchedRegisterAccess_MatchesSequential checks audio.write_regs
// applies the same writes, in the same order, as a sequence of write_reg calls.
func TestIEScriptBatchedRegisterAccess_MatchesSequential(t *testing.T) {
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())

	const script = `
		audio.write_reg(0x5000, 0x11)
		audio.write_reg(0x5004, 0x22)
		audio.write_regs({ {0x5008, 0x33}, {0x500C, 0x44} })
	`
	if err := se.RunString(script, "regs"); err != nil {
		t.Fatalf("run: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	for _, want := range []struct {
		addr uint32
		val  uint32
	}{{0x5000, 0x11}, {0x5004, 0x22}, {0x5008, 0x33}, {0x500C, 0x44}} {
		if got := bus.Read32(want.addr); got != want.val {
			t.Fatalf("reg $%X = %#x, want %#x", want.addr, got, want.val)
		}
	}
}

// TestIEScriptEventWait_WakesOnCondition checks sys.wait_until returns as soon as
// its predicate holds, driven by a machine value flipped from outside the script.
func TestIEScriptEventWait_WakesOnCondition(t *testing.T) {
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())

	// The predicate becomes true only on its fifth evaluation, so wait_until must
	// park across frames and wake when the condition finally holds. Driving the
	// condition from script-internal state keeps the awaited value single-writer,
	// so the check does not race the emulated RAM against the test goroutine.
	const script = `
		cpu.freeze()
		local calls = 0
		local woke = sys.wait_until(function()
			calls = calls + 1
			return calls >= 5
		end, 600)
		if woke and calls >= 5 then mem.write8(0x5002, 0x77) end
		cpu.resume()
	`
	if err := se.RunString(script, "wait"); err != nil {
		t.Fatalf("run: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !se.IsRunning() {
			break
		}
		se.onFrameTiming()
		time.Sleep(5 * time.Millisecond)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read8(0x5002); got != 0x77 {
		t.Fatalf("wait_until did not wake on condition: sentinel = %#x, want 0x77", got)
	}
}

func TestIEScriptEventWait_ZeroBudgetDoesNotWait(t *testing.T) {
	bus := NewMachineBus()
	se := NewScriptEngine(bus, NewVideoCompositor(nil), NewTerminalMMIO())

	const script = `
		cpu.freeze()
		local calls = 0
		local woke = sys.wait_until(function()
			calls = calls + 1
			return false
		end, 0)
		if not woke and calls == 1 then mem.write8(0x5003, 0x78) end
		cpu.resume()
	`
	if err := se.RunString(script, "wait-zero"); err != nil {
		t.Fatalf("run: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err != nil {
		t.Fatalf("script error: %v", err)
	}
	if got := bus.Read8(0x5003); got != 0x78 {
		t.Fatalf("zero-budget wait sentinel = %#x, want 0x78", got)
	}
}

func TestIEScriptEventWait_RejectsNegativeBudget(t *testing.T) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	const script = `sys.wait_until(function() return false end, -1)`
	if err := se.RunString(script, "wait-negative"); err != nil {
		t.Fatalf("run: %v", err)
	}
	waitScriptStopped(t, se)
	if err := se.LastError(); err == nil {
		t.Fatal("negative max_frames did not raise an error")
	}
}

// TestIEScriptCompileCache_KeyedByName checks the cache does not return a proto
// compiled under a different script name for identical source, which would make
// runtime stack traces report the wrong script.
func TestIEScriptCompileCache_KeyedByName(t *testing.T) {
	se := NewScriptEngine(NewMachineBus(), NewVideoCompositor(nil), NewTerminalMMIO())
	se.enableCompileCache()

	const src = `local x = 1`
	a, _, err := se.compileScript(src, "first")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := se.compileScript(src, "second")
	if err != nil {
		t.Fatal(err)
	}
	if se.CompileCount() != 2 {
		t.Fatalf("compiles = %d, want 2 (distinct names must not share a proto)", se.CompileCount())
	}
	if a.SourceName == b.SourceName {
		t.Fatalf("both protos carry name %q; name not in cache key", a.SourceName)
	}
	// Same name and source must still hit.
	if _, parsed, _ := se.compileScript(src, "first"); parsed {
		t.Fatal("same name+source reparsed instead of hitting the cache")
	}
}
