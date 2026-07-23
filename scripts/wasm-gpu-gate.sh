#!/usr/bin/env bash
# wasm-gpu-gate.sh - run the GPU conversion gates against WebGL in a browser.
#
# (c) 2024 - 2026 Zayn Otley. GPLv3 or later.
#
# The js/wasm build ships the same Ebiten backend and Kage shaders as the
# desktop build, so the conversion gates are meaningful there, but neither the
# native runner nor the Node harness can run them: the native runner re-executes
# a process, and Node has no WebGL. This builds a non-headless js/wasm test
# binary, serves it, and drives it with headless Chrome on a real GPU.
#
# Usage: scripts/wasm-gpu-gate.sh [test regexp]

set -euo pipefail

RUN="${1:-TestGPUConvert}"
PORT="${IE_WASM_GATE_PORT:-8731}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"; [ -n "${SERVER_PID:-}" ] && kill "$SERVER_PID" 2>/dev/null || true' EXIT

CHROME=""
for candidate in chromium google-chrome-stable chromium-browser google-chrome; do
	if command -v "$candidate" >/dev/null 2>&1; then
		CHROME="$candidate"
		break
	fi
done
if [ -z "$CHROME" ]; then
	echo "wasm-gpu-gate: no Chrome or Chromium found; skipping" >&2
	exit 0
fi

echo "Building the js/wasm test binary (Ebiten included, so no headless tag)..."
(cd "$ROOT" && GOOS=js GOARCH=wasm GOEXPERIMENT=none go test -c -tags "novulkan embed_basic" -o "$WORK/test.wasm" .)
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$WORK/wasm_exec.js"

cat > "$WORK/index.html" <<'HTML'
<!doctype html><meta charset="utf-8"><title>running</title>
<pre id="out"></pre>
<script src="wasm_exec.js"></script>
<script>
const out = document.getElementById('out');
const log = console.log.bind(console);
const append = s => { out.textContent += s + "\n"; };
console.log = (...a) => { append(a.join(' ')); log(...a); };
console.error = (...a) => { append(a.join(' ')); log(...a); };
const go = new Go();
const params = new URLSearchParams(location.search);
go.argv = ["test.wasm", "-test.run=" + (params.get('run') || 'TestGPUConvert'), "-test.v"];
go.env = { IE_GPU_BENCH: params.get('bench') || '' };
go.exit = code => {
  append("EXIT:" + code);
  document.title = "done-" + code;
  fetch('/result?code=' + code + '&data=' + encodeURIComponent(out.textContent.slice(-4000)));
};
WebAssembly.instantiateStreaming(fetch('test.wasm'), go.importObject).then(r => {
  globalThis.__goMem = r.instance.exports.mem;
  return go.run(r.instance);
}).catch(e => { append("ERR:" + e); document.title = "done-err"; });
</script>
HTML

(cd "$WORK" && python3 -m http.server "$PORT" > "$WORK/server.log" 2>&1 &)
SERVER_PID=$!
sleep 1

echo "Running $RUN under $CHROME on WebGL..."
timeout 300 "$CHROME" --headless=new --enable-gpu --use-angle=gl --no-sandbox \
	--disable-frame-rate-limit "http://localhost:$PORT/?run=$RUN" >/dev/null 2>&1 || true

RESULT="$(grep -o 'result?code=[^ ]*' "$WORK/server.log" | tail -1 || true)"
if [ -z "$RESULT" ]; then
	echo "wasm-gpu-gate: the page never reported a result" >&2
	exit 1
fi
python3 - "$RESULT" <<'PY'
import sys, urllib.parse
q = urllib.parse.parse_qs(sys.argv[1].split('?', 1)[1])
print(urllib.parse.unquote(q.get('data', [''])[0]))
sys.exit(int(q.get('code', ['1'])[0]))
PY
