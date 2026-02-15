# Toolchain Reference

Assembler toolchains for each Intuition Engine CPU core.

## Running Assembled Programs

Programs can be run from the command line or from the EhBASIC interpreter:

```bash
# Command line
./bin/IntuitionEngine -ie32 program.iex

# From EhBASIC (at the BASIC prompt)
RUN "program.iex"
```

The `RUN` command auto-detects format by extension: `.iex` (IE32), `.ie64` (IE64), `.ie68` (M68K), `.ie80` (Z80), `.ie65`/`.bin` (6502), `.ie86` (x86).

## Built-in Assemblers

### IE32 Assembler (`ie32asm`)

```bash
make ie32asm                              # Build
sdk/bin/ie32asm program.asm                 # Assemble (produces program.iex)
sdk/bin/ie32asm -I sdk/include program.asm  # With include search path
./bin/IntuitionEngine -ie32 program.iex   # Run (or: RUN "program.iex" from BASIC)
```

- Custom assembler built from `assembler/ie32asm.go`
- Supports `-I dir` include search paths (multiple allowed, searched after source file directory)
- Supports `.include`, `.equ`, `.org`, `.db`, `.dw`, `.dd`, labels, macros
- Fixed 8-byte instruction format

### IE64 Assembler (`ie64asm`)

```bash
make ie64asm                              # Build
sdk/bin/ie64asm program.asm                 # Assemble (produces program.ie64)
sdk/bin/ie64asm -I sdk/include program.asm  # With include search path
sdk/bin/ie64asm -list program.asm           # Assemble with listing output
./bin/IntuitionEngine -ie64 program.ie64  # Run (or: RUN "program.ie64" from BASIC)
```

- Custom assembler built from `assembler/ie64asm.go`
- Supports `-I dir` include search paths (multiple allowed, searched after source file directory)
- Supports `.include`, `equ`, `org`, `dc.b/w/l/q`, labels, macros with positional parameters (`\1`..`\9`)
- Variable-length instruction encoding (4-12 bytes)

### IE64 Disassembler (`ie64dis`)

```bash
make ie64dis                              # Build
sdk/bin/ie64dis program.ie64                # Disassemble
```

### IE32-to-IE64 Converter (`ie32to64`)

```bash
make ie32to64                             # Build
sdk/bin/ie32to64 program.asm                # Convert IE32 assembly to IE64
```

See [ie32to64.md](ie32to64.md) for conversion details.

## External Assemblers

### M68K: VASM (`vasmm68k_mot`)

**Install:**
```bash
# Download from http://sun.hasenbraten.de/vasm/
# Build:
make CPU=m68k SYNTAX=mot
```

**Assemble:**
```bash
vasmm68k_mot -Fbin -m68020 -devpac -o output.ie68 input.asm

# With include path
vasmm68k_mot -Fbin -m68020 -devpac -I assembler -o output.ie68 input.asm
```

**Run:**
```bash
./bin/IntuitionEngine -m68k output.ie68   # Or: RUN "output.ie68" from BASIC
```

### Z80: VASM (`vasmz80_std`)

**Install:**
```bash
# Download from http://sun.hasenbraten.de/vasm/
# Build:
make CPU=z80 SYNTAX=std
```

**Assemble:**
```bash
vasmz80_std -Fbin -I assembler -o output.ie80 input.asm

# Or via Makefile helper:
make ie80asm SRC=assembler/program.asm
```

**Run:**
```bash
./bin/IntuitionEngine -z80 output.ie80    # Or: RUN "output.ie80" from BASIC
```

### 6502: cc65 (`ca65` / `ld65`)

**Install:**
```bash
# Ubuntu/Debian
sudo apt install cc65

# macOS
brew install cc65
```

**Assemble:**
```bash
ca65 -I assembler -o program.o program.asm
ld65 -C assembler/ie65.cfg -o program.ie65 program.o
rm program.o

# Or via Makefile helper:
make ie65asm SRC=assembler/program.asm
```

The `ie65.cfg` linker configuration defines the Intuition Engine 6502 memory layout.

**Run:**
```bash
./bin/IntuitionEngine -m6502 program.ie65  # Or: RUN "program.ie65" from BASIC
# Or with explicit load/entry addresses:
./bin/IntuitionEngine -m6502 --load-addr 0x0600 --entry 0x0600 program.bin
```

### x86: NASM

**Install:**
```bash
# Ubuntu/Debian
sudo apt install nasm

# macOS
brew install nasm
```

**Assemble:**
```bash
nasm -f bin -o program.ie86 program.asm
```

**Run:**
```bash
./bin/IntuitionEngine -x86 program.ie86   # Or: RUN "program.ie86" from BASIC
```

## Environment Variables

The SDK build scripts support custom tool paths:

| Variable | Default | Description |
|----------|---------|-------------|
| `IE_BIN_DIR` | `./bin` | Path to IE assembler binaries |
| `VASM_M68K` | `vasmm68k_mot` | M68K assembler path |
| `VASM_Z80` | `vasmz80_std` | Z80 assembler path |
| `CA65` / `LD65` | `ca65` / `ld65` | cc65 toolchain paths |
| `NASM` | `nasm` | x86 assembler path |
