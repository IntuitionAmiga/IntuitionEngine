//go:build !js

package main

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestZ80WasmBrowser_InstantiatesAndAccessesMemory is deliberately a real
// Chromium check, not a wazero substitute: it verifies the browser accepts
// the generated module and that its wasm32 context points at shared memory.
func TestZ80WasmBrowser_InstantiatesAndAccessesMemory(t *testing.T) {
	module, err := z80WasmCompileBlock([]z80WasmInstr{
		{opcode: 0x3E, operand: 0x5A}, // LD A,$5A
		{opcode: 0x47},                // LD B,A
	}, 0x1000)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "z80_wasm_probe.html")
	html := `<!doctype html><meta charset="utf-8"><body>pending</body><script>
const bytes = Uint8Array.from(atob("` + base64.StdEncoding.EncodeToString(module) + `"), c => c.charCodeAt(0));
try {
  const mem = new WebAssembly.Memory({initial: 1});
  const view = new DataView(mem.buffer), ctx = 0x100, cpu = 0x200;
  view.setUint32(ctx + 0, cpu, true);
  const instance = new WebAssembly.Instance(new WebAssembly.Module(bytes), {env: {mem}});
  instance.exports.block(ctx);
  document.body.textContent = view.getUint8(cpu + 0) === 0x5A && view.getUint8(cpu + 2) === 0x5A ? "PASS" : "FAIL:state";
} catch (err) { document.body.textContent = "FAIL:" + err; }
</script>`
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	chrome := findChromeForWasmTest(t)
	profile := filepath.Join(dir, "chrome-profile")
	out, err := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--disable-crash-reporter", "--disable-features=Crashpad", "--user-data-dir="+profile, "--dump-dom", "file://"+path).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "setsockopt: Operation not permitted") || strings.Contains(string(out), "trace/breakpoint trap") {
			if os.Getenv("IE_REQUIRE_Z80_WASM_BROWSER") == "1" {
				t.Fatalf("required browser execution blocked by host sandbox: %s", strings.TrimSpace(string(out)))
			}
			t.Skipf("chrome blocked by host sandbox: %s", strings.TrimSpace(string(out)))
		}
		t.Fatalf("chrome failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("browser did not run Z80 wasm block:\n%s", out)
	}
}
