# IEMon Symbols

IEMon keeps an in-memory symbol table per CPU address space. Symbols can come from manual monitor commands, VICE label files, `.iesym` files that use the same accepted label syntax, or ELF `.symtab` entries.

## Sources

| Source | Command | CPU notes |
|--------|---------|-----------|
| Manual symbol | `sym add <name> <addr> [func|object|label]` | Accepted for all CPUs |
| VICE labels / `.iesym` labels | `sym loadlbl <file> [base]` | Canonical sidecar format for 6502, Z80, and other retro media; accepted for all CPUs |
| ELF `.symtab` | `sym loadelf <file>` | Canonical for IE32, IE64, X86, and symbol-bearing M68K OS builds |

VICE labels and `.iesym` sidecars use lines such as `al 00:c000 .irq_handler`; the loader ignores blank lines, `#` comments, `;` comments, and non-`al` records. When an address contains a bank prefix, the text after the last colon is used. A `base` argument is added to each parsed address for guest-load hooks and relocated programs.

## Automatic Sidecars

When the runtime can see a neighbouring symbol file, IEMon ingests it
without requiring a manual `sym load...` command:

| Loaded artefact | Sidecar candidates | Symbol CPU |
|-----------------|--------------------|------------|
| SID, TED, PRG, SAP/POKEY media | `<file>.lbl`, then `<file-without-ext>.lbl` | 6502 |
| AY/YM/SNDH/VTX/PT/STC/SQT/ASC/FTC/VGM/PSG media | `<file>.lbl`, then `<file-without-ext>.lbl` | Z80 |
| AHX or MOD media | `<file>.lbl`, then `<file-without-ext>.lbl` | M68K |
| EmuTOS ROM image | `<rom>.elf`, then `<rom-without-ext>.elf` | M68K |
| AROS ROM image | `<rom>.elf`, then `<rom-without-ext>.elf` | M68K |
| GEMDOS `Pexec` program | `<program>.iesym`, `<program>.lbl`, then stem variants | M68K, relocated to the loaded TEXT base |
| AROS DOS `LoadSeg` program | `<program>.iesym`, `<program>.lbl`, then stem variants | M68K, relocated to the guest `LoadSeg` base |

Sidecar loading is best-effort. Missing sidecars are ignored; malformed
sidecars print a warning at the runtime load site or return an error from the
manual `sym` command.

AROS DOS file system interception has an explicit `LoadSeg` symbol notification
in the IE host-device protocol. The guest handler reports the loaded path and
relocation base after a successful `LoadSeg`, so neighbouring `.iesym` or `.lbl`
files are rebased automatically. Manual `sym loadlbl <file> <base>` remains
available for older guest handlers or ad-hoc symbol files.

## Address Syntax

Commands that accept register-plus-address expressions also accept symbols:

```
> sym add main $2000 func
> d main
> b main+0x10
> sym resolve $2010
```

## CPU Scope

| CPU | Scope |
|-----|-------|
| IE64 | Separate IE64 symbol namespace |
| IE32 | Separate IE32 symbol namespace |
| M68K | Separate M68K symbol namespace |
| Z80 | Separate Z80 symbol namespace |
| 6502 | Separate 6502 symbol namespace |
| X86 | Separate X86 symbol namespace |

## DWARF Source Lines

`sym loadelf <file>` also attempts to read DWARF line information when present. `d /s` interleaves source locations into disassembly, and `list [addr]` prints the source location nearest an address plus a small source context window when the file is available. Relative source paths are resolved from the current directory and then from `IEMON_SRC_PATH` entries.

| CPU | Source-line support |
|-----|---------------------|
| IE64 | DWARF from ELF when present |
| X86 | DWARF from ELF when present |
| M68K | Graceful no-source fallback unless DWARF-bearing ELF is loaded |
| IE32 | Graceful no-source fallback unless DWARF-bearing ELF is loaded |
| Z80 | Graceful no-source fallback |
| 6502 | Graceful no-source fallback |

## IEScript

Scripts use the same focussed CPU namespace:

```lua
sym.load_elf("demo.elf")
sym.load_vice("demo.lbl", 0x2000)
sym.load_dwarf("demo.elf")

local loaded = sym.autoload("demo.ie68", 0x4000)
if loaded.loaded then
    sys.print("loaded " .. loaded.kind .. " symbols from " .. loaded.path)
elseif loaded.err then
    error(loaded.err)
end

local here = sym.resolve(dbg.get_pc())
local src = dbg.source_at(dbg.get_pc())
```

`sym.autoload(image_path [, base])` probes in this order: `<image_path>.elf`, `<stem>.elf`, then `<image_path>.iesym`, `<image_path>.lbl`, `<stem>.iesym`, and `<stem>.lbl`. It stops at the first existing candidate and returns `{loaded=bool, path=string|nil, kind="elf"|"guest"|nil, err=string|nil}`. Script path sandboxing still applies: `image_path` is normalised under the script roots, and every sidecar that exists must pass approved read-path validation before it is loaded.
