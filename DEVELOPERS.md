# Intuition Engine Developer Guide

This guide is the starting point for building, testing and contributing to
Intuition Engine. For the project overview, see [README.md](README.md). The
canonical architecture, instruction-set and JIT references are under
[sdk/docs](sdk/docs/).

Intuition Engine is a multi-CPU fantasy computer implemented primarily in the
root Go `main` package. It supports IE64, IE32, M68K, Z80, 6502 and x86 guest
CPUs. Most hardware is shared through one guest bus and exposed through native
MMIO or an architecture-specific I/O window. CLI and boot-profile routing select
the initial CPU mode. The Program Executor handles later guest-requested
launches, including BASIC `RUN`, while the Coprocessor Manager can run additional
CPU workers concurrently on the same MachineBus.

## 1. Prerequisites

Requires [Go 1.27rc2 or later](https://go.dev/dl/). The default x64 and Linux
ARM64 builds enable the experimental `simd/archsimd` API.

The default native build uses Ebiten, Oto and the Vulkan-backed Voodoo renderer.
It therefore needs the normal native compiler, window-system, audio and Vulkan
development dependencies for the host. If the Vulkan SDK is unavailable, use
`make novulkan`. For CI or a display-free build, use `make headless` or
`make headless-novulkan`.

On Ubuntu or Debian, the native dependencies used by the Linux CI build can be
installed with:

```bash
sudo apt-get update
sudo apt-get install -y build-essential git make \
    libasound2-dev libgl1-mesa-dev libxcursor-dev libxi-dev \
    libxinerama-dev libxrandr-dev libxxf86vm-dev liblhasa-dev
```

The default Vulkan build also needs the
[LunarG Vulkan SDK](https://vulkan.lunarg.com/). It is not required by the
`novulkan` or headless profiles.

Some guest toolchains are optional:

| Guest | External toolchain |
|-------|--------------------|
| M68K | [VASM](http://sun.hasenbraten.de/vasm/) using Motorola syntax |
| Z80 | [VASM](http://sun.hasenbraten.de/vasm/) using standard Z80 syntax |
| 6502 | [cc65](https://cc65.github.io/) (`ca65` and `ld65`) |
| x86 | [NASM](https://www.nasm.us/) |

Individual examples and OS rebuilds may require additional cross-compilers.
Their Makefile targets report missing prerequisites.

## 2. Building

```bash
# VM and all first-party SDK tools
make

# VM only
make intuition-engine

# VM without the Vulkan backend
make novulkan

# CI build without display, audio or Vulkan backends
make headless

# Headless build without Vulkan
make headless-novulkan

# Browser build
make wasm

# List all supported targets
make help
```

### Cleaning the worktree

The cleaning targets are not routine build commands. `make clean` removes build
outputs and the tracked `sdk/examples/prebuilt/` demo images. Restore those
tracked images with `git restore sdk/examples/prebuilt` if they contained no
intentional changes, or rebuild the relevant examples before continuing.

`make distclean` also removes checked-in generated IntuitionOS assets,
downloaded test fixtures and the AROS build tree. Use it only when deliberately
rebuilding all of those inputs from scratch.

The VM is written to `bin/IntuitionEngine`. First-party tools are written to
`sdk/bin/`:

| Tool | Purpose |
|------|---------|
| `ie32asm` | IE32 assembler |
| `ie64asm` | IE64 assembler |
| `ie64dis` | IE64 disassembler |
| `ie32to64` | IE32-to-IE64 converter |
| `m68kto64` | M68K-to-IE64 transpiler |
| `ie64-cproc` | IE64 freestanding C compiler driver |
| `ie64ld` | IE64 static linker |
| `ie64-ar` | IE64 archive tool |
| `ie64-ranlib` | IE64 archive indexer |

The [Host SDK guide](sdk/docs/host-sdk-README.md) describes the packaged Linux
x86-64, ARM64 and Windows x86-64 SDKs, including their compiler components,
runtime, libraries and public headers. Download the [Linux x86-64 archive](https://intuitionengine.io/assets/intuition-engine-host-sdk-linux-amd64.tar.xz),
[Linux ARM64 archive](https://intuitionengine.io/assets/intuition-engine-host-sdk-linux-arm64.tar.xz)
or [Windows x86-64 archive](https://intuitionengine.io/assets/intuition-engine-host-sdk-windows-amd64.zip).
They contain the IE32 and IE64 development tools, not the VM or the external
M68K, Z80, 6502 and x86 toolchains listed above. Build the corresponding
source-tree packages with `make dist-host-sdk-linux-amd64`,
`make dist-host-sdk-linux-arm64` and `make dist-host-sdk-windows-amd64`.

### Makefile build baseline

Makefile-driven builds export `GOEXPERIMENT=simd`. On x64 they also export
`GOAMD64=v3`, so those binaries require an x86-64-v3-compatible processor.
`IE_SIMD=0` selects scalar kernels at runtime but does not lower that processor
baseline.

A bare `go build .` does not inherit `GOAMD64=v3`. It compiles scalar kernels
unless `GOEXPERIMENT=simd` has been configured in the Go environment. Linux
ARM64 SIMD is enabled by the same experiment setting. Use the Makefile targets
for release artefacts and performance measurements.

### Build profiles

| Profile | Command | Host backends |
|---------|---------|---------------|
| Default | `make` | Ebiten, Oto and Vulkan Voodoo |
| No Vulkan | `make novulkan` | Ebiten, Oto and software Voodoo |
| Headless | `make headless` | Display and audio stubs; the headless constraints exclude Vulkan |
| Headless CI | `make headless-novulkan` | The same guest model with explicit `headless novulkan` tags and native JITs |
| Browser | `make wasm` | WebGL, WebAudio, software Voodoo and WebAssembly JIT backends |

Build tags change host backends, not the guest ISA or MMIO contracts.

| Tag | Effect |
|-----|--------|
| `headless` | Replace GUI, audio and video presentation backends with test stubs |
| `novulkan` | Use the software Voodoo renderer |
| `goexperiment.simd` | Compile x64 and Linux ARM64 SIMD kernels when the Go experiment is enabled |
| `embed_basic` | Embed the EhBASIC image |
| `embed_emutos` | Embed the EmuTOS ROM |
| `embed_aros` | Embed the AROS ROM |
| `ie64` | Build the IE64 assembler entry point |
| `ie64dis` | Build the IE64 disassembler entry point |
| `m68k_test` | Enable M68K-specific tests |
| `musashi` | Enable the Musashi reference core for M68K validation |
| `empirical` | Enable empirical audio validation tests |
| `empiricaljson` | Enable empirical audio JSON export tests |
| `audiolong` | Enable long-running audio demonstrations |
| `videolong` | Enable long-running video demonstrations |

`GOOS=js GOARCH=wasm` selects the browser-specific implementation files. It is
not a manually supplied build tag.

### Generated IE64 opcode metadata

`internal/ie64meta/table.go` is the source of truth for IE64 opcode constants
and mnemonic tables. After changing it, regenerate the derived files and run
the assembler and disassembler tests:

```bash
go generate ./...
go test ./assembler
go test -tags ie64 ./assembler
go test -tags ie64dis ./assembler
go test -tags headless -run TestAssemblerExamples .
```

The tagged tools must be built from the `assembler` package:

```bash
go build -tags ie64 -o ie64asm ./assembler
go build -tags ie64dis -o ie64dis ./assembler
```

## 3. Repository orientation

| Path | Responsibility |
|------|----------------|
| `main.go` | CLI parsing and runtime assembly |
| `machine_bus.go` | Guest RAM, MMIO dispatch and fast bus paths |
| `machine_lifecycle.go` | Loading, reset and profile transitions |
| `program_executor.go` | Guest-requested programme launches and launch hand-off |
| `coprocessor_manager.go` | Concurrent CPU worker lifecycle, instances and tickets |
| `cpu_*.go`, `cpu_*_runner.go` | Guest CPU interpreters and execution routing |
| `jit_*.go` | Shared and CPU-specific JIT implementations |
| `video_chip.go`, `video_*.go` | Video hardware and host backends |
| `audio_chip.go`, `*_engine.go`, `*_player.go` | Audio synthesis and playback |
| `debug_*.go` | Machine Monitor, snapshots and debugging services |
| `script_*.go` | Lua and IEScript integration |
| `assembler/` | IE32 and IE64 assemblers and IE64 disassembler |
| `cmd/` | Converters, compiler and SDK command-line tools |
| `internal/ie64meta/` | Canonical IE64 opcode metadata |
| `internal/ie64obj/`, `internal/ie64link/`, `internal/ie64archive/` | IE64 object, linker and archive support |
| `sdk/include/` | Guest assembly includes, linker configuration and public C header |
| `sdk/examples/` | Guest examples and assets |
| `sdk/docs/` | Architecture, ISA, ABI and tool references |

Almost all runtime code belongs to the root `main` package. Keep new files with
the subsystem they extend and follow the existing
`{subsystem}_{component}[_variant].go` naming pattern.

Before changing `sdk/intuitionos/` or `iexec_*.go`, read
[`sdk/intuitionos/CLAUDE.md`](sdk/intuitionos/CLAUDE.md). It defines additional
kernel, ELF loader and grant-model constraints.

## 4. Canonical technical references

Do not infer guest behaviour from source comments alone. Check the implementation
and the relevant maintained reference:

| Subject | Reference |
|---------|-----------|
| Machine architecture and MMIO | [architecture.md](sdk/docs/architecture.md) |
| IE32 instruction set | [IE32_ISA.md](sdk/docs/IE32_ISA.md) |
| IE64 instruction set | [IE64_ISA.md](sdk/docs/IE64_ISA.md) |
| IE32 JIT | [IE32_JIT.md](sdk/docs/IE32_JIT.md) |
| IE64 JIT | [IE64_JIT.md](sdk/docs/IE64_JIT.md) |
| M68K JIT | [M68K_JIT.md](sdk/docs/M68K_JIT.md) |
| 6502 JIT | [6502_JIT.md](sdk/docs/6502_JIT.md) |
| Z80 JIT | [Z80_JIT.md](sdk/docs/Z80_JIT.md) |
| x86 JIT | [x86_JIT.md](sdk/docs/x86_JIT.md) |
| Machine Monitor | [iemon.md](sdk/docs/iemon.md) |
| IEScript | [iescript.md](sdk/docs/iescript.md) |

The include files are architecture-specific views of shared hardware. They do
not all expose identical helpers or addresses:

| Include | Guest syntax and notable differences |
|---------|--------------------------------------|
| `ie32.inc` | IE32 assembler constants and macros |
| `ie64.inc` | IE64 constants, macros and coprocessor helpers |
| `ie64_fp.inc` | IE64 floating-point library using raw FP64 calling-convention values and hardware FPU wrappers |
| `ie65.inc`, `ie65.cfg` | cc65 constants, macros, zero-page allocation and linker layout; 6502 I/O uses its translated window |
| `ie68.inc` | Motorola-syntax M68K constants and macros |
| `ie80.inc` | Z80 constants and port mappings |
| `ie86.inc` | NASM constants, port I/O and VGA definitions |

Consult the chosen include rather than assuming that a helper, register or
native MMIO address exists on every CPU. In particular, ANTIC/GTIA is not
addressable through the 6502 guest view, and deprecated timer constants are not
an implemented timer device.

### Guest RAM

Native production builds derive RAM sizing from host memory through
platform-specific discovery, after which a host reserve and the selected
profile ceiling are applied. Browser builds instead use a fixed 256 MiB heap
backing. A legacy 32 MiB fallback exists for native hosts where discovery is
unavailable and for the default test bus. It is not the fixed machine size.

Guest software must discover memory through `SYSINFO_TOTAL_RAM_LO/HI` and
`SYSINFO_ACTIVE_RAM_LO/HI`, using the access mapping for its CPU. IE64 can also
read the active size from `CR_RAM_SIZE_BYTES`. IE32, x86 and M68K are bounded by
their 32-bit guest address space. The 6502 and Z80 use banked windows, and OS
profiles may impose narrower source-owned layouts. See the architecture
reference for the current ranges and profile bounds.

## 5. Development workflow

1. Identify the guest-visible contract and the code that owns it.
2. Add or update a focused test that observes the required behaviour.
3. Make the smallest implementation change that satisfies that contract.
4. Run the narrow test first, then the relevant subsystem or package tests.
5. Update canonical documentation when an ISA, MMIO, ABI, tool or build contract
   changes.
6. Run the applicable verification gates before submitting the change.

Use `gofmt` on changed Go files. M68K assembly uses Motorola syntax, 6502 uses
cc65 syntax, and x86 uses NASM syntax.

### Change-specific verification

| Change | Minimum relevant checks |
|--------|-------------------------|
| Root Go runtime | Focused test, then `go test -tags headless ./...` |
| IE32 or IE64 assembler | Tagged assembler test plus `TestAssemblerExamples` |
| CPU interpreter | Focused instruction tests and available differential suites |
| JIT backend | Interpreter parity, backend-specific tests and target execution where available |
| MMIO device | Register-level tests plus architecture documentation check |
| Browser implementation | `make test-wasm-build` and `make test-wasm-node` |
| SDK toolchain | `make test-ie64-toolchain` or the tool-specific package tests |
| Documentation | `make check-docs` and `git diff --check` |

Do not accept cross-compilation alone as proof that target-specific native code
works. Linux arm64 JIT correctness should be executed on arm64 hardware or under
QEMU when the relevant gate supports it.

## 6. Testing and verification

Use the `headless` tag unless a test explicitly requires a host backend.

```bash
# One focused test
go test -tags headless -run TestName ./...

# Main test suite
go test -tags headless -timeout 10m -count=1 ./...

# Static analysis and documentation
make vet
make check-docs

# Makefile quality gates
make test
make test-makefile

# No-Vulkan and headless build checks
go build -tags novulkan .
CGO_ENABLED=1 go build -tags "novulkan headless" .
```

`make tidy` runs `go mod tidy -v` and can modify `go.mod` and `go.sum`. Use it
when dependencies change, then inspect those files before keeping the result.

Relevant repository gates include:

```bash
make test-race
make test-simd
make test-6502-jit-parity
make test-ie32-jit-parity
make test-ie32-jit-race
make test-z80-jit-parity
make test-x86-jit-parity
make test-wasm-build
make test-wasm-node
make testdata-harte
make test-harte-short
make testdata-x86
make test-x86-harte-short
```

The Harte data targets download external test corpora. Full Harte runs are
long-running and should be selected when the affected CPU semantics warrant
them.

`make test-ie32-jit-parity` executes the IE32 contract on the native backend,
under Node and Chromium for WebAssembly, and on Linux arm64 under QEMU. It also
runs the focused IE32 JIT race gate. Use `make test-ie32-jit-race` directly when
working only on cache invalidation or concurrent-write behaviour.

Long-running demonstration tests use `audiolong` or `videolong`:

```bash
go test -tags audiolong -run TestSineWave_BasicWaveforms .
go test -tags videolong -run TestFireEffect .
```

### Performance work

Correctness and parity come before benchmark results. Use the recorded
benchstat workflow rather than copying isolated local numbers into this guide:

```bash
make bench-baseline BENCH_ITEM=my_change BENCH_REGEX='BenchmarkIE64_'
make bench-after BENCH_ITEM=my_change BENCH_REGEX='BenchmarkIE64_'
make bench-compare BENCH_ITEM=my_change
```

Captures are written under `benchmarks/<item>/`. Record the host, toolchain,
build tags, environment switches and revision needed to reproduce the result.
IE32 JIT work has matching interpreter and JIT workloads, including the shipped
Voodoo Mega Demo:

```bash
make ie32-bench-baseline BENCH_ITEM=my_change
make ie32-bench-after BENCH_ITEM=my_change
make ie32-bench-compare BENCH_ITEM=my_change
```

## 7. JIT development

JIT availability follows the dispatch implementation, not the presence of an
emitter file:

| Host | JIT-enabled guest CPUs |
|------|------------------------|
| Linux amd64 | IE32, IE64, M68K, 6502, Z80 and x86 |
| Linux arm64 | IE32, IE64, M68K, 6502, Z80 and x86 |
| Windows amd64 | IE64, M68K, Z80 and x86 |
| Windows arm64 | IE64 and M68K |
| macOS amd64 | IE64, M68K, Z80 and x86 |
| macOS arm64 | IE64 and M68K |
| Browser, js/wasm | IE32, IE64, M68K, 6502 and Z80 WebAssembly backends; x86 also requires WebAssembly SIMD, its executable coverage manifest, the Go memory export and `X86_WASM_JIT` not set to `0` |

IE32 has JIT backends on Linux amd64, Linux arm64 and js/wasm. They lower the
eligible direct-RAM subset and resume through the interpreter at observation
boundaries. JIT execution is enabled by default on those hosts. `--nojit`
selects the interpreter for the primary IE32 CPU, Program Executor launches and
IE32 coprocessor workers created by that process.

Unsupported JIT operations and hosts must retain interpreter fallback.
`--nojit` is the primary CLI opt-out for every guest CPU.

The IE64 JIT supports native amd64 and arm64 hosts and a separate WebAssembly
backend. Hot regions can retain selected guest GPR and FPU state across internal
edges. Helper exits, invalidation, interrupt checks, MMU context and retired
instruction accounting are part of its correctness contract. See
[IE64_JIT.md](sdk/docs/IE64_JIT.md) rather than duplicating backend internals
here.

The M68K JIT supports native amd64 and arm64 hosts plus WebAssembly. Its native
implementation includes block chaining and lazy CCR handling. Its optional
eight-entry RTS cache is disabled by default and is enabled with
`IE_M68K_JIT_ENABLE_RTS_CACHE=1`. See
[M68K_JIT.md](sdk/docs/M68K_JIT.md) for current backend files, admission rules
and verification.

The 6502 JIT is restricted to Linux amd64, Linux arm64 and WebAssembly. Windows
and macOS use the interpreter. Z80 and x86 have their own asymmetric host
matrices as shown above. Use the CPU-specific JIT reference where one exists and
the architecture reference for the combined matrix.

Common runtime switches include:

| Switch | Effect |
|--------|--------|
| `IE_JIT_DISPATCH_CACHE=0` | Disable the shared generation-tagged direct dispatch cache |
| `IE_JIT_SMC_RANGE=0` | Use whole-cache invalidation instead of exact SMC ranges where supported |
| `IE64_JIT_REGIONS=0` | Disable IE64 region promotion |
| `IE64_JIT_REGION_MMU=0` | Disable IE64 region formation under MMU |
| `IE_M68K_JIT_ENABLE_RTS_CACHE=1` | Enable the experimental M68K eight-entry RTS cache |
| `X86_JIT_REGIONS=1` | Enable experimental x86 region promotion |
| `X86_JIT_CHAINS=0` | Disable compatible x86 block chaining |

Treat kill switches as diagnostic controls, not alternate guest semantics.

## 8. Running and debugging a guest

The VM selects a CPU explicitly or from a recognised file extension:

```bash
./bin/IntuitionEngine -ie32 program.iex
./bin/IntuitionEngine -ie64 program.ie64
./bin/IntuitionEngine -m68k program.ie68
./bin/IntuitionEngine -z80 program.ie80
./bin/IntuitionEngine -m6502 program.ie65
./bin/IntuitionEngine -x86 program.ie86
```

Use `--load-addr` and `--entry` when a raw 6502 or Z80 image does not carry the
required placement information. Run `./bin/IntuitionEngine -help` for the live
CLI contract.

Runtime controls:

| Key | Action |
|-----|--------|
| `F7` | Cycle the CRT presentation filter |
| `F8` | Toggle the Lua REPL overlay when the Machine Monitor is inactive |
| `F9` | Toggle the Machine Monitor |
| `F10` | Hard reset to the configured boot profile |
| `F11` | Toggle fit or stretch scaling when available |
| `Shift+F11` | Toggle fullscreen mode when available |
| `F12` | Toggle the runtime status bar |
| `Ctrl+Alt` | Release captured relative mouse mode |

Overlays consume their own input while active. Do not assume that a control key
also reaches the guest in those states.

The Machine Monitor supports all six CPU types. Its maintained command and UI
reference is [iemon.md](sdk/docs/iemon.md). The shared debug-output register is
at `0xF0700`; use the native address or I/O-window mapping defined by the chosen
CPU include.

Selected shared status registers:

| Register | Native address | Meaning |
|----------|----------------|---------|
| `VIDEO_STATUS` | `0xF0008` | VBlank is bit 1 |
| `BLT_STATUS` | `0xF0044` | Bit 0 `ERR`, bit 1 `DONE`, bit 2 `IRQ_PENDING` |
| `PSG_PLAY_STATUS` | `0xF0C1C` | PSG player state |
| `SAP_PLAY_STATUS` | `0xF0D1C` | SAP player state |
| `SID_PLAY_STATUS` | `0xF0E2C` | SID player state |

## 9. Live images and release engineering

| Platform | Build profile or target | JIT coverage |
|----------|-------------------------|--------------|
| Linux x64 | `make`, `make novulkan`, `make headless`, `make headless-novulkan` | See the JIT matrix above |
| Linux ARM64 | `make`, `make novulkan`, `make headless`, `make headless-novulkan` | See the JIT matrix above |
| Raspberry Pi 4 and Pi 400 | `make build-image-pi4` | IE32, IE64, M68K, 6502, Z80 and x86 |
| Raspberry Pi 5 | `make build-image-pi5` | IE32, IE64, M68K, 6502, Z80 and x86 |
| Windows amd64 and arm64 | `make novulkan` | See the JIT matrix above |
| macOS amd64 and arm64 | `make novulkan` | See the JIT matrix above |
| Browser | `make wasm` | IE32, IE64, M68K, 6502 and Z80; x86 requires WebAssembly SIMD |

Intuition Engine is distributed as bootable USB Live images from
[intuitionengine.io](https://intuitionengine.io). Each image boots directly into
IE64 BASIC.

| Appliance | Build target | Local output |
|-----------|--------------|--------------|
| x64 | `make x64-live` | `build/x64-live/intuition-engine-x64.img` and its ZIP archive |
| Raspberry Pi 4 and Pi 400 | `make build-image-pi4` | `build/rpi4-live/intuition-engine-rpi4.img` |
| Raspberry Pi 5 | `make build-image-pi5` | `build/rpi5-live/intuition-engine-rpi5.img` |
| Both Raspberry Pi families | `make rpi-live-images` | Both Raspberry Pi images above |

These targets assemble complete appliances from golden images, guest payloads,
demos and SDK material. They have additional host tools and source inputs beyond
the ordinary VM build.

The Makefile also retains `release-linux`, `release-windows`, `release-macos`,
`release-sdk` and `release-all` for developer archives. Those archives are not
the published Intuition Engine distribution. `release-all` creates source, SDK
and platform archives plus `SHA256SUMS`. Its `release-verify` step checks the
staged layout and then builds `ab3d64`, so it also requires the external AB3D2
source and its build prerequisites.

## 10. Contribution checklist

Before submitting a change:

- Keep the patch scoped to one coherent purpose.
- Add tests for changed behaviour and failure paths.
- Run `gofmt` on changed Go files.
- Run the narrowest proving test and the appropriate broader gate.
- Check every affected host build constraint and fallback path.
- Update architecture, ISA, JIT or SDK documentation when its contract changes.
- Run `make check-docs` for documentation changes.
- Run `git diff --check`.
- State which checks were run and which were skipped.

Public documentation and comments should use British English, concrete technical
language and current implementation evidence. Avoid copying temporary plans,
benchmark snapshots or speculative explanations into maintained reference
documents.
