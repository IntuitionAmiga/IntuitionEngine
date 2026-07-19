# Intuition Engine in the Browser (js/wasm)

The `make wasm` target compiles the whole VM to WebAssembly and produces the
website's live demo: an IE64 BASIC machine that boots in a browser
tab, renders through a WebGL canvas, plays audio through WebAudio, takes
keyboard input, and serves a read/write disk volume of demos fetched over
HTTP. It is the same Go source tree as the native build; the browser build is
selected purely by `GOOS=js GOARCH=wasm` and build constraints.

## Building

```bash
make wasm              # build intuitionengine.com/demo/ie.wasm (+ wasm_exec.js)
make wasm-deploy       # build, then netlify deploy --prod on success
make wasm-profile      # profiling build: keeps symbol names for devtools; do not deploy
make test-wasm-build   # build gate: package compiles and links for js/wasm
make web-demos         # rebuild the assets MANIFEST and restage demos when their source tree exists
```

`make x64-live` also rebuilds the wasm demo, so a live-image build never
ships the site a stale `ie.wasm`. Deploying stays a separate, explicit step
(`wasm-deploy`): demo iteration rebuilds constantly and none of those builds
should touch production.

`make wasm` builds with `-tags embed_basic` and `-ldflags "-s -w"`, copies
`wasm_exec.js` from the Go toolchain, and runs `wasm-opt -Oz` when Binaryen is
installed (non-fatal if absent or incompatible). The artefact lands in
`intuitionengine.com/demo/` alongside `index.html`, the loader page.

Run `make x64-live-demos` before `web-demos` when the browser demo set must be
refreshed. If that staged IESHARE `Demos/` tree exists, `web-demos` mirrors it
into `intuitionengine.com/assets/Demos/`. If it is absent, the target warns and
keeps any existing browser demos. It also copies `sdk/examples/basic` and
`sdk/examples/assets` under `assets/sdk/examples/`, then writes a recursive
`assets/MANIFEST` of relative paths. Manifest entries form the disk volume the
browser machine sees.

## Running

Serve `intuitionengine.com/` over HTTP and open `/demo/`. The server must send
`.wasm` files as `application/wasm` because the page uses
`WebAssembly.instantiateStreaming`. The page downloads and compiles `ie.wasm`,
then boots the machine as soon as instantiation finishes; no Launch click is
needed. WebAudio still requires a user gesture before sound may play, and Oto
resumes its AudioContext on the first keypress or click in the machine, so
audio unlocks when the visitor starts typing. The page's
Content-Security-Policy needs
`'wasm-unsafe-eval'` and `blob:` in `script-src` because Oto loads its
AudioWorklet from a blob URL.

When a visitor scrolls within 1200px of the main page's launch section, the
page warms the demo: `wasm_exec.js` is fetched into the browser's HTTP cache,
and `ie.wasm` is fetched and passed to `WebAssembly.compileStreaming`. The
resulting module is not retained or passed to the demo page. This warming step
is intended to populate the browser's HTTP and compiled-code caches before the
demo opens. The demo page fetches `ie.wasm` normally and passes the response
directly to `instantiateStreaming`. Both pages unregister the obsolete demo
service worker where possible. The main page also removes its old CacheStorage
entry. The demo page removes that entry when it finds a service-worker
registration. Cleanup failures do not stop the page from loading. A page that
is still controlled by that worker may use it for one final fetch; the demo
page detects this case and reloads after the cleanup. Cache reuse and the
resulting start-up time remain browser-dependent.

## What is different from native

- **IE64 and M68K have wasm JIT tiers; the other CPUs interpret.** Hot IE64 blocks are
  translated into runtime wasm modules and installed asynchronously while the
  interpreter continues. The tier is enabled by default and is disabled while
  the architectural timer or MMU is active. `IE64_WASM_JIT=0` or
  `/demo/?jit=0` disables it. The complete backend, helper, region, SMC,
  instruction-coverage and diagnostic contract is documented in
  [`IE64_JIT.md`](IE64_JIT.md).
- **M68K uses an ISA-specific wasm runtime.** Supported M68020 integer, memory,
  control-flow and cleanly mappable 68881 blocks compile synchronously into
  small modules. Unsupported instructions fall back one instruction at a time
  with exact resume PC and retired-count accounting. Big-endian accesses,
  effective-address side effects, stack bounds, SMC exits and structured
  counted loops are handled by the M68K backend. `M68K_WASM_JIT=0` disables
  this tier. Extended and packed 68881 formats, transcendental operations and
  FPU memory/control forms remain interpreter fallback. See
  [`M68K_JIT.md`](M68K_JIT.md) and
  [`M68K_JIT_PARITY_MATRIX.md`](M68K_JIT_PARITY_MATRIX.md).
- **Observed promotion records direct invocation results.** For an eligible
  conditional or register-indirect entry, invocation 64 bypasses the chain
  driver and records the resulting successor. Later recording calls also
  invoke the installed function directly rather than using the chain driver.
  The triggered entry is initially compiled as one block, but a later
  successor may already be a static multi-block region. A missing successor
  rejects recording and rebuilds any static fallback. Asynchronous replacement
  checks the invalidation generation and original entry identity, reuses the
  existing function-table slot, and replaces the SMC ranges and PC-cache entry.
- **VBlank remains observable to a parked poll.** The js build holds the VBlank
  status bit readable for a few milliseconds after each set edge
  (`videoVBlankHoldNs`). The compositor can otherwise set and clear VBlank in
  one tick while the CPU goroutine is parked, leaving a WAIT-VSYNC loop unable
  to observe it. `TestWasmJIT_Node_MMIOPollParks` covers the generic MMIO poll
  parking service. It uses a synthetic status register rather than the VBlank
  device path.
- **Constant-only folding also applies to wasm translation.** The shared
  per-block analysis (`jit_ie64_const_fold.go`) precomputes pure integer
  results with fully known inputs over the full integer whitelist, folds
  through plain `LOAD`/`STORE` traffic, and the translator emits results as
  `i64.const` through the normal register-write path. Loop-invariant chains
  are hoisted before the structured `loop` opens. The observed conditional
  keeps its existing structured layout: the cold exit lives in the
  conditional's arm and the hot path continues directly after it.
- **No Vulkan.** The Voodoo uses the software rasteriser; Vulkan files carry
  `!js` build constraints, so the exclusion needs no `-tags novulkan`.
- **Fixed 256 MiB guest RAM.** There is no `/proc/meminfo` and no mmap in the
  browser, so the bus uses a plain heap backing at the EhBASIC profile minimum
  (`ehbasicMinRequiredRAM`).
- **Cooperative yield.** Wasm has one cooperatively scheduled thread and no
  async preemption, so a tight interpreter loop would starve the JS event
  loop: no requestAnimationFrame, no rendering, no keyboard events. The IE64,
  IE32, M68K, Z80 and 6502 interpreter loops call `hostCooperativeYield()`
  (`cpu_yield_wasm.go`) periodically. The IE64 JIT dispatcher checks every 64
  dispatch iterations, since one iteration there can be a whole chained run.
  The browser x86 interpreter does not yet call this hook. On the yielding
  paths, after each guest slice (default 16 ms, one display frame) the CPU
  M68K wasm dispatcher also yields on a tight dispatch cadence. The CPU
  goroutine parks until the browser's next requestAnimationFrame, resuming
  through a zero-delay timeout so the paint happens first; a 50 ms timeout
  races the frame so hidden tabs (where rAF stops) keep executing.
  Fixed-duration sleeps are not used by default: an
  expired Go timer callback often runs before the same turn's rendering
  step, so the guest re-blocks the thread and frames are skipped. The guest
  slice is overridable with `IE_WASM_YIELD_MS` (`/demo/?yield=N`), and
  `IE_WASM_YIELD_SLEEP_MS` (`/demo/?ysleep=N`) forces the legacy fixed-sleep
  mode for A/B measurement. Both values must be whole milliseconds from 1 to
  1000; invalid values use the default.
- **In-memory disk volume.** There is no host filesystem. The FileIO and
  BootstrapHostFS devices run against in-memory stores (`file_io_mem.go`,
  `bootstrap_hostfs_mem.go`). At boot the machine fetches `assets/MANIFEST`
  and registers every path; file contents are fetched lazily over HTTP on
  first read, so large art and music only download when a demo uses them.
  Lookups are case-insensitive and resolve by path suffix, then by base name
  anywhere in the tree, so `RUN "iedoom.ie68"` finds `Demos/m68k/iedoom.ie68`.
  `SAVE` writes back into the in-memory volume for the life of the tab. The
  demo page can also import a visitor's file into that volume and export a
  saved file as a download. One selection accepts at most 64 files, 64 MiB per
  file and 128 MiB in total. The Go bridge also enforces the 64 MiB per-file
  limit.
- **Machine loading reads are indirected.** The Program Executor, machine
  loader, media loader and CPU flat-binary loaders use
  `hostReadFile`/`hostStatExists` (`file_read_native.go`,
  `file_read_wasm.go`): `os.ReadFile` on native, the in-memory volume on wasm.
  `RUN "file.iex"` from the BASIC prompt therefore reboots the machine into
  the file's CPU mode as it does on native. Other runtime components may have
  their own platform-specific file handling.
- **Main-thread rendering.** Ebiten's `RunGame` must run on the programme's
  main goroutine on js. The wasm build reuses the Darwin main-thread pump
  (`mainthread_mainloop.go`, built for `darwin || wasm`): the machine runs in
  goroutines and the main goroutine drives the render loop.
- **Browser host integrations are limited.** The clipboard MMIO device is
  present, but its host clipboard backend reports that the platform is
  unsupported. The normal Ebiten host overlay is compiled into the browser
  build. `HOST` operations that require the external helper or a host process
  are unavailable.

## Guest-visible contract

The ISA, MMIO addresses, BASIC dialect and device register layout remain the
reference surface. The wasm build changes some observable host and device
behaviour as described above, including RAM sizing, VBlank visibility, file
storage and unavailable desktop integrations. Subject to those limits, a
`.bas` or `.ie*` programme that runs on the native VM can run on the browser
VM. Hot IE64 and supported M68K code can use their wasm JITs; the other CPUs
remain interpreted.

## Testing

- Layer A (host): the in-memory volume and wasm RAM-sizing seams compile and
  test natively; run `go test -tags headless -run 'TestWasm' .`.
- Layer B (build gate): `make test-wasm-build` builds the package for js/wasm
  both plain (Vulkan excluded by `!js`) and with `-tags novulkan`.
- Layer C (runtime): `make test-wasm-node` runs the selected `TestWasmJIT_`
  and `TestWasmFileBridge_` js/wasm tests under Node via the repo-local runner
  (`tools/wasm/go_js_wasm_exec`). The runner exposes the module memory as
  `__goMem`, matching the demo page. See [`IE64_JIT.md`](IE64_JIT.md) for the
  wasm JIT test contract and native differential-test command. Wasm-only RAM
  tests are not selected by this target.
  Browser verification: serve the site, open `/demo/` and `/demo/?jit=0`;
  the JIT variant logs `IE64 wasm JIT: first block installed` to the
  console, both boot to the BASIC Ready prompt.

## Diagnostics

- `/demo/?trace=1` sets `IE_TRACE_HOSTIO=1` in the Go environment, tracing
  FileIO, HostFS and media loader activity to the browser console.
- `/demo/?yield=N` overrides the cooperative-yield interval with an integer
  from 1 to 1000 milliseconds.
- `/demo/?ysleep=N` forces fixed-duration cooperative yielding with an `N`
  millisecond sleep for comparison testing, where `N` is from 1 to 1000.
- `/demo/?jitdiag=1` sets `IE64_WASM_JIT_DIAG=1`, publishing wasm JIT state to
  `globalThis.__ieJITDiag` and logging throttled dispatcher diagnostics.
- `make wasm-profile` builds without symbol stripping so the devtools
  Performance tab shows Go function names. It overwrites `ie.wasm`; run
  `make wasm` again before deploying.

## Known limitations

- The wasm artefact is large. A first visit downloads and compiles it. The
  production cache policy requires revalidation on every visit. The browser
  may reuse its cached response when the server reports that the binary has not
  changed.
- Guest CPU speed: IE64 tiers up through the wasm JIT; IE32, M68K, Z80, 6502
  and x86 are interpreter-only in the browser.
- The browser x86 interpreter does not call the cooperative-yield hook. A tight
  x86 workload can therefore block rendering and input until it stops or
  halts.
- The IE64 wasm JIT has a deliberately narrower instruction surface than the
  native backends. Its exact coverage and fallback rules are documented in
  [`IE64_JIT.md`](IE64_JIT.md).
- The disk volume is per-tab and in-memory; `SAVE` does not persist across a
  reload.
- The guest terminal serial stream remains available, but there is no bridge
  to an external serial port. `HOST` command paths that require the external
  helper or spawn host processes are native-only.
