# IntuitionEngine

Retro hardware emulator: CPUs (6502, Z80, M68K, IE32), audio chips (PSG, SID, POKEY, TED), video (VGA + copper coprocessor).

## Build

```bash
make                    # Build VM to bin/, tools to sdk/bin/
make ie32asm            # Build just the IE32 assembler
make ie64asm            # Build just the IE64 assembler
make basic              # Build with embedded EhBASIC interpreter
make novulkan           # Build without Vulkan (software Voodoo only)
make headless           # Build without display/audio (CI/testing)
make headless-novulkan  # Fully portable CGO_ENABLED=0 build
go build ./...          # Quick build without compression
```

## Test

```bash
go test ./...                          # All tests
go test -run TestName                  # Single test
go test -v -run TestHarte68000 -short  # M68K tests (sampling)
```

## Assemble

```bash
# IE32 (custom CPU)
sdk/bin/ie32asm sdk/examples/asm/program.asm

# M68K (requires vasmm68k_mot)
vasmm68k_mot -Fbin -m68020 -devpac -o out.ie68 input.asm

# 6502 (requires cc65)
make ie65asm SRC=sdk/examples/asm/program.asm

# Z80 (requires vasmz80_std)
make ie80asm SRC=sdk/examples/asm/program.asm
```

## Code Style

- Go: standard gofmt
- Assembly: Motorola syntax for M68K, cc65 syntax for 6502

## Architecture

- `cpu_*.go` - CPU emulators
- `video_*.go` - Video chip and VGA
- `audio_chip.go` - Main audio engine
- `*_engine.go` - Chip engines (PSG, SID, POKEY, TED)
- `*_player.go` - Music file players
- `assembler/` - Assembler tool source code (ie32asm, ie64asm, ie64dis, ie32to64)
- `machine_bus.go` - Machine bus, memory mapping and I/O

## External Tools

- `vasmm68k_mot` / `vasmz80_std` - VASM assemblers (http://sun.hasenbraten.de/vasm/)
- `ca65` / `ld65` - cc65 toolchain for 6502
- `sstrip`, `upx` - Binary compression (optional, for release builds)
