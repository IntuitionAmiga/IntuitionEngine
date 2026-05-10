# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Intuition Engine is a retro hardware emulator/virtual machine supporting 6 CPU architectures (IE64, IE32, M68K, Z80, 6502, x86), 9 audio engines, and 6 video systems. Written in Go with ~134K lines of implementation code (~258K including tests), all in a single main package (no sub-packages except `internal/musashi`, `assembler/`, `cmd/`, and `tools/`).

## Build Commands

```bash
make                        # Build everything (VM + all SDK tools)
make intuition-engine       # Build only the VM
make clean                  # Remove all build artifacts
make distclean              # Destructive full cleanup including generated IntuitionOS assets/testdata/AROS
make tidy                   # Run go mod tidy explicitly

# Build profiles
make novulkan               # No Vulkan SDK required
make headless               # No display/audio/Vulkan (CI/testing)
make headless-novulkan      # CGO_ENABLED=0, fully portable
```

Build outputs go to `./bin/IntuitionEngine` and `sdk/bin/` (assemblers/tools).

## Testing

```bash
# Standard test run (always use headless tag)
go test -tags headless ./...
make test                   # Equivalent Makefile target

# Single test
go test -tags headless -run TestName ./...

# With timeout (CI uses 10m)
go test -tags headless -timeout 10m -count=1 ./...

# Benchmarks (skip tests with -run='^$')
go test -tags headless -run='^$' -bench BenchmarkIE64_ -benchtime 3s ./...

# JIT-specific tests
go test -v -run TestAMD64_ -tags headless ./...      # x86-64 JIT
go test -v -run TestARM64_ -tags headless ./...      # ARM64 JIT
go test -v -run TestJIT_ -tags headless ./...        # Shared JIT infra

# M68K Harte test suite
make testdata-harte         # Download test data (one-time)
make test-harte             # Full suite (~30 min)
make test-harte-short       # Sampling mode (~5 min)

# x86 Harte 8088 test suite
make testdata-x86           # Download test data (one-time)
make test-x86-harte         # Full suite
make test-x86-harte-short   # Sampling mode

# Long-running demo tests (require special build tags)
go test -tags audiolong -run TestSineWave_BasicWaveforms
go test -tags videolong -run TestFireEffect

# Env-gated tests (skipped by default headless run)
IE_HARTE_LONG=1 go test -tags headless -run TestHarte68000   # full Harte 68000 suite
IE_HARTE_LONG=1 go test -tags headless -run TestHarte8086    # full Harte 8086 suite
IE_AROS_DIAG=1 go test -tags headless -run TestAROS.*Diagnostic   # AROS post-ready diagnostic harnesses

# Order-dependent IExec tests (skipped in full-suite runs to avoid shared-state
# pollution; runnable solo via narrow -run pattern, or forced via env var):
go test -tags headless -run 'TestIExec_M13_Phase2_CreateTaskUsesDynamicLayout$' .   # solo run
IE_RUN_QUARANTINED=1 go test -tags headless ./...   # force flakes to run in full suite
```

## Linting / Vet

```bash
make vet                    # go vet -tags headless -unsafeptr=false ./...
go vet ./...
```

## Build Verification Before Submitting

```bash
go build ./...
go test -tags headless ./...
make test-makefile
make release-verify
go build -tags novulkan .
CGO_ENABLED=0 go build -tags "novulkan headless" .
```

## Build Tags

| Tag | Effect |
|-----|--------|
| `headless` | Stubs out GUI/audio/video backends (required for CI/testing) |
| `novulkan` | Software Voodoo rasterizer, no Vulkan dependency |
| `embed_basic` | Embed EhBASIC binary |
| `embed_emutos` | Embed EmuTOS ROM image |
| `embed_aros` | Embed AROS ROM image |
| `m68k_test` | M68K-specific tests |
| `musashi` | Enable Musashi reference CPU for M68K validation (requires CGO) |
| `ie64` | Build ie64asm assembler |
| `ie64dis` | Build ie64dis disassembler |
| `empirical` | Audio empirical validation tests |
| `empiricaljson` | Audio empirical JSON export tests |
| `audiolong` / `videolong` | Long-running demonstration tests |

## Architecture

### Single-Package Design

The entire VM is in one Go package (root directory). Files follow the naming convention `{subsystem}_{component}[_variant].go` with co-located test files.

### Core Subsystems & Key Files

- **Memory bus** (`machine_bus.go`): autodetected guest RAM with I/O region mapping via callbacks. Thread-safe with atomic operations and lock-free fast paths. The appliance picks total guest RAM from host `/proc/meminfo` minus a per-platform reserve at boot (see `memory_sizing.go`). `bootGuestRAMFromComputed` constructs the bus via `NewMachineBusSized` and attaches an mmap-backed `Backing` (`MmapBacking`) for the high-range guest RAM. **`len(bus.memory)` is the legacy direct-slice low compatibility window, NOT the guest RAM size authority** — IE64-family modes pin it at `lowMemWindowBytes = 256 MiB`; the high `Backing` carries the full host-scale total. `TotalGuestRAM` / `ActiveVisibleRAM` are the only RAM-size sources of truth and what SYSINFO + `CR_RAM_SIZE_BYTES` report. `busMemMaxBytes = 0xFFFF0000` sits below the M68K sign-extension alias zone. On Linux/darwin the production `bus.memory` allocator uses anonymous `mmap` so multi-GiB advertised ranges are demand-paged (no eager commit); Linux high-range `MmapBacking` passes `MAP_NORESERVE` so virtual reservation does not eagerly reserve swap/commit. `MachineBus.Reset` uses `madvise` so guest reset releases pages instead of touching every byte. Non-mmap platforms soft-fall back via `ErrHighRangeBackingUnsupported` and clamp at `busMemBootClamp = 256 MiB`. **CPU reset/load paths must not iterate over guest RAM**: every core's `LoadProgramBytes` (and the `buildReloadClosure` reload paths that wrap it) clamps writes to the CPU-visible program window — IE64/IE32 to `[PROG_START, STACK_START)`, Z80/6502 to `BankedVisibleCeiling`, x86 to `ProfileMemoryCap`, M68K to `[M68K_ENTRY_POINT, M68K_STACK_START)`. F10/reload of an oversize cached program must not spill past these bounds into MMIO, vectors, stack, or RAM beyond the visible ceiling. Memory zeroing belongs to `MachineBus.Reset`. `ApplyProfileVisibleCeiling` clamps active visible RAM (called before `RegisterSysInfoMMIOFromBus`). Guests discover sizes via the SYSINFO MMIO pairs (`SYSINFO_TOTAL_RAM_LO/HI`, `SYSINFO_ACTIVE_RAM_LO/HI`) and IE64 `CR_RAM_SIZE_BYTES` (read-only, live-read of `bus.ActiveVisibleRAM()`; MTCR raises ILLEGAL_INSTRUCTION). Source-owned profiles (EmuTOS, AROS, EhBASIC) impose explicit profile bounds via `profile_bounds.go`; AROS caps at 2 GiB. **Bus32 high-backing**: 32-bit modes (IE32, x86, bare-M68K) reach advertised RAM only via `bus.memory` (no high-range `Backing`). On Linux/darwin the mmap-lazy allocator sizes `bus.memory` up to `busMemMaxBytes` so guests see up to ~4 GiB-page through the legacy uint32 paths; `TotalGuestRAM` / `ActiveVisibleRAM` for these modes equal `min(host total, busMemMaxBytes)` (further clamped by `busMemBootClamp` on non-mmap platforms).
- **CPU cores** (`cpu_*.go`): CPU64 (IE64, `cpu_ie64.go`), CPU (IE32, `cpu_ie32.go`), M68KRunner, CPUZ80Runner, CPU6502Runner, CPUX86Runner. The 8/16/32-bit cores use runner wrappers; IE32/IE64 are instantiated directly.
- **JIT compilers** (`jit_*.go`): Block-based compilation with code cache. On Linux/Windows amd64: IE64, 6502, M68K, Z80, x86. On Linux/Windows/macOS arm64: IE64 only. No JIT for IE32.
- **Audio** (`audio_chip.go`, `*_engine.go`): SoundChip (10-channel synth: 4 base + 3 SID2 + 3 SID3) plus PSG, SID, POKEY, TED, AHX, MOD, WAV engines. AROS Paula-style DMA shim (`aros_audio_dma.go`) for AROS. Music format players in `*_player.go`.
- **Compression** (`lha_archive.go`, `lh*_decompress.go`, `ice_unpack.go`): LHA (`-lh0-`, `-lh1-`, `-lh4-`..`-lh7-`), ICE!, and gzip paths are strict-by-default with allocation caps. See `sdk/docs/decompression.md`.
- **Video** (`video_chip.go`, `video_*.go`): IEVideoChip (primary), VGA, ULA, TED, ANTIC/GTIA engines. Copper coprocessor, DMA blitter (with SNES-style Mode 7 affine transform), Voodoo 3D. VGA is an IE-bus chip, not PC; access via MMIO `0xF1xxx`, VRAM `0xA0000`, text `0xB8000`. ULA is an IE-native chip with Spectrum-style bitmap/attribute rendering: registers are `0xF2000-0xF2017`; 32-bit-address CPUs use the VRAM aperture `0xFA000-0xFBAFF`; Z80/6502 use explicit paged VRAM address/data ports/registers. Do not reintroduce a hidden `$4000` ULA alias. The compositor owns sorted source registration, interleaves scanline-aware and opaque layers by z-order, treats alpha as a binary mask, keeps the guest tick fixed at 60 Hz, and calls frame callbacks once per composite pass even when no source produces content.
- **Main entry** (`main.go`): CLI flags, initialization, Ebiten game loop.
- **Debug** (`debug_*.go`): Machine monitor (F9), per-CPU disassemblers, breakpoints/watchpoints. Snapshot save/load/backstep is focused CPU-local; whole-machine snapshots are not implemented.
- **Scripting** (`script_*.go`): Lua 5.1 via gopher-lua with IEScript API modules.
- **OS integration** (`emutos_loader.go`, `aros_loader.go`, `gemdos_intercept.go`, `aros_dos_intercept.go`): EmuTOS and AROS boot/filesystem support.
- **Assembler tools** (`assembler/`): ie32asm, ie64asm, ie64dis (separate build tags: `ie64`, `ie64dis`). Also ie32to64 transpiler (`cmd/ie32to64/`) and m68kto64 transpiler (`cmd/m68kto64/`, covers 68000 + 68020 integer ISA + 68881/68882 FPU; see `sdk/docs/M68KtoIE64.md`).

### Component Communication

- CPU cores read/write the MachineBus; I/O region writes dispatch to chip handler callbacks
- Audio/video chips are memory-mapped to registers in the I/O region
- Hardware interrupts flow from timer/VBlank through the bus to CPU cores
- Ebiten game loop drives the 60 FPS frame boundary; audio generates at 44.1 kHz

### Architecture-Specific Code

- JIT backends: `*_amd64.go` / `*_arm64.go` with corresponding build constraints
- `be_unsupported.go`: rejects big-endian architectures at compile time

### Platform Backends

- **Display**: Ebiten (default) or headless stub
- **Audio**: Oto (default) or headless stub
- **3D**: Vulkan (default) or software rasterizer (`novulkan`)

## Code Style

- Standard `gofmt`
- Assembly: Motorola syntax (M68K), cc65 (6502), NASM (x86)

## Prerequisites

- Go 1.26+
- `upx` for binary compression (`sstrip` is disabled by default in Makefile, set to no-op)
- Vulkan SDK only needed for full profile (not novulkan/headless)

# Agent Directives: Mechanical Overrides

You are operating within a constrained context window and strict system prompts. To produce production-grade code, you MUST adhere to these overrides:

## Pre-Work

1. THE "STEP 0" RULE: Dead code accelerates context compaction. Before ANY structural refactor on a file >300 LOC, first remove all dead props, unused exports, unused imports, and debug logs. Commit this cleanup separately before starting the real work.

2. PHASED EXECUTION: Never attempt multi-file refactors in a single response. Break work into explicit phases. Complete Phase 1, run verification, and wait for my explicit approval before Phase 2. Each phase must touch no more than 5 files.

## Code Quality

3. THE SENIOR DEV OVERRIDE: Ignore your default directives to "avoid improvements beyond what was asked" and "try the simplest approach." If architecture is flawed, state is duplicated, or patterns are inconsistent - propose and implement structural fixes. Ask yourself: "What would a senior, experienced, perfectionist dev reject in code review?" Fix all of it.

4. FORCED VERIFICATION: Your internal tools mark file writes as successful even if the code does not compile. You are FORBIDDEN from reporting a task as complete until you have fixed ALL resulting errors.

## Context Management

5. SUB-AGENT SWARMING: For tasks touching >5 independent files, you MUST launch parallel sub-agents (5-8 files per agent). Each agent gets its own context window. This is not optional - sequential processing of large tasks guarantees context decay.

6. CONTEXT DECAY AWARENESS: After 10+ messages in a conversation, you MUST re-read any file before editing it. Do not trust your memory of file contents. Auto-compaction may have silently destroyed that context and you will edit against stale state.

7. FILE READ BUDGET: Each file read is capped at 2,000 lines. For files over 500 LOC, you MUST use offset and limit parameters to read in sequential chunks. Never assume you have seen a complete file from a single read.

8. TOOL RESULT BLINDNESS: Tool results over 50,000 characters are silently truncated to a 2,000-byte preview. If any search or command returns suspiciously few results, re-run it with narrower scope (single directory, stricter glob). State when you suspect truncation occurred.

## Edit Safety

9.  EDIT INTEGRITY: Before EVERY file edit, re-read the file. After editing, read it again to confirm the change applied correctly. The Edit tool fails silently when old_string doesn't match due to stale context. Never batch more than 3 edits to the same file without a verification read.

10. NO SEMANTIC SEARCH: You have grep, not an AST. When renaming or
    changing any function/type/variable, you MUST search separately for:
    - Direct calls and references
    - Type-level references (interfaces, type assertions, type switches)
    - String literals containing the name
    - Interface satisfaction (implicit in Go)
    - go:generate directives and build tags
    - Test files and mocks
    Do not assume a single grep caught everything.
