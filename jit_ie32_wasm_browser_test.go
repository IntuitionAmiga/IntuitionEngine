//go:build !js

package main

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// TestIE32WasmBrowser_InstantiatesDirectBlock proves that a browser executes
// representative direct and bounded-loop modules emitted by the IE32 backend.
// The standalone linear-memory context covers the ABI used by direct RAM
// accesses; full Go runtime dispatch remains covered by the js/wasm Node tests.
func TestIE32WasmBrowser_InstantiatesDirectBlock(t *testing.T) {
	block := ie32AnnotateResidentImmediateALU([]ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LDA, AddrMode: ADDR_IMMEDIATE, Operand: 0x1234},
		{PC: PROG_START + INSTRUCTION_SIZE, Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{PC: PROG_START + 2*INSTRUCTION_SIZE, Opcode: XOR, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 3},
		{PC: PROG_START + 3*INSTRUCTION_SIZE, Opcode: MUL, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 5},
	})
	module, err := compileIE32WasmBlock(block)
	if err != nil {
		t.Fatalf("compile IE32 wasm block: %v", err)
	}
	loopStart := uint32(PROG_START + INSTRUCTION_SIZE)
	loop := []ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LOAD, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: 3},
		{PC: loopStart, Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_DIRECT, Operand: 0x200},
		{PC: loopStart + INSTRUCTION_SIZE, Opcode: STORE, Reg: REG_A, AddrMode: ADDR_DIRECT, Operand: 0x204},
		{PC: loopStart + 2*INSTRUCTION_SIZE, Opcode: SUB, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{PC: loopStart + 3*INSTRUCTION_SIZE, Opcode: JNZ, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: loopStart},
	}
	plan := ie32AnalyseCountedLoop(loop)
	if plan == nil {
		t.Fatal("browser loop did not produce a counted-loop plan")
	}
	loopModule, err := compileIE32WasmCountedLoopBlockAtStack(loop, plan, STACK_START)
	if err != nil {
		t.Fatalf("compile IE32 wasm counted loop: %v", err)
	}

	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "ie32_wasm_probe.html")
	html := `<!doctype html>
<meta charset="utf-8">
<body>pending</body>
<script>
const directBytes = Uint8Array.from(atob("` + base64.StdEncoding.EncodeToString(module) + `"), c => c.charCodeAt(0));
const loopBytes = Uint8Array.from(atob("` + base64.StdEncoding.EncodeToString(loopModule) + `"), c => c.charCodeAt(0));
const mem = new WebAssembly.Memory({initial: 1});
try {
  const dv = new DataView(mem.buffer);
  const cpu = 0x1000;
  const ram = 0x4000;
  dv.setUint32(cpu + ` + strconv.FormatUint(uint64(unsafe.Offsetof(CPU{}.memory)), 10) + `, ram, true);
  const direct = new WebAssembly.Instance(new WebAssembly.Module(directBytes), {env: {mem}});
  direct.exports.block(cpu);
  const directValue = dv.getUint32(cpu + ` + strconv.FormatUint(uint64(unsafe.Offsetof(CPU{}.A)), 10) + `, true);
  dv.setUint32(ram + 0x200, 9, true);
  const loop = new WebAssembly.Instance(new WebAssembly.Module(loopBytes), {env: {mem}});
  loop.exports.block(cpu);
  const a = dv.getUint32(cpu + ` + strconv.FormatUint(uint64(unsafe.Offsetof(CPU{}.A)), 10) + `, true);
  const b = dv.getUint32(cpu + ` + strconv.FormatUint(uint64(unsafe.Offsetof(CPU{}.B)), 10) + `, true);
  const stored = dv.getUint32(ram + 0x204, true);
  const pc = dv.getUint32(cpu + ` + strconv.FormatUint(uint64(unsafe.Offsetof(CPU{}.PC)), 10) + `, true);
  document.body.textContent = directValue === 0x5B0E && a === 9 && b === 0 && stored === 9 && pc === ` + strconv.FormatUint(uint64(loopStart+4*INSTRUCTION_SIZE), 10) + ` ? "PASS" : "FAIL: " + JSON.stringify({directValue, a, b, stored, pc});
} catch (err) {
  document.body.textContent = "FAIL:" + err;
}
</script>`
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		t.Fatalf("write browser probe: %v", err)
	}

	chrome := findChromeForWasmTest(t)
	out, err := exec.Command(chrome,
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-crash-reporter",
		"--disable-features=Crashpad",
		"--dump-dom",
		"file://"+htmlPath,
	).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "setsockopt: Operation not permitted") ||
			strings.Contains(string(out), "trace/breakpoint trap") {
			t.Skipf("Chrome blocked by host sandbox: %s", strings.TrimSpace(string(out)))
		}
		t.Fatalf("Chrome failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("browser did not instantiate IE32 wasm block:\n%s", out)
	}
}
