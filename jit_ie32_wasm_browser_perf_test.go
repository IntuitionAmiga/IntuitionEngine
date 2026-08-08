//go:build !js

package main

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestIE32WasmBrowser_PairedPerformanceHarness compiles the js/wasm test
// binary and runs its paired CPU benchmark in Chromium.  The page publishes
// PASS only after the Go test has timed both Execute modes in the browser.
func TestIE32WasmBrowser_PairedPerformanceHarness(t *testing.T) {
	required := os.Getenv("IE_REQUIRE_IE32_WASM_BROWSER") == "1"
	if runtime.GOARCH != "amd64" {
		if required {
			t.Fatal("the required IE32 browser performance harness needs an amd64 host")
		}
		t.Skip("Chromium performance evidence is collected on the x64 host")
	}
	var chrome string
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			chrome = path
			break
		}
	}
	if chrome == "" {
		if required {
			t.Fatal("the required IE32 browser performance harness needs Chrome or Chromium in PATH")
		}
		t.Skip("no Chrome/Chromium binary found in PATH")
	}
	dir := t.TempDir()
	wasmPath := filepath.Join(dir, "ie32_browser_perf.test.wasm")
	cmd := exec.Command("go", "test", "-c", "-o", wasmPath, "-tags", "novulkan headless", ".")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "GOEXPERIMENT=none")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile browser IE32 performance test: %v\n%s", err, out)
	}
	wasmExec, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		t.Fatalf("read wasm_exec.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), wasmExec, 0o644); err != nil {
		t.Fatalf("write wasm_exec.js: %v", err)
	}
	const page = `<!doctype html><meta charset="utf-8"><body>pending</body>
<script src="/wasm_exec.js"></script><script>
const go = new Go();
go.argv = ["ie32_browser_perf.test", "-test.run", "^TestIE32WasmBrowserPairedPerformance$", "-test.v"];
WebAssembly.instantiateStreaming(fetch("/ie32_browser_perf.test.wasm"), go.importObject).then((result) => {
  globalThis.__goMem = result.instance.exports.mem;
  return go.run(result.instance);
}).catch((err) => { document.body.textContent = "FAIL: " + err; });
</script>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatalf("write browser harness: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for browser harness: %v", err)
	}
	server := &http.Server{Handler: http.FileServer(http.Dir(dir))}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	out, err := exec.Command(chrome,
		"--headless=new", "--disable-gpu", "--no-sandbox", "--disable-crash-reporter",
		"--disable-features=Crashpad", "--virtual-time-budget=30000", "--dump-dom", "http://"+listener.Addr().String(),
	).CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "setsockopt: Operation not permitted") || strings.Contains(string(out), "trace/breakpoint trap") {
			if required {
				t.Fatalf("required browser execution blocked by host sandbox: %s", strings.TrimSpace(string(out)))
			}
			t.Skipf("Chrome blocked by host sandbox: %s", strings.TrimSpace(string(out)))
		}
		t.Fatalf("Chrome paired IE32 performance harness failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS:") {
		t.Fatalf("browser did not complete paired IE32 performance test:\n%s", out)
	}
}
