---
title: "IE Script - Scripted Control and Live Debugging"
sources:
  - script_engine.go
  - feature_scripting.go
---

# Chapter 33 - IE Script

IE Script is the batch-control partner to IE Mon. IE Mon is typed
one command at a time; IE Script stores a sequence of commands and
runs them as one job. Use it for repeatable setup, frame waits,
VideoChip checks, breakpoint sessions, and fault capture.

IE Script is not the normal way to write CPU programs. BASIC and
IE Mon remain the native programming route. IE Script automates the
machine while a program is running.

## 33.1 Running a Script

A stored script uses the `.IES` suffix. From BASIC, run it with:

```text
RUN "FIRST.IES"
```

From IE Mon, run the same stored script with:

```text
(ie64)> script FIRST.IES
```

When a script finishes, BASIC or IE Mon regains control. If it
raises an error, the message is printed and control returns to the
surface that launched it.

## 33.2 Language Shape

An IE Script is a sequence of statements. It supports integers,
booleans, strings, arithmetic, comparison, tables, functions,
`if`/`then`/`elseif`/`else`/`end`, `while`, and `for`.

Comments begin with `--`:

```ies
-- Wait for two video frames, then print a line.
sys.wait_frames(2)
sys.print("ready")
```

Function names are lower case. Module names are lower case.
Strings may use single or double quotes.

## 33.3 System Module

`sys` handles time, text output, script exit, and storage helpers.

| Function | Purpose |
|----------|---------|
| `sys.wait_frames(n)` | Yield until `n` video frames have passed. |
| `sys.wait_ms(ms)` | Yield until `ms` milliseconds have passed. |
| `sys.print(text)` | Print text to the terminal. |
| `sys.log(text)` | Write a diagnostic line. |
| `sys.time_ms()` | Current monotonic time in milliseconds. |
| `sys.frame_count()` | Number of completed frames. |
| `sys.frame_time()` | Most recent frame time. |
| `sys.fps()` | Current frame-rate estimate. |
| `sys.quit()` | Stop the script and return to the caller. |
| `sys.exit(code)` | Stop Intuition Engine with status `code`. |
| `sys.mkdir(name)` | Create a directory in approved script storage. |
| `sys.read_file(name)` | Return stored bytes as a string. |
| `sys.write_file(name, data)` | Write bytes to approved script storage. |
| `sys.copy_file(from, to)` | Copy stored bytes. |
| `sys.capture_output(name)` | Start terminal output capture. |
| `sys.capture_output_off()` | Stop terminal output capture. |

Long loops should call `sys.wait_frames` or `sys.wait_ms`; this
keeps cancellation responsive.

## 33.4 CPU Module

`cpu` controls the selected CPU:

| Function | Purpose |
|----------|---------|
| `cpu.load(name)` | Load and start a stored CPU program. |
| `cpu.load_stopped(name)` | Load a stored CPU program but leave it stopped. |
| `cpu.reset()` | Reset the selected CPU. |
| `cpu.freeze()` | Stop the selected CPU for safe raw RAM access. |
| `cpu.resume()` | Resume after `cpu.freeze()`. |
| `cpu.start()` | Start execution. |
| `cpu.stop()` | Stop execution. |
| `cpu.is_running()` | Return true if the selected CPU is running. |
| `cpu.mode()` | Return the selected CPU name. |
| `cpu.execution_mode()` | Return the current execution mode name. |

Raw RAM access through `mem` requires the CPU to be frozen. MMIO
access is allowed while the CPU is running.

## 33.5 Memory Module

`mem` reads and writes the bus:

| Function | Purpose |
|----------|---------|
| `mem.read8(addr)` | Read one byte. |
| `mem.read16(addr)` | Read one little-endian word. |
| `mem.read32(addr)` | Read one little-endian long. |
| `mem.write8(addr, value)` | Write one byte. |
| `mem.write16(addr, value)` | Write one word. |
| `mem.write32(addr, value)` | Write one long. |
| `mem.read_block(addr, count)` | Return `count` bytes as a string. |
| `mem.write_block(addr, data)` | Write the bytes of `data`. |
| `mem.fill(addr, count, byte)` | Fill a range with one byte. |

If a raw RAM read or write is attempted while the CPU is not
frozen, the script raises an error. Use `cpu.freeze()` before the
access and `cpu.resume()` afterwards.

## 33.6 Terminal Module

`term` drives keyboard, mouse, and terminal text:

| Function | Purpose |
|----------|---------|
| `term.type(text)` | Type text into the terminal. |
| `term.type_line(text)` | Type text followed by Return. |
| `term.read()` | Read pending terminal output. |
| `term.clear()` | Clear terminal output. |
| `term.echo(on)` | Enable or disable local echo. |
| `term.wait_output(text, timeout)` | Wait for text to appear. |
| `term.mouse_move(x, y)` | Move the mouse pointer. |
| `term.mouse_delta(dx, dy)` | Move the mouse pointer by a delta. |
| `term.mouse_click(button)` | Press and release a mouse button. |
| `term.mouse_release(button)` | Release a mouse button. |
| `term.scancode(code)` | Inject a keyboard scancode. |
| `term.key_press(code)` | Press and release a key. |

## 33.7 Audio Module

`audio` controls the mixer and supported playback engines:

| Function group | Purpose |
|----------------|---------|
| `audio.start()`, `audio.stop()`, `audio.reset()` | Control audio output. |
| `audio.freeze()`, `audio.resume()` | Pause and resume audio processing. |
| `audio.write_reg(addr, value)` | Write an audio MMIO register. |
| `audio.set_master_gain_db(v)` | Set master gain in dB. |
| `audio.get_master_gain_db()` | Read master gain in dB. |
| `audio.set_master_auto_level_enabled(on)` | Enable automatic master levelling. |
| `audio.set_master_compressor_enabled(on)` | Enable master compression. |
| `audio.psg_load/play/stop/is_playing/metadata` | PSG playback helpers. |
| `audio.sid_load/play/stop/is_playing/metadata` | SID playback helpers. |
| `audio.ted_load/play/stop/is_playing` | TED playback helpers. |
| `audio.pokey_load/play/stop/is_playing` | POKEY playback helpers. |
| `audio.ahx_load/play/stop/is_playing` | AHX playback helpers. |

## 33.8 Video Module

`video` controls display chips, blitter operations, and frame
inspection:

| Function group | Purpose |
|----------------|---------|
| `video.write_reg(addr, value)`, `video.read_reg(addr)` | Raw video MMIO. |
| `video.get_dimensions()`, `video.is_enabled()` | Current VideoChip state. |
| `video.vga_enable(on)`, `video.vga_set_mode(mode)` | VGA control. |
| `video.vga_set_palette(i, r, g, b)` | VGA palette write. |
| `video.ula_enable(on)`, `video.ula_border(n)` | ULA control. |
| `video.antic_enable(on)`, `video.antic_dlist(addr)` | ANTIC control. |
| `video.gtia_color(i, value)` | GTIA colour register write. |
| `video.ted_enable(on)`, `video.ted_mode(a, b)` | TED control. |
| `video.voodoo_enable(on)`, `video.voodoo_draw()` | Voodoo control. |
| `video.copper_enable(on)`, `video.copper_set_program(addr)` | Copper control. |
| `video.blit_copy(...)`, `video.blit_fill(...)`, `video.blit_line(...)` | Blitter commands. |
| `video.blit_wait()` | Wait until the blitter is idle. |
| `video.get_pixel(x, y)` | Return one composited RGBA pixel. |
| `video.get_region(x, y, w, h)` | Return a rectangle of composited RGBA bytes. |
| `video.frame_hash()` | Hash the current frame. |
| `video.wait_pixel(...)` | Wait for one pixel to match. |
| `video.wait_stable(frames, timeout)` | Wait for a stable frame hash. |
| `video.wait_condition(fn, timeout)` | Wait until callback `fn` returns true. |

## 33.9 Recording Module

`rec` captures frames:

| Function | Purpose |
|----------|---------|
| `rec.screenshot(name)` | Save one frame. |
| `rec.start(name)` | Start recording. |
| `rec.start_screen(name)` | Start screen recording. |
| `rec.stop()` | Stop and finalise recording. |
| `rec.is_recording()` | Return true while recording. |
| `rec.frame_count()` | Number of recorded frames. |

## 33.10 Debug Module

`dbg` drives IE Mon from a script:

| Function group | Purpose |
|----------------|---------|
| `dbg.open()`, `dbg.close()`, `dbg.is_open()` | Control monitor visibility. |
| `dbg.step()`, `dbg.continue()`, `dbg.run_until(addr)` | Execution control. |
| `dbg.set_bp(addr)`, `dbg.clear_bp(addr)`, `dbg.list_bp()` | Breakpoints. |
| `dbg.set_wp(addr)`, `dbg.clear_wp(addr)`, `dbg.list_wp()` | Watchpoints. |
| `dbg.get_reg(name)`, `dbg.set_reg(name, value)` | Register access. |
| `dbg.get_pc()`, `dbg.set_pc(addr)` | Program counter access. |
| `dbg.read_mem(addr, n)`, `dbg.write_mem(addr, data)` | Memory access through the monitor. |
| `dbg.disasm(addr, count)` | Disassemble instructions. |
| `dbg.backtrace()` | Return a stack backtrace. |
| `dbg.timeline(count)` | Return recent timeline entries. |
| `dbg.on_fault(kind, fn)` | Call `fn` when a selected fault occurs. |
| `dbg.poll_faults()` | Poll pending fault events. |
| `dbg.command(line)` | Run one IE Mon command. |

Fault callbacks receive a table with `cpu_id`, `pc`, `addr`,
`kind`, and `info` fields.

## 33.11 Symbols, Regions, and Bits

| Module | Useful functions |
|--------|------------------|
| `sym` | `add`, `lookup`, `resolve`, `list` |
| `regions` | `list`, `lookup` |
| `bit32` | `band`, `bor`, `bxor`, `bnot`, `lshift`, `rshift`, `arshift`, `lrotate`, `rrotate`, `btest`, `extract`, `replace` |

## 33.12 Runnable Video Example

Store this as `FIRST.IES`, then run `RUN "FIRST.IES"` from BASIC
or `script FIRST.IES` from IE Mon:

```ies
-- Draw a blue VideoChip field with a green diagonal.
sys.print("IE SCRIPT VIDEO")
video.write_reg(983044, 4)
video.write_reg(983172, 1048576)
video.write_reg(983040, 1)
video.blit_fill(1048576, 320, 200, 255, 1280)
video.blit_line(0, 0, 319, 199, 65280)
video.blit_wait()
sys.print("BLT " .. video.read_reg(983108))
sys.quit()
```

The comment marks the visible job before the device writes begin.
The script sets VideoChip mode `4` (`320` by `200`), selects
framebuffer base `$100000`, enables the chip, fills the
framebuffer, draws a diagonal, waits for the blitter, and prints
the blitter status. The expected status is `BLT 2`, meaning DONE
set and ERR clear. Try changing `65280` to `16711680` in the
`video.blit_line` call; the diagonal changes from green to red.

## 33.13 Runnable Audio Example

This companion script uses the same bus-facing style for sound. It
programs SoundChip channel `0` through the flexible-channel
registers, waits for a short time, and prints back the control
register so you can see the gate bit that was written.

Store this as `TONE.IES`:

```ies
-- SoundChip channel 0, square wave at about middle C.
audio.start()
audio.write_reg(0xF0A80, 262 * 256)
audio.write_reg(0xF0A84, 96)
audio.write_reg(0xF0AA4, 0)
audio.write_reg(0xF0A88, 3)
sys.wait_ms(250)
sys.print("CH0 " .. mem.read32(0xF0A88))
sys.quit()
```

`audio.start()` enables the global audio path. The frequency
register uses 16.8 fixed-point hertz, so `262 * 256` means about
`262` Hz. The volume write sets an audible level, `WAVE_TYPE` `0`
selects a square wave, and control value `3` means enabled plus
gate. The print should include `CH0 3`.

## 33.14 Fault Callback Example

This pattern records the program counter for any IE64 illegal
instruction fault and then exits cleanly after a short wait:

```ies
local seen = 0

dbg.on_fault("ie64.illegal", function(ev)
  seen = ev.pc
  sys.print("FAULT PC " .. seen)
end)

sys.wait_ms(100)
sys.quit()
```

## 33.15 Limits and Error Behaviour

Scripts run cooperatively. A script that loops forever without
calling a wait function cannot be cancelled promptly.

Storage helpers are limited to approved script storage. Names that
escape that storage are rejected before any read or write occurs.

Raw RAM access through `mem` requires `cpu.freeze()`. MMIO access
does not. If a script fails after freezing a CPU, audio, or the
monitor, IE Script releases those holds before returning control.

## 33.16 What Comes Next

Part IV ends here. Part V covers persistent storage, machine
control commands, input MMIO, and the serial interface.
