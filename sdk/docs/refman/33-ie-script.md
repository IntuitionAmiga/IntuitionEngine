---
title: "IE Script - Scripted Control and Live Debugging"
sources:
  - script_engine.go
  - feature_scripting.go
---

# Chapter 33 - IE Script

IE Script is the second of Intuition Engine's two debugging
surfaces. Where the Machine Monitor (Chapter 32) is interactive,
typed one command at a time, IE Script is a small scripting
language that runs from a file: it can drive long sequences of
actions, watch for events, react to faults, capture frames, and
script regression tests of your own programs.

A script file has the extension `.ies`. You launch one from BASIC
with `RUN "path/to/file.ies"`, or hand it to the system at start.

## 33.1 The shape of a script

An IE Script file is a sequence of statements. Expressions are
familiar: integers, booleans, strings (with `'single'` or
`"double"` quotes), arithmetic, comparison, `and`/`or`/`not`,
`if`/`then`/`elseif`/`else`/`end`, `while`, `for`, `function`
definitions, and tables.

Comments begin with `--`.

```ies
-- print a banner and pause one frame
print("hello from script")
sys.sleep_frames(1)

-- toggle the cursor for one second
for i = 1, 60 do
  term.cursor(i % 2 == 0)
  sys.sleep_frames(1)
end
```

## 33.2 Global tables

The script's environment exposes Intuition Engine's subsystems as
a small set of global tables. Each table is a namespace of
functions; the descriptions below name each function by its full
path.

### 33.2.1 `sys`

Coarse-grained system control:

| Function                        | Purpose                                |
|---------------------------------|----------------------------------------|
| `sys.sleep_frames(n)`           | Yield until `n` video frames have elapsed |
| `sys.sleep_ms(ms)`              | Yield until `ms` milliseconds elapse   |
| `sys.frames()`                  | Number of frames since boot            |
| `sys.now_ms()`                  | Master clock in milliseconds           |
| `sys.reset()`                   | Hard reset Intuition Engine            |
| `sys.run(path)`                 | Load a program image and run it        |
| `sys.quit()`                    | Exit the script (back to BASIC or to caller) |
| `sys.exit(code)`                | Exit Intuition Engine with status `code` |

### 33.2.2 `cpu`

Active-CPU control. The active CPU is whatever the program
executor most recently switched to, or the CPU you nominate with
`cpu.switch`.

| Function                        | Purpose                                |
|---------------------------------|----------------------------------------|
| `cpu.switch(name)`              | Switch to a named CPU (`"ie64"`, etc.) |
| `cpu.name()`                    | Name of the active CPU                 |
| `cpu.pc()` / `cpu.set_pc(v)`    | Get/set the PC                         |
| `cpu.reg(name)` / `cpu.set_reg(name, v)` | Get/set a register             |
| `cpu.step()`                    | Single-step                            |
| `cpu.freeze()` / `cpu.thaw()`   | Stop / resume the active CPU           |
| `cpu.running()`                 | True if the active CPU is running      |

### 33.2.3 `mem`

Bus accesses. Width is given by the function name.

| Function                  | Purpose                                       |
|---------------------------|-----------------------------------------------|
| `mem.peek8(addr)`         | Read one byte                                 |
| `mem.peek16(addr)`        | Read one little-endian word                   |
| `mem.peek32(addr)`        | Read one little-endian long                   |
| `mem.poke8(addr, v)`      | Write one byte                                |
| `mem.poke16(addr, v)`     | Write one word                                |
| `mem.poke32(addr, v)`     | Write one long                                |
| `mem.read(addr, n)`       | Read `n` bytes, return as a string            |
| `mem.write(addr, s)`      | Write the bytes of string `s` to `addr`       |
| `mem.fill(addr, n, byte)` | Fill `n` bytes with one byte                  |

### 33.2.4 `term`

Terminal control.

| Function                  | Purpose                                       |
|---------------------------|-----------------------------------------------|
| `term.print(s)`           | Print a string to the terminal                |
| `term.clear()`            | Clear the terminal                            |
| `term.cursor(on)`         | Show or hide the cursor                       |
| `term.echo(on)`           | Enable / disable local echo                   |
| `term.input()`            | Return a line of input, or `nil` if none yet  |

### 33.2.5 `audio`

Audio mixer + per-engine control.

| Function                          | Purpose                                |
|-----------------------------------|----------------------------------------|
| `audio.volume(v)`                 | Set the master volume (`0`–`1`)        |
| `audio.mute(on)`                  | Mute / unmute the mixer                |
| `audio.freeze()` / `audio.thaw()` | Freeze / thaw the audio subsystem      |
| `audio.engine(name).play(file)`   | Play `file` on the named engine        |
| `audio.engine(name).stop()`       | Stop the engine                        |

The engine names match the BASIC verbs in Chapter 11: `"sid"`,
`"psg"`, `"sn"`, `"pokey"`, `"ted"`, `"ahx"`, `"mod"`, `"wav"`,
`"paula"`, `"sfx"`.

### 33.2.6 `video`

Video subsystem.

| Function                       | Purpose                                  |
|--------------------------------|------------------------------------------|
| `video.mode()` / `video.set_mode(m)` | Read / set VideoChip resolution     |
| `video.fb_base()`              | Current framebuffer base address         |
| `video.frame()`                | Index of the most recent rendered frame  |
| `video.snapshot(file)`         | Save the current frame as a PNG          |

### 33.2.7 `rec`

Video recording.

| Function                       | Purpose                                  |
|--------------------------------|------------------------------------------|
| `rec.start(file)`              | Begin recording to `file`                |
| `rec.stop()`                   | Stop and finalise                        |
| `rec.is_recording()`           | Return `true` while recording            |

### 33.2.8 `dbg`

The debugging surface. This is the bridge between scripts and the
Machine Monitor.

| Function                                       | Purpose                                |
|------------------------------------------------|----------------------------------------|
| `dbg.cmd(line)`                                | Run a monitor command, return its text output |
| `dbg.break_at(addr)`                           | Set a code breakpoint                  |
| `dbg.watch(addr, size, mode)`                  | Set a memory watchpoint                |
| `dbg.on_fault(cb)`                             | Register a callback for trap/fault events |
| `dbg.wait_fault(timeout_ms)`                   | Block until a fault fires              |
| `dbg.disasm(addr, count)`                      | Disassemble `count` instructions       |
| `dbg.regs()`                                   | Return a table of register name → value |
| `dbg.history(count)`                           | Return recent execution history        |

`dbg.cmd("d 1000 20")` is the script equivalent of typing
`d 1000 20` at the monitor prompt. The full command surface of
Chapter 32 is available through `dbg.cmd`.

### 33.2.9 `sym` and `regions`

Symbol lookup and memory-region maps:

| Function                       | Purpose                                  |
|--------------------------------|------------------------------------------|
| `sym.lookup(name)`             | Return the address of a symbol           |
| `sym.name(addr)`               | Return the symbol at `addr` if any       |
| `regions.list()`               | Table of `{base, end, name}` for each MMIO region |

### 33.2.10 `bit32`

Bitwise utilities (the language does not have native bit
operators): `bit32.band`, `bor`, `bxor`, `bnot`, `lshift`,
`rshift`, `arshift`, `lrotate`, `rrotate`, `btest`, `extract`,
`replace`.

## 33.3 Event-driven scripting

Scripts can be linear (do this, then that, then exit) or
event-driven (wait for things to happen, react). The two main
event sources are video frames and faults.

```ies
-- count frames until something interesting happens
local stop = false
dbg.on_fault(function(ev)
  print(string.format("fault at PC=%08x cause=%d", ev.pc, ev.cause))
  stop = true
end)

while not stop do
  sys.sleep_frames(1)
end
```

The callback runs synchronously when the CPU faults; the main
script body resumes on the next frame.

## 33.4 Running scripts

There are three ways to start a script:

1. **From BASIC**: `RUN "path/to/file.ies"` from the BASIC prompt
   loads and starts the script. BASIC waits until the script
   returns or calls `sys.quit`.
2. **From the monitor**: at the `(cpu)>` prompt, type
   `load_script path/to/file.ies` to run a script while keeping
   the monitor open. Faults that fire during the script return
   control to the monitor.
3. **At launch**: the boot environment can be configured to run a
   script as the first user-facing action, useful for automated
   regression suites.

A script that fails - syntax error, uncaught error, or explicit
`error(msg)` - prints the message to the terminal and returns
control to whichever surface launched it.

## 33.5 A small example

A script that runs a fresh program, waits ten frames, captures a
PNG, and exits:

```ies
sys.reset()
sys.run("demo.ie64")
sys.sleep_frames(10)
video.snapshot("frame10.png")
sys.quit()
```

A more substantial example that sets a breakpoint, runs a known
program, dumps registers at the break, and continues to exit:

```ies
sys.run("test.ie64")
dbg.break_at(0x2000)

local hit = false
dbg.on_fault(function(ev)
  if ev.kind == "breakpoint" then
    local r = dbg.regs()
    print("at $2000:")
    for k, v in pairs(r) do
      print(string.format("  %-4s = %08x", k, v))
    end
    hit = true
  end
end)

while not hit do
  sys.sleep_frames(1)
end
```

## 33.6 What comes next

Part IV ends here. Part V picks up the I/O side of Intuition
Engine: disk and file I/O (Chapter 34), the `HOST` command and its
appliance-control bridge (Chapter 35), the keyboard / mouse /
controller MMIO (Chapter 36), and the serial interface
(Chapter 37).
