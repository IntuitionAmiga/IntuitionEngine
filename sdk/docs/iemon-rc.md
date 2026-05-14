# IEMon Project RC Files

*Last updated: 2026-05-14*

IEMon looks for `.iemonrc` files by walking from the current working directory up to the file system root. Files are never executed on first sight. A file must be trusted by absolute path and SHA-256 hash before it can be loaded manually or auto-loaded later. Auto-loading only happens while the monitor has exactly one registered CPU; in multi-CPU sessions, use `rc load` explicitly after selecting the intended focus.

`.iemonrc` trust management is host policy and is intentionally not exposed to IEScript; scripts should use typed `dbg.*` setup calls instead.

## Commands

| Command | Description |
|---------|-------------|
| `rc list` | List discovered `.iemonrc` files with hash and trust state |
| `rc trust [file]` | Trust the current contents of a file |
| `rc load [file]` | Load a trusted file |

If no file is supplied, `rc trust` and `rc load` use the nearest discovered `.iemonrc`. Trust records are stored in the IEMon home directory, normally `~/.iemon/trusted`. Set `IEMON_HOME` to use a different directory.

## File Format

Each non-empty line is a monitor command. Lines beginning with `#` or `;` are comments. Arguments use the same quoting rules as the interactive monitor.

Example:

```text
# Project bring-up defaults
sym add reset $1000 func
b reset
bpmdw $5000
pg add $A0000 $A00FF rw cpu=current
history config 32 64 8 256
layout debug
alias bootbp b reset
```

## Allowed Commands

RC files are limited to debugger setup commands:

| Command | Notes |
|---------|-------|
| `b`, `bc` | Breakpoint setup and clearing |
| `ww`, `wc`, `bpm*` | Watchpoint setup and clearing |
| `pg add`, `pg clear`, `pg list` | Page guard setup and inspection |
| `sym add` | Manual symbol insertion |
| `history config` | Whole-machine snapshot chain tuning; use either `history config` to print current values or `history config <delta-interval> <delta-miB> <checkpoints> [snapshots]` to set them |
| `layout` | Layout presets and saving |
| `alias` | Accepted only when the alias target is also allowed. `alias` with no target is allowed as a harmless listing command |

File I/O, scripts, guest memory mutation, and execution-control commands are rejected from rc files. `sym load...` commands are also rejected because they read host files; use `sym add` in rc files and load symbol files manually or through IEScript. If a trusted file changes, its hash no longer matches and it must be trusted again before loading.
