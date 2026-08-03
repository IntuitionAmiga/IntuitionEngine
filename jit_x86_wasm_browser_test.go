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

func findChromeForWasmTest(t testing.TB) string {
	t.Helper()
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("no Chrome/Chromium binary found in PATH")
	return ""
}

func TestX86WasmBrowser_InstantiatesDirectX87SIMDBlock(t *testing.T) {
	const startPC = uint32(0x1000)
	mem := make([]byte, 0x2000)
	copy(mem[startPC:], []byte{0xD8, 0xC1}) // FADD ST(0),ST(1)
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "x86_wasm_simd_probe.html")
	html := `<!doctype html>
<meta charset="utf-8">
<body>pending</body>
<script>
const bytes = Uint8Array.from(atob("` + base64.StdEncoding.EncodeToString(compiled.module) + `"), c => c.charCodeAt(0));
const mem = new WebAssembly.Memory({initial: 1});
try {
  new WebAssembly.Instance(new WebAssembly.Module(bytes), {env: {mem}});
  document.body.textContent = "PASS";
} catch (err) {
  document.body.textContent = "FAIL:" + err;
}
</script>`
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
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
			t.Skipf("chrome blocked by host sandbox: %s", strings.TrimSpace(string(out)))
		}
		t.Fatalf("chrome failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("browser did not instantiate x86 SIMD block:\n%s", out)
	}
}
