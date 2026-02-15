# SDK Getting Started

Quick guide to building and running your first Intuition Engine program using the SDK.

## Prerequisites

1. Build the Intuition Engine VM, assemblers, and pre-assemble SDK demos:
   ```bash
   make sdk
   ```

2. Install external toolchains for your target CPU (only IE32 and IE64 are built-in):

| CPU | Toolchain | Install |
|-----|-----------|---------|
| M68K | `vasmm68k_mot` | [VASM](http://sun.hasenbraten.de/vasm/) |
| Z80 | `vasmz80_std` | [VASM](http://sun.hasenbraten.de/vasm/) |
| 6502 | `ca65` / `ld65` | [cc65](https://cc65.github.io/) |
| x86 | `nasm` | [NASM](https://www.nasm.us/) |

## Your First Program

The simplest SDK example is the VGA text mode demo. If you ran `make sdk`, it's already pre-assembled:

```bash
# Run the pre-assembled demo
./bin/IntuitionEngine -ie32 sdk/examples/prebuilt/vga_text_hello.iex

# Or assemble from source and run
sdk/bin/ie32asm sdk/examples/asm/vga_text_hello.asm
./bin/IntuitionEngine -ie32 vga_text_hello.iex

# Or from the EhBASIC prompt:
# RUN "sdk/examples/prebuilt/vga_text_hello.iex"
```

This displays coloured text on an 80x25 VGA text screen.

## Building All Examples

```bash
# Build everything (skips missing toolchains gracefully)
./sdk/scripts/build-all.sh

# Build for a specific CPU
./sdk/scripts/build-ie32.sh
./sdk/scripts/build-ie64.sh
./sdk/scripts/build-m68k.sh
./sdk/scripts/build-z80.sh
./sdk/scripts/build-6502.sh
./sdk/scripts/build-x86.sh
```

## Writing a New Program

1. Create your source file:
   ```bash
   my_demo.asm
   ```

2. Include the appropriate hardware header:
   ```assembly
   ; IE32
   .include "ie32.inc"

   ; IE64
   include "ie64.inc"

   ; M68K
   include "ie68.inc"
   ```

3. Use defined constants instead of raw addresses:
   ```assembly
   ; Instead of: STORE A, @0x0F0000
   STORE A, @VIDEO_CTRL
   ```

4. Assemble and run:
   ```bash
   sdk/bin/ie32asm my_demo.asm
   ./bin/IntuitionEngine -ie32 my_demo.iex
   ```

## EhBASIC Programs

BASIC programs run on the IE64 core:

```bash
# Build the BASIC-enabled VM
make basic

# Start the interpreter
./bin/IntuitionEngine -basic

# Or run in console mode
./bin/IntuitionEngine -basic -term
```

Example BASIC program:
```basic
10 SCREEN 0, 1
20 CLS
30 FOR I = 0 TO 15
40   COLOR I
50   PRINT "HELLO WORLD"
60 NEXT I
70 VSYNC
```

## Next Steps

- [SDK README](../README.md) - Full SDK documentation with demo matrix
- [Tutorial](TUTORIAL.md) - Build a complete demoscene intro
- [IE64 ISA](IE64_ISA.md) - IE64 instruction set reference
- [IE64 Cookbook](IE64_COOKBOOK.md) - Common patterns and recipes
- [EhBASIC Guide](ehbasic_ie64.md) - BASIC language reference
- [DEVELOPERS.md](../../DEVELOPERS.md) - Build profiles, testing, and contribution guide
