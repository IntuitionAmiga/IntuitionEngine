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

# Ebitengine permits one RunGame per process and invalidates image creation
# after it returns, so each gate needs its own wasm instance. The script
# launches the browser once per test rather than handing it a regexp that would
# match several and leave all but the first failing.
DEFAULT_GATES="TestGPUConvertReadback_MatchesCPU_CLUT8 \
TestGPUConvertLivePath_MatchesCPU_CLUT8 \
TestGPUConvertFailure_FallsBackToCPUExpansion"
if [ "$#" -gt 0 ]; then
	GATES=("$@")
else
	read -r -a GATES <<< "$DEFAULT_GATES"
fi
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
};
WebAssembly.instantiateStreaming(fetch('test.wasm'), go.importObject).then(r => {
  globalThis.__goMem = r.instance.exports.mem;
  return go.run(r.instance);
}).catch(e => { append("ERR:" + e); document.title = "done-err"; });
</script>
HTML

# Background the server itself, not a subshell containing it: $! must be the
# server's own PID or the exit trap leaves it bound to the port and the next
# run talks to a stale server serving a deleted work directory.
python3 -m http.server "$PORT" --directory "$WORK" > "$WORK/server.log" 2>&1 &
SERVER_PID=$!
sleep 1

STATUS=0
for GATE in "${GATES[@]}"; do
	echo "Running $GATE under $CHROME on WebGL..."
	# --virtual-time-budget makes the browser exit once the page is idle, which
	# it otherwise never does headlessly, and --dump-dom gives us the page text
	# without a round trip through the server log. Virtual time distorts wall
	# clock measurements, so this drives correctness gates only.
	OUT="$(timeout 300 "$CHROME" --headless=new --enable-gpu --use-angle=gl --no-sandbox \
		--virtual-time-budget=60000 --dump-dom "http://localhost:$PORT/?run=$GATE" 2>/dev/null || true)"
	# Match only a digit-bearing marker: the page's own script text contains
	# the literal string EXIT: and would otherwise be picked up as a result.
	# The trailing || true matters: under pipefail a grep that matches nothing
	# fails the whole pipeline, and set -e would abort the run on the first
	# gate that produced no marker instead of recording it and carrying on.
	RESULT="$(printf '%s' "$OUT" | grep -o 'EXIT:[0-9][0-9]*' | tail -1 || true)"
	printf '%s\n' "$OUT" | grep -E '^(=== RUN|--- |PASS$|FAIL|ok |gpu gate)' || true
	if [ -z "$RESULT" ]; then
		echo "wasm-gpu-gate: $GATE never reported a result" >&2
		STATUS=1
	elif [ "$RESULT" != "EXIT:0" ]; then
		echo "wasm-gpu-gate: $GATE failed ($RESULT)" >&2
		STATUS=1
	fi
done
exit "$STATUS"
