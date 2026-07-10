# Intuition Engine in the Browser (js/wasm)

The `make wasm` target compiles the whole VM to WebAssembly and produces the
website's live demo: an interpreter-only IE64 BASIC that boots in a browser
tab, renders through a WebGL canvas, plays audio through WebAudio, takes
keyboard input, and serves a read/write disk volume of demos fetched over
HTTP. It is the same Go source tree as the native build; the browser build is
selected purely by `GOOS=js GOARCH=wasm` and build constraints.

## Building

```bash
make wasm              # build intuitionengine.com/demo/ie.wasm (+ wasm_exec.js)
make wasm-profile      # profiling build: keeps symbol names for devtools; do not deploy
make test-wasm-build   # build gate: package compiles and links for js/wasm
make web-demos         # restage the assets disk volume and its MANIFEST
```

`make wasm` builds with `-tags embed_basic` and `-ldflags "-s -w"`, copies
`wasm_exec.js` from the Go toolchain, and runs `wasm-opt -Oz` when Binaryen is
installed (non-fatal if absent or incompatible). The artefact lands in
`intuitionengine.com/demo/` alongside `index.html`, the loader page.

`web-demos` mirrors the IESHARE `Demos/` tree (built by `make x64-live-demos`)
into `intuitionengine.com/assets/Demos/`, copies `sdk/examples/basic` and
`sdk/examples/assets` under `assets/sdk/examples/`, and writes a recursive
`assets/MANIFEST` of relative paths. That assets folder is the disk volume the
browser machine sees.

## Running

Serve `intuitionengine.com/` over HTTP (any static server) and open `/demo/`.
The page downloads and compiles `ie.wasm`, then boots the machine as soon as
instantiation finishes; no Launch click is needed. WebAudio still requires a
user gesture before sound may play, and Oto resumes its AudioContext on the
first keypress or click in the machine, so audio unlocks the moment the
visitor starts typing. The page's Content-Security-Policy needs
`'wasm-unsafe-eval'` and `blob:` in `script-src` because Oto loads its
AudioWorklet from a blob URL.

When a visitor scrolls within 1200px of the main page's launch section, the
page warms the demo: `wasm_exec.js` is fetched into the browser's HTTP cache,
and `ie.wasm` goes through `WebAssembly.compileStreaming`, which both commits
the download to the HTTP cache (a real fetch, not a droppable `rel=prefetch`
hint) and gives the engine the chance to seed its URL-keyed compiled-wasm
code cache while the visitor reads. The compiled Module is discarded; it
cannot cross a navigation, but the caches can. The demo page's own fetch is
then a cheap ETag revalidation, and its Response is handed to
`instantiateStreaming` untouched, deliberately NOT served through a service
worker or wrapped in a constructed Response: Chromium's compiled-wasm code
cache only attaches to responses that come from the network stack's HTTP
cache, so either indirection forces a full recompile on every load. Cold
visits pay the wasm compile; warmed and repeat visits skip most of it. The
code cache is written in the background, so two rapid reloads may both
compile, and DevTools' "Disable cache" defeats it entirely.

## What is different from native

- **IE64 has a wasm JIT tier; the other CPUs interpret.** The browser gives
  Go no executable memory, so the native JIT backends are absent and
  `jitAvailable` is false. IE64 instead carries a wasm bytecode backend: the
  dispatcher (`jit_exec_wasm.go`) counts visits to block-start PCs, and hot
  blocks are translated to WebAssembly (`jit_wasm_ie64_emit.go`, using the
  same scanner and JITContext protocol as the native backends) and handed to
  the browser's own engine through `WebAssembly.instantiate`. Compilation is
  asynchronous; the interpreter keeps running and the block installs at the
  next cooperative yield. Generated modules import the machine's own linear
  memory (the page exposes it as `__goMem`), so they mutate CPU state in
  place. MMIO accesses, stack faults and anything outside the supported
  instruction set exit to the Go dispatcher through the helper protocol.
  Block-to-block dispatch chains inside wasm through a driver module and a
  shared function table (about 5.3 times interpreter speed on the node
  hot-loop benchmark). While the MMU is enabled, blocks are neither
  compiled nor entered. Stores into compiled code pages are detected in
  generated code and flush the block cache. The backend is on by default and `IE64_WASM_JIT=0` (or
  `/demo/?jit=0`) disables it. Runtime-generated modules are small and are
  never cached by the browser; only the main `ie.wasm` participates in the
  compiled-wasm code cache. The heavy pixel work in the showcase demos
  (blitter, copper, Mode 7) is compiled Go and runs at full speed
  regardless.
- **No Vulkan.** The Voodoo uses the software rasteriser; Vulkan files carry
  `!js` build constraints, so the exclusion needs no `-tags novulkan`.
- **Fixed 256 MiB guest RAM.** There is no `/proc/meminfo` and no mmap in the
  browser, so the bus uses a plain heap backing at the EhBASIC profile minimum
  (`ehbasicMinRequiredRAM`).
- **Cooperative yield.** Wasm has one cooperatively scheduled thread and no
  async preemption, so a tight interpreter loop would starve the JS event
  loop: no requestAnimationFrame, no rendering, no keyboard events. Every CPU
  interpreter loop calls `hostCooperativeYield()` (`cpu_yield_wasm.go`)
  periodically (the JIT dispatcher every 64 dispatch iterations, since one
  iteration there can be a whole chained run). After each guest slice
  (default 6 ms) the CPU goroutine parks until the browser's next
  requestAnimationFrame, resuming through a zero-delay timeout so the paint
  happens first; a 50 ms timeout races the frame so hidden tabs (where rAF
  stops) keep executing. Fixed-duration sleeps are not used by default: an
  expired Go timer callback often runs before the same turn's rendering
  step, so the guest re-blocks the thread and frames are skipped. The guest
  slice is overridable with `IE_WASM_YIELD_MS` (`/demo/?yield=N`), and
  `IE_WASM_YIELD_SLEEP_MS` (`/demo/?ysleep=N`) forces the legacy fixed-sleep
  mode for A/B measurement.
- **In-memory disk volume.** There is no host filesystem. The FileIO and
  BootstrapHostFS devices run against in-memory stores (`file_io_mem.go`,
  `bootstrap_hostfs_mem.go`). At boot the machine fetches `assets/MANIFEST`
  and registers every path; file contents are fetched lazily over HTTP on
  first read, so large art and music only download when a demo uses them.
  Lookups are case-insensitive and resolve by path suffix, then by base name
  anywhere in the tree, so `RUN "iedoom.ie68"` finds `Demos/m68k/iedoom.ie68`.
  `SAVE` writes back into the in-memory volume for the life of the tab.
- **Host reads are indirected.** Every "load a file by path" site (the
  Program Executor, machine loader, media loader, CPU flat-binary loaders)
  goes through `hostReadFile`/`hostStatExists` (`file_read_native.go`,
  `file_read_wasm.go`): `os.ReadFile` on native, the in-memory volume on
  wasm. `RUN "file.iex"` from the BASIC prompt therefore reboots the machine
  into the file's CPU mode exactly as on native.
- **Main-thread rendering.** Ebiten's `RunGame` must run on the programme's
  main goroutine on js. The wasm build reuses the Darwin main-thread pump
  (`mainthread_mainloop.go`, built for `darwin || wasm`): the machine runs in
  goroutines and the main goroutine drives the render loop.
- **Desktop-only host integrations are stubbed.** Clipboard, host overlay and
  similar host code follow the headless stubs on wasm.

## Guest-visible contract

Unchanged. The MMIO map, ISA, BASIC dialect and device registers are the
reference surface; only host backends differ. A `.bas` or `.ie*` programme
that runs on the native VM runs on the browser VM within the limits above
(IE64 code reaches wasm-JIT speed once hot; the other CPUs run at interpreter
speed).

## Testing

- Layer A (host): the in-memory volume and wasm RAM-sizing seams compile and
  test natively; run `go test -tags headless -run 'TestWasm' .`.
- Layer B (build gate): `make test-wasm-build` builds the package for js/wasm
  both plain (Vulkan excluded by `!js`) and with `-tags novulkan`.
- Layer C (runtime): `make test-wasm-node` runs the js/wasm test suite under
  Node via the repo-local runner (`tools/wasm/go_js_wasm_exec`), which
  exposes the module memory as `__goMem` exactly like the demo page. It
  covers the JITContext ABI guard, end-to-end interpreter-versus-JIT parity
  on a hot loop, both halves of the MMU gate and the kill switch. The wasm
  JIT's differential suite (generated blocks executed under wazero against
  the interpreter) runs natively:
  `go test -tags headless -run 'TestWasmJIT_|TestWasmEnc_' .`
  Browser verification: serve the site, open `/demo/` and `/demo/?jit=0`;
  the JIT variant logs `IE64 wasm JIT: first block installed` to the
  console, both boot to the BASIC Ready prompt.

## Diagnostics

- `/demo/?trace=1` sets `IE_TRACE_HOSTIO=1` in the Go environment, tracing
  FileIO, HostFS and media loader activity to the browser console.
- `/demo/?yield=N` overrides the cooperative-yield interval in milliseconds.
- `make wasm-profile` builds without symbol stripping so the devtools
  Performance tab shows Go function names. It overwrites `ie.wasm`; run
  `make wasm` again before deploying.

## Known limitations

- 24 MB artefact (after `wasm-opt`), roughly 6 MB over the wire once the CDN
  compresses it; first visit pays the download, and repeat visits revalidate
  by ETag and reuse the browser's cached, already-compiled copy.
- Guest CPU speed: IE64 tiers up through the wasm JIT; IE32, M68K, Z80, 6502
  and x86 are interpreter-only in the browser.
- The wasm JIT covers the MMU-off integer core (ALU, load/store, branches,
  subroutine and stack operations) and FP64 (arithmetic, moves, compares,
  converts, DINT rounding modes, sticky exception flags and condition
  codes). FP64 transcendentals (DSIN and friends), FP32, MMU-on execution
  and the remaining system opcodes stay on the interpreter; blocks
  containing them are simply not compiled past that point.
- The disk volume is per-tab and in-memory; `SAVE` does not persist across a
  reload.
- No serial, no host shell (`HOST` command paths that spawn processes are
  native-only).
