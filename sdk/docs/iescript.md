# IEScript Lua Automation Manual

IEScript is the Lua automation layer for Intuition Engine. It allows scripted control of CPU execution, memory, terminal I/O, audio/video devices, monitor debugging, media playback, coprocessor workflows, and recording.

Scripts use the `.ies` extension.

## Contents

1. [Overview](#overview)
2. [Getting Started](#getting-started)
3. [Launch Modes](#launch-modes)
4. [Script Runtime Model](#script-runtime-model)
5. [Safety and Concurrency Rules](#safety-and-concurrency-rules)
6. [Module Reference](#module-reference)
   - [sys](#sys) — Timing, diagnostics, lifecycle
   - [cpu](#cpu) — CPU lifecycle and mode
   - [mem](#mem) — Memory/bus operations
   - [term](#term) — Terminal automation
   - [audio](#audio) — Sound chip and player controls
   - [video](#video) — Video chip, VGA, ULA, ANTIC/GTIA, TED, Voodoo, Copper, Blitter
   - [repl](#repl) — REPL overlay control (show/hide, print, scroll)
   - [rec](#rec) — Recording and screenshots
   - [dbg](#dbg) — Monitor/debugger integration
   - [coproc](#coproc) — Coprocessor manager
   - [media](#media) — Format-agnostic media loader
   - [bit32](#bit32) — Bitwise operations
7. [Recording and Screenshots](#recording-and-screenshots)
8. [Lua REPL Overlay (F8)](#lua-repl-overlay-f8)
9. [EhBASIC Integration](#ehbasic-integration)
10. [Worked Examples](#worked-examples)
11. [Troubleshooting](#troubleshooting)
12. [Quick Reference](#quick-reference)

## Overview

IEScript is designed for deterministic demo automation and tooling:

- drive boot flows and REPL input
- orchestrate debug sessions
- run chip visual/audio transitions
- capture reproducible screenshots and MP4 output

The runtime uses GopherLua (Lua 5.1-compatible semantics).

## Getting Started

IEScript is built in — no separate install required. Create a file with the `.ies` extension and pass it on the command line:

```bash
./bin/IntuitionEngine -script demo.ies
```

A minimal script:

```lua
sys.print("Hello from IEScript!")
sys.wait_frames(60)  -- wait one second at 60 fps
sys.print("fps:", sys.fps())
```

## Launch Modes

### CLI file execution

```bash
./bin/IntuitionEngine -script demo.ies
./bin/IntuitionEngine -ie64 program.ie64 -script demo.ies
./bin/IntuitionEngine -headless -script render.ies
```

### EhBASIC dispatch

From EhBASIC:

```basic
RUN "demo.ies"
```

ProgramExecutor recognises `.ies` and routes it through the external launcher path.

### In-window Lua REPL

Press `F8` to toggle the Lua REPL overlay.

## Script Runtime Model

Scripts run asynchronously alongside the emulator in a dedicated goroutine. Yield helpers (`sys.wait_frames`, `sys.wait_ms`, visual waits) are frame/timer synchronisation points.

### Frame channel

The compositor calls back into the script engine on every completed frame. This callback sends a notification on an internal channel (capacity 1, non-blocking). When the channel is already full the notification is dropped — this means if script execution between yields takes longer than a frame period, frames are silently skipped rather than queued.

### Timing patterns

| Pattern | Mechanism | Use case |
|---------|-----------|----------|
| `sys.wait_frames(n)` | Blocks until `n` compositor frame callbacks | Frame-accurate sequencing |
| `sys.wait_ms(ms)` | Blocks on a wall-clock timer | Time-based delays independent of frame rate |
| `video.wait_pixel(...)` | Polls each frame until pixel matches | Visual synchronisation |
| `video.wait_stable(...)` | Polls each frame until hash unchanged | Wait for rendering to settle |
| `video.wait_condition(...)` | Polls each frame, calls user function | Arbitrary visual predicates |

### Performance monitoring

Use `sys.frame_time()` to check how many host milliseconds have elapsed since the last yield. If this value consistently exceeds your frame period (e.g. 16 ms at 60 fps), your inter-yield work is too heavy and frames are being dropped.

### Important behaviour

- `sys.wait_frames(1)` waits for one compositor frame callback.
- `sys.frame_count()` reports global compositor frame count.
- `sys.frame_time()` reports elapsed host milliseconds since the last yield point.
- All blocking waits (`wait_frames`, `wait_ms`, visual waits) respect script cancellation — if the script is cancelled they raise a Lua error.

## Safety and Concurrency Rules

### Raw RAM access

Raw RAM access requires freezing:

- `mem.read*`, `mem.write*`, `mem.read_block`, `mem.write_block`, `mem.fill`

Use:

```lua
cpu.freeze()
-- raw memory operations
cpu.resume()
```

### MMIO access

MMIO access is allowed without freezing. This includes device registers and control paths through mapped I/O pages. The `requireFrozenForAddress` check consults the bus I/O address bitmap — if the target address falls within a mapped I/O region, the operation proceeds without a freeze.

### Freeze reference counting

Freeze requests are reference-counted across API surfaces (`cpu.*`, `dbg.*`). One subsystem closing a freeze source does not implicitly clear another active freeze source. Specifically:

- `cpu.freeze()` increments the global freeze counter.
- `cpu.resume()` decrements it (floor of 0 — extra resumes are harmless).
- `dbg.open()` / `dbg.freeze()` activate the monitor *and* increment the freeze counter.
- `dbg.close()` / `dbg.resume()` deactivate the monitor *and* decrement the freeze counter.

## Module Reference

---

## `sys`

Timing, diagnostics, lifecycle.

`sys.wait_frames(n)` — Block until `n` compositor frames have completed. Yields to the frame channel on each frame. Returns: nothing.

`sys.wait_ms(ms)` — Block for `ms` milliseconds (wall-clock timer). Returns: nothing.

`sys.print(...)` — Print all arguments to host stdout, space-separated, with trailing newline. Returns: nothing.

`sys.log(...)` — Log all arguments to host stdout (mirrors `sys.print` currently). Returns: nothing.

`sys.time_ms()` — Current Unix time in milliseconds. Returns: number.

`sys.frame_count()` — Global compositor frame count since engine start. Returns: number.

`sys.frame_time()` — Milliseconds elapsed since the last yield point (wait_frames, wait_ms, or visual wait). Useful for detecting slow scripts. Returns: number.

`sys.fps()` — Current compositor refresh rate in Hz. Returns: number.

`sys.quit()` — Stop any active recording and shut down the emulator. Returns: nothing.

Example:

```lua
sys.print("fps", sys.fps())
sys.wait_frames(120)
sys.print("frame_time:", sys.frame_time(), "ms")
```

---

## `cpu`

CPU lifecycle and mode.

`cpu.load(path)` — Load a program binary from `path` into the active CPU. The file format must match the active CPU mode. Returns: nothing. Raises on error.

`cpu.reset()` — Perform a hard reset of the emulator (all CPUs and devices). Returns: nothing. Raises on error.

`cpu.freeze()` — Increment the global freeze counter, pausing CPU execution for safe memory access. Returns: nothing.

`cpu.resume()` — Decrement the global freeze counter. Execution resumes when the counter reaches zero. Extra resume calls beyond zero are harmless (counter floors at 0). Returns: nothing.

`cpu.start()` — Start execution on the active CPU. Returns: nothing.

`cpu.stop()` — Stop execution on the active CPU. Returns: nothing.

`cpu.is_running()` — Check whether the active CPU is currently executing. Returns: boolean.

`cpu.mode()` — Return the active CPU type as a string. Returns: one of `"ie32"`, `"ie64"`, `"m68k"`, `"z80"`, `"x86"`, `"6502"`, or `"none"`.

Example:

```lua
cpu.load("program.ie32")
cpu.start()
sys.wait_frames(60)
sys.print("CPU mode:", cpu.mode(), "running:", cpu.is_running())
cpu.freeze()
-- safe to read/write memory here
cpu.resume()
```

---

## `mem`

Memory/bus operations. All `mem.*` functions require `cpu.freeze()` for raw RAM addresses. MMIO addresses (device registers) are allowed without freezing.

`mem.read8(addr)` — Read one byte from bus address `addr`. Returns: number (0..255).

`mem.read16(addr)` — Read a 16-bit word from bus address `addr`. Returns: number.

`mem.read32(addr)` — Read a 32-bit word from bus address `addr`. Returns: number.

`mem.write8(addr, value)` — Write one byte `value` to bus address `addr`. Returns: nothing.

`mem.write16(addr, value)` — Write a 16-bit word `value` to bus address `addr`. Returns: nothing.

`mem.write32(addr, value)` — Write a 32-bit word `value` to bus address `addr`. Returns: nothing.

`mem.read_block(addr, len)` — Read `len` bytes starting at `addr`. Returns: string (raw bytes).

`mem.write_block(addr, bytes)` — Write the raw byte string `bytes` starting at `addr`. Returns: nothing.

`mem.fill(addr, len, value)` — Fill `len` bytes starting at `addr` with byte `value`. Returns: nothing.

Example:

```lua
cpu.freeze()
local val = mem.read32(0x1000)
sys.print("value at 0x1000:", val)
mem.write8(0x2000, 0xFF)
mem.fill(0x3000, 256, 0)
local block = mem.read_block(0x1000, 16)
cpu.resume()
```

---

## `term`

Terminal automation for driving the emulated terminal I/O.

`term.type(str)` — Enqueue each byte of `str` as keyboard input to the terminal. Does not append a newline. Returns: nothing.

`term.type_line(str)` — Enqueue `str` followed by a newline character. Returns: nothing.

`term.read()` — Drain and return all pending terminal output. Returns: string.

`term.clear()` — Drain and discard all pending terminal output. Returns: nothing.

`term.echo(on)` — Enable or disable terminal echo. Returns: nothing.

`term.wait_output(pattern, timeout_ms)` — Poll terminal output every 10 ms until `pattern` (a plain string, not a regex) is found or `timeout_ms` expires. Accumulates output across polls. Returns: boolean (`true` if pattern found, `false` on timeout).

Example:

```lua
term.type_line('PRINT "HELLO"')
local ok = term.wait_output("HELLO", 2000)
if not ok then
  error("expected output not seen")
end
```

---

## `audio`

Sound chip and player controls.

### Core

`audio.start()` — Start the sound chip. Returns: nothing.

`audio.stop()` — Stop the sound chip. Returns: nothing.

`audio.reset()` — Reset the sound chip to initial state. Returns: nothing.

`audio.freeze()` — Freeze audio generation (silence output). Returns: nothing.

`audio.resume()` — Resume audio generation after a freeze. Returns: nothing.

`audio.write_reg(addr, value)` — Write a 32-bit `value` to sound chip register at bus address `addr`. This is an MMIO write (no freeze required). Returns: nothing.

### PSG (AY-3-8910 / YM2149)

`audio.psg_load(path)` — Load a PSG music file (VGM, VTX, PT3, PT2, PT1, STC, SQT, ASC, FTC). Returns: nothing. Raises on error.

`audio.psg_play()` — Start PSG playback. Returns: nothing.

`audio.psg_stop()` — Stop PSG playback. Returns: nothing.

`audio.psg_is_playing()` — Check whether the PSG engine is currently playing. Returns: boolean.

`audio.psg_metadata()` — Return metadata for the currently loaded PSG file. Returns: table with fields:

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Track title |
| `author` | string | Composer name |
| `system` | string | Target system |

### SID (MOS 6581/8580)

`audio.sid_load(path [, subsong])` — Load a SID music file. The optional `subsong` parameter selects a sub-song index (default 0). Returns: nothing. Raises on error.

`audio.sid_play()` — Start SID playback. Returns: nothing.

`audio.sid_stop()` — Stop SID playback. Returns: nothing.

`audio.sid_is_playing()` — Check whether the SID player is currently playing. Returns: boolean.

`audio.sid_metadata()` — Return metadata for the currently loaded SID file. Returns: table with fields:

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Track title |
| `author` | string | Composer name |
| `released` | string | Release information |
| `duration` | string | Duration text |

### TED (MOS 7360/8360)

`audio.ted_load(path)` — Load a TED music file. Returns: nothing. Raises on error.

`audio.ted_play()` — Start TED playback. Returns: nothing.

`audio.ted_stop()` — Stop TED playback. Returns: nothing.

`audio.ted_is_playing()` — Check whether the TED player is currently playing. Returns: boolean.

### POKEY (Atari C012294)

`audio.pokey_load(path)` — Load a POKEY music file (SAP). Returns: nothing. Raises on error.

`audio.pokey_play()` — Start POKEY playback. Returns: nothing.

`audio.pokey_stop()` — Stop POKEY playback. Returns: nothing.

`audio.pokey_is_playing()` — Check whether the POKEY player is currently playing. Returns: boolean.

### AHX (Abyss' Highest eXperience)

`audio.ahx_load(path)` — Load an AHX music file. Returns: nothing. Raises on error.

`audio.ahx_play()` — Start AHX playback. Returns: nothing.

`audio.ahx_stop()` — Stop AHX playback. Returns: nothing.

`audio.ahx_is_playing()` — Check whether the AHX engine is currently playing. Returns: boolean.

Example:

```lua
audio.psg_load("music/track.vgm")
audio.psg_play()
sys.wait_frames(300)
local meta = audio.psg_metadata()
sys.print("Now playing:", meta.title, "by", meta.author)
audio.psg_stop()
```

---

## `video`

Video chip, VGA, ULA, ANTIC/GTIA, TED, Voodoo 3D, Copper coprocessor, Blitter, and frame inspection.

### General

`video.write_reg(addr, value)` — Write a 32-bit `value` to a video register at bus address `addr` (MMIO). Returns: nothing.

`video.read_reg(addr)` — Read a 32-bit value from a video register at bus address `addr`. Returns: number.

`video.get_dimensions()` — Get the compositor output dimensions. Returns: width, height (two numbers).

`video.is_enabled()` — Check whether the primary VideoChip is enabled. Returns: boolean.

### VGA

`video.vga_enable(on)` — Enable or disable the VGA output. Returns: nothing.

`video.vga_set_mode(mode)` — Set the VGA video mode (e.g. 0x13 for Mode 13h). Returns: nothing.

`video.vga_set_palette(idx, r, g, b)` — Set VGA palette entry `idx` (0..255) to the given RGB values (each 0..255). Returns: nothing.

`video.vga_get_palette(idx)` — Read VGA palette entry `idx`. Returns: r, g, b (three numbers, each 0..255).

`video.vga_get_dimensions()` — Get VGA framebuffer dimensions. Returns: width, height.

### ULA (ZX Spectrum)

`video.ula_enable(on)` — Enable or disable ULA video output. Returns: nothing.

`video.ula_is_enabled()` — Check whether ULA is enabled. Returns: boolean.

`video.ula_border(colour)` — Set the ULA border colour (0..7). Returns: nothing.

`video.ula_get_dimensions()` — Get ULA display dimensions. Returns: width, height.

### ANTIC (Atari 8-bit)

`video.antic_enable(on)` — Enable or disable ANTIC video output. Returns: nothing.

`video.antic_is_enabled()` — Check whether ANTIC is enabled. Returns: boolean.

`video.antic_dlist(addr)` — Set the ANTIC display list address. Returns: nothing.

`video.antic_dma(flags)` — Set ANTIC DMA control flags (DMACTL register, 0..255). Returns: nothing.

`video.antic_scroll(h, v)` — Set ANTIC horizontal and vertical scroll values (each 0..15). Returns: nothing.

`video.antic_charset(page)` — Set ANTIC character set base page (CHBASE register, 0..255). Returns: nothing.

`video.antic_pmbase(page)` — Set ANTIC player/missile base page (PMBASE register, 0..255). Returns: nothing.

`video.antic_get_dimensions()` — Get ANTIC display dimensions. Returns: width, height.

### GTIA (Atari 8-bit)

`video.gtia_color(reg, value)` — Set a GTIA colour register. Register indices: 0=COLPF0, 1=COLPF1, 2=COLPF2, 3=COLPF3, 4=COLBK, 5=COLPM0, 6=COLPM1, 7=COLPM2, 8=COLPM3. Value is 0..255 (Atari hue/luminance byte). Returns: nothing.

`video.gtia_player_pos(player, x)` — Set horizontal position of player sprite `player` (0..3) to `x` (0..255). Returns: nothing.

`video.gtia_player_size(player, size)` — Set width of player sprite `player` (0..3). Size: 0=normal, 1=double, 3=quad. Returns: nothing.

`video.gtia_player_gfx(player, data)` — Set graphics data byte for player sprite `player` (0..3). Returns: nothing.

`video.gtia_priority(value)` — Set GTIA priority register (PRIOR, 0..255). Returns: nothing.

### TED Video (Commodore Plus/4)

`video.ted_enable(on)` — Enable or disable TED video output. Returns: nothing.

`video.ted_is_enabled()` — Check whether TED video is enabled. Returns: boolean.

`video.ted_mode(ctrl1, ctrl2)` — Set TED control registers 1 and 2 (each 0..255). Returns: nothing.

`video.ted_colors(bg0, bg1, bg2, bg3, border)` — Set all five TED colour registers (each 0..127, TED colour format). Returns: nothing.

`video.ted_charset(page)` — Set TED character set base page (0..255). Returns: nothing.

`video.ted_video_base(page)` — Set TED video memory base page (0..255). Returns: nothing.

`video.ted_cursor(pos, colour)` — Set TED cursor position (0..65535) and colour (0..127). Returns: nothing.

`video.ted_get_dimensions()` — Get TED display dimensions. Returns: width, height.

### Voodoo 3D

Voodoo functions accept integer pixel coordinates for vertices and 0..255 byte values for colours. Internally, vertex coordinates are converted to 12.4 fixed-point and colours to 12.12 fixed-point.

`video.voodoo_enable(on)` — Enable or disable Voodoo 3D rendering. Returns: nothing.

`video.voodoo_is_enabled()` — Check whether Voodoo is enabled. Returns: boolean.

`video.voodoo_resolution(w, h)` — Set the Voodoo framebuffer resolution. Returns: nothing.

`video.voodoo_vertex(ax, ay, bx, by, cx, cy)` — Set the three triangle vertex positions in integer pixel coordinates. Returns: nothing.

`video.voodoo_color(idx, r, g, b, a)` — Set vertex colour for vertex `idx` (0..2). RGBA values are 0..255. Returns: nothing.

`video.voodoo_depth(z)` — Set the starting depth value for the triangle (integer, converted to 20.12 fixed-point). Returns: nothing.

`video.voodoo_texcoord(s, t, w)` — Set texture coordinates. `s` and `t` are floating-point texture coordinates; `w` is the perspective correction factor. Returns: nothing.

`video.voodoo_draw()` — Submit the current triangle for rasterisation. Returns: nothing.

`video.voodoo_swap()` — Swap the Voodoo front and back buffers. Returns: nothing.

`video.voodoo_clear(r, g, b)` — Clear the Voodoo framebuffer with the given RGB colour (each 0..255). Returns: nothing.

`video.voodoo_fog(on, r, g, b)` — Enable/disable fog and set the fog colour. Returns: nothing.

`video.voodoo_alpha(mode)` — Set the Voodoo alpha blending mode register. Returns: nothing.

`video.voodoo_zbuffer(mode)` — Set the Voodoo depth buffer mode (FBZ_MODE register). Returns: nothing.

`video.voodoo_clip(left, right, top, bottom)` — Set the Voodoo clip rectangle. Returns: nothing.

`video.voodoo_texture(w, h, data)` — Upload texture data. `w` and `h` are the texture dimensions; `data` is a raw byte string of pixel data. Returns: nothing.

`video.voodoo_chromakey(on, r, g, b)` — Enable/disable chroma keying and set the key colour. Returns: nothing.

`video.voodoo_dither(on)` — Enable or disable dithering. Returns: nothing.

`video.voodoo_get_dimensions()` — Get Voodoo framebuffer dimensions. Returns: width, height.

### Copper Coprocessor

`video.copper_enable(on)` — Enable or disable the copper coprocessor. Returns: nothing.

`video.copper_set_program(addr)` — Set the copper program pointer to bus address `addr`. Returns: nothing.

`video.copper_is_running()` — Check whether the copper is currently executing. Returns: boolean.

### Blitter

`video.blit_copy(src, dst, w, h, src_stride, dst_stride)` — Start a blitter copy operation. Copies a `w`x`h` rectangle from `src` to `dst` with the given strides. Returns: nothing.

`video.blit_fill(dst, w, h, colour, dst_stride)` — Start a blitter fill operation. Fills a `w`x`h` rectangle at `dst` with `colour` (32-bit RGBA). Returns: nothing.

`video.blit_line(x0, y0, x1, y1, colour)` — Draw a line from (`x0`,`y0`) to (`x1`,`y1`) with `colour` (32-bit RGBA). Returns: nothing.

`video.blit_wait()` — Block until the blitter is idle. Polls every 1 ms. Returns: nothing.

### Frame Inspection

`video.get_pixel(x, y)` — Read a pixel from the current compositor frame. Returns: r, g, b, a (four numbers, each 0..255). Returns all zeros if coordinates are out of bounds.

`video.get_region(x, y, w, h)` — Read a rectangular region from the current compositor frame. Returns: string (raw RGBA bytes, row-major, 4 bytes per pixel). Returns empty string if region is invalid.

`video.frame_hash()` — Compute an FNV-1a hash of the current compositor frame. Returns: number. Returns 0 if no frame is available.

### Visual Waits

All visual waits block on the frame channel (yielding per frame) and respect script cancellation.

`video.wait_pixel(x, y, r, g, b, timeout_ms)` — Wait until the pixel at (`x`,`y`) matches the target RGB colour within a tolerance of +/-2 per channel, or until `timeout_ms` expires. Returns: boolean (`true` if matched, `false` on timeout).

`video.wait_stable(n_frames, timeout_ms)` — Wait until the compositor frame hash remains unchanged for `n_frames` consecutive frames, or until `timeout_ms` expires. Useful for waiting until rendering has settled. Returns: boolean.

`video.wait_condition(fn, timeout_ms)` — Call the Lua function `fn` once per frame. If `fn` returns `true`, the wait succeeds. Continues until `fn` returns `true` or `timeout_ms` expires. Returns: boolean.

Example — Voodoo triangle:

```lua
video.voodoo_enable(true)
video.voodoo_resolution(320, 240)
video.voodoo_clear(0, 0, 64)
video.voodoo_vertex(160, 10, 10, 230, 310, 230)
video.voodoo_color(0, 255, 0, 0, 255)
video.voodoo_color(1, 0, 255, 0, 255)
video.voodoo_color(2, 0, 0, 255, 255)
video.voodoo_draw()
video.voodoo_swap()
```

---

## `repl`

Programmatic control of the Lua REPL overlay. Use this module from scripts to display information, title cards, or scrolling text on-screen without affecting the underlying emulator display.

`repl.show()` — Show the REPL overlay. Returns: nothing.

`repl.hide()` — Hide the REPL overlay. Returns: nothing.

`repl.is_open()` — Check whether the overlay is currently visible. Returns: boolean.

`repl.print(text)` — Append a line of text to the overlay output buffer. Returns: nothing.

`repl.clear()` — Clear the overlay output buffer. Returns: nothing.

`repl.scroll_up(n)` — Scroll the overlay output up by `n` lines. Returns: nothing.

`repl.scroll_down(n)` — Scroll the overlay output down by `n` lines. Returns: nothing.

`repl.line_count()` — Get the total number of lines in the overlay output buffer. Returns: number.

Example — title card:

```lua
repl.show()
repl.clear()
repl.print("  ================================================")
repl.print("  Intuition Engine Demo")
repl.print("  ================================================")
sys.wait_ms(3000)
repl.hide()
```

Example — scrolling source code listing:

```lua
local f = io.open("source.bas", "r")
if f then
    repl.show(); repl.clear()
    for line in f:lines() do repl.print(line) end
    f:close()
    repl.scroll_up(repl.line_count())
    for _ = 1, repl.line_count() do
        repl.scroll_down(1)
        sys.wait_ms(60)
    end
    sys.wait_ms(1500)
    repl.hide()
end
```

---

## `rec`

Recording and screenshot capture.

`rec.screenshot(path)` — Capture the current compositor frame as a PNG file at `path`. Pure Go implementation — no external dependencies. Returns: nothing. Raises on error.

`rec.start(path)` — Start recording video (and audio) to an MP4 file at `path`. Requires FFmpeg in `PATH`. Returns: nothing. Raises on error.

`rec.stop()` — Stop an active recording and finalise the file. Returns: nothing. Raises on error.

`rec.is_recording()` — Check whether a recording is in progress. Returns: boolean.

`rec.frame_count()` — Number of frames captured in the current recording session. Returns: number.

Example:

```lua
rec.screenshot("before.png")
rec.start("demo.mp4")
sys.wait_frames(300)  -- record 5 seconds at 60 fps
rec.stop()
sys.print("Recorded", rec.frame_count(), "frames")
rec.screenshot("after.png")
```

---

## `dbg`

Monitor/debugger integration. Most functions require the Machine Monitor to be available (set via `-monitor` or programmatically).

### Core

`dbg.open()` — Activate the monitor and increment the freeze counter. This is the standard way to enter a debug session from a script. Returns: nothing.

`dbg.close()` — Deactivate the monitor and decrement the freeze counter. Returns: nothing.

`dbg.is_open()` — Check whether the monitor is currently active. Returns: boolean.

`dbg.freeze()` — Alias for `dbg.open()`. Activates the monitor and increments the freeze counter. Returns: nothing.

`dbg.resume()` — Alias for `dbg.close()`. Deactivates the monitor and decrements the freeze counter. Returns: nothing.

### Execution Control

`dbg.step([n])` — Single-step the focused CPU by `n` instructions (default 1). Returns: nothing.

`dbg.continue()` — Resume execution on the focused CPU (equivalent to monitor `g` command). Returns: nothing.

`dbg.run_until(addr)` — Run the focused CPU until it reaches address `addr`. Returns: nothing.

`dbg.backstep()` — Step the focused CPU backward by one instruction (if trace history is available). Returns: nothing.

### Breakpoints

`dbg.set_bp(addr)` — Set an unconditional breakpoint at address `addr`. Returns: nothing.

`dbg.set_conditional_bp(addr, condition)` — Set a conditional breakpoint at `addr` with condition string `condition` (e.g. `"A==$FF"`). Returns: nothing.

`dbg.clear_bp(addr)` — Remove the breakpoint at address `addr`. Returns: nothing.

`dbg.clear_all_bp()` — Remove all breakpoints on the focused CPU. Returns: nothing.

`dbg.list_bp()` — List all breakpoints on the focused CPU. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `addr` | number | Breakpoint address |
| `condition` | string | Condition expression (empty if unconditional) |
| `hit_count` | number | Number of times this breakpoint has been hit |

### Watchpoints

`dbg.set_wp(addr)` — Set a watchpoint (memory write watch) at address `addr`. Returns: nothing.

`dbg.clear_wp(addr)` — Remove the watchpoint at address `addr`. Returns: nothing.

`dbg.clear_all_wp()` — Remove all watchpoints on the focused CPU. Returns: nothing.

`dbg.list_wp()` — List all watchpoint addresses. Returns: table (array) of numbers.

### Registers

`dbg.get_reg(name)` — Read a CPU register by name (e.g. `"A"`, `"PC"`, `"SP"`). Returns: number, or `nil` if the register name is unknown.

`dbg.set_reg(name, value)` — Write a value to a CPU register by name. Returns: nothing. Raises on unknown register.

`dbg.get_regs()` — Read all CPU registers. Returns: table `{name = value, ...}`.

`dbg.get_pc()` — Read the program counter. Returns: number.

`dbg.set_pc(addr)` — Set the program counter to `addr`. Returns: nothing.

### Memory

`dbg.read_mem(addr, len)` — Read `len` bytes from the focused CPU's memory at `addr`. Returns: string (raw bytes).

`dbg.write_mem(addr, data)` — Write raw byte string `data` to the focused CPU's memory at `addr`. Returns: nothing.

`dbg.fill_mem(addr, len, value)` — Fill `len` bytes starting at `addr` with byte `value`. Returns: nothing.

`dbg.hunt_mem(start, len, pattern)` — Search for byte pattern `pattern` within `len` bytes starting at `start`. Returns: table (array) of matching addresses.

`dbg.compare_mem(start, len, dest)` — Compare `len` bytes between `start` and `dest`, reporting differences. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `offset` | number | Byte offset where difference was found |
| `val1` | number | Byte value at `start + offset` |
| `val2` | number | Byte value at `dest + offset` |

`dbg.transfer_mem(start, len, dest)` — Copy `len` bytes from `start` to `dest` (safe for overlapping regions). Returns: nothing.

### Disassembly and Trace

`dbg.disasm(addr, count)` — Disassemble `count` instructions starting at `addr`. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `addr` | number | Instruction address |
| `hex` | string | Raw instruction bytes in hex |
| `mnemonic` | string | Disassembled instruction text |

`dbg.trace(n)` — Execute `n` instructions on the focused CPU, recording each step. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `addr` | number | Instruction address |
| `mnemonic` | string | Disassembled instruction text |
| `reg_changes` | table | Register changes (currently empty table) |

`dbg.backtrace([depth])` — Return a call stack backtrace up to `depth` frames (default 8). Returns: table (array) of strings.

`dbg.trace_file(path)` — Start logging execution trace to file at `path`. Returns: nothing.

`dbg.trace_file_off()` — Stop trace file logging. Returns: nothing.

`dbg.trace_watch_add(addr)` — Add a memory address to the trace watch list. Returns: nothing.

`dbg.trace_watch_del(addr)` — Remove a memory address from the trace watch list. Returns: nothing.

`dbg.trace_watch_list()` — List all trace watch addresses. Returns: table (array) of numbers.

`dbg.trace_history(addr_str)` — Get the write history for a memory address. Pass the address as a hex string (e.g. `"$1000"`). Passing `"*"` returns an empty table (per-address query only). Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `pc` | number | Program counter at time of write |
| `old_val` | number | Previous value |
| `new_val` | number | New value written |

`dbg.trace_history_clear(addr)` — Clear write history for address `addr` (string, e.g. `"$1000"` or `"*"`). Returns: nothing.

### State Save/Load

`dbg.save_state(path)` — Save the current machine state to file at `path`. Returns: nothing.

`dbg.load_state(path)` — Restore machine state from file at `path`. Returns: nothing.

`dbg.save_mem_file(start, length, path)` — Save `length` bytes starting at `start` to a binary file at `path`. Returns: nothing.

`dbg.load_mem_file(path, addr)` — Load a binary file from `path` into memory at `addr`. Returns: nothing.

### Multi-CPU

`dbg.cpu_list()` — List all registered CPUs. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | number | CPU identifier |
| `label` | string | CPU label/name |
| `cpu_name` | string | CPU architecture name |
| `is_running` | boolean | Whether the CPU is currently running |

`dbg.cpu_focus(id)` — Switch monitor focus to a CPU by numeric `id` or string label. Returns: nothing.

`dbg.freeze_cpu(label)` — Freeze a specific CPU by label. Returns: nothing.

`dbg.thaw_cpu(label)` — Thaw (resume) a specific CPU by label. Returns: nothing.

`dbg.freeze_all()` — Freeze all CPUs. Returns: nothing.

`dbg.thaw_all()` — Thaw all CPUs. Returns: nothing.

### Audio Debug

`dbg.freeze_audio()` — Freeze audio generation (silence). Returns: nothing.

`dbg.thaw_audio()` — Resume audio generation. Returns: nothing.

### I/O Inspection

`dbg.io_devices()` — List all available I/O device names. Returns: table (array) of strings.

`dbg.io(device)` — Read all registers for the named I/O device. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Register name |
| `addr` | number | Register address |
| `value` | number | Current register value |
| `access` | string | Access mode (e.g. `"RW"`, `"RO"`) |

### Scripting

`dbg.run_script(path)` — Execute a monitor script file at `path`. Returns: nothing.

`dbg.macro(name, cmds)` — Define a monitor macro. `name` is the macro name; `cmds` is the command string. Returns: nothing.

`dbg.command(cmd)` — Execute a raw monitor command string. Returns: nothing.

Example — breakpoint workflow:

```lua
dbg.open()
dbg.set_bp(0x1000)
dbg.continue()
-- execution stops at breakpoint
local pc = dbg.get_pc()
sys.print("Stopped at:", string.format("$%04X", pc))
local regs = dbg.get_regs()
for name, val in pairs(regs) do
  sys.print(name, "=", string.format("$%X", val))
end
local dis = dbg.disasm(pc, 5)
for _, entry in ipairs(dis) do
  sys.print(string.format("$%04X  %s  %s", entry.addr, entry.hex, entry.mnemonic))
end
dbg.clear_all_bp()
dbg.close()
```

---

## `coproc`

Coprocessor manager for offloading work to secondary CPU instances.

Supported CPU types: `"ie32"`, `"6502"`, `"m68k"`, `"z80"`, `"x86"`.

### Ticket lifecycle

1. `coproc.start(cpu_type, filename)` — launch a worker.
2. `coproc.enqueue(cpu_type, op, request)` — submit work, get a ticket ID.
3. `coproc.poll(ticket)` or `coproc.wait(ticket, timeout_ms)` — check/wait for completion.
4. `coproc.response(ticket)` — retrieve the response data.
5. `coproc.stop(cpu_type)` — tear down the worker when done.

### Functions

`coproc.start(cpu_type, filename)` — Start a coprocessor worker of the given `cpu_type`, loading the program from `filename`. Returns: nothing. Raises on error.

`coproc.stop(cpu_type)` — Stop the coprocessor worker for `cpu_type`. Returns: nothing. Raises on error.

`coproc.enqueue(cpu_type, op, request)` — Enqueue a work request. `op` is a numeric opcode; `request` is a raw byte string payload. Returns: number (ticket ID).

`coproc.poll(ticket)` — Check the status of a ticket without blocking. Returns: string — one of `"pending"`, `"running"`, `"ok"`, `"error"`, `"timeout"`.

`coproc.wait(ticket, timeout_ms)` — Block until the ticket completes or `timeout_ms` expires. Returns: status (string), response (string, raw bytes). The response is empty if the ticket did not complete successfully.

`coproc.workers()` — List all active coprocessor workers. Returns: table (array) of entries, each with fields:

| Field | Type | Description |
|-------|------|-------------|
| `cpu_type` | string | CPU type name |
| `is_running` | boolean | Whether the worker is active |

`coproc.response(ticket)` — Retrieve the response data for a ticket. If the ticket completed successfully, returns the response bytes. If the ticket is not found in the response ring but was previously enqueued, returns the raw contents of the preallocated response buffer (which may contain stale or partial data). Returns empty string only if the ticket is entirely unknown. Returns: string (raw bytes).

Example:

```lua
coproc.start("ie32", "worker.ie32")
local ticket = coproc.enqueue("ie32", 1, "input data")
local status, response = coproc.wait(ticket, 5000)
sys.print("Status:", status, "Response length:", #response)
coproc.stop("ie32")
```

---

## `media`

Format-agnostic media loader. Supports SID, PSG/VGM, TED, AHX, POKEY/SAP formats. Unlike the engine-specific `audio.*` functions, `media.*` auto-detects the format and routes to the appropriate player.

`media.load(filename)` — Load and start playing a music file, auto-detecting format. Returns: nothing. Raises only on immediate setup failures (e.g. scratch memory unavailable); format detection and decode errors are reported asynchronously via `media.status()` and `media.error()`.

`media.load_subsong(filename, subsong)` — Load a music file and select a specific sub-song index. Returns: nothing. Same error semantics as `media.load`.

`media.play()` — Resume playback (if paused or after load). Returns: nothing.

`media.stop()` — Stop playback. Returns: nothing.

`media.status()` — Get the current playback status. Returns: string — one of `"idle"`, `"loading"`, `"playing"`, `"error"`.

`media.type()` — Get the detected media type. Returns: string — one of `"sid"`, `"psg"`, `"ted"`, `"ahx"`, `"pokey"`, `"none"`.

`media.error()` — Get the last error code (0 if no error). Returns: number.

Example:

```lua
media.load("music/song.sid")
sys.wait_frames(60)
sys.print("Playing:", media.type(), "Status:", media.status())
sys.wait_frames(600)
media.stop()
```

---

## `bit32`

Lua 5.1 does not include a bitwise library. IEScript provides a `bit32` global table with unsigned 32-bit operations, compatible with the Lua 5.2 `bit32` library interface.

`bit32.band(...)` — Bitwise AND of all arguments. With zero arguments, returns `0xFFFFFFFF`. Returns: number.

`bit32.bor(...)` — Bitwise OR of all arguments. Returns: number.

`bit32.bxor(...)` — Bitwise XOR of all arguments. Returns: number.

`bit32.bnot(x)` — Bitwise NOT (ones complement). Returns: number.

`bit32.lshift(x, disp)` — Logical left shift by `disp` bits (masked to 0..31). Returns: number.

`bit32.rshift(x, disp)` — Logical right shift by `disp` bits (masked to 0..31). Returns: number.

`bit32.arshift(x, disp)` — Arithmetic right shift by `disp` bits (sign-extending). Returns: number.

`bit32.lrotate(x, disp)` — Left rotation by `disp` bits. Returns: number.

`bit32.rrotate(x, disp)` — Right rotation by `disp` bits. Returns: number.

Example:

```lua
local flags = bit32.bor(0x01, 0x04, 0x10)   -- 0x15
local masked = bit32.band(flags, 0x0F)       -- 0x05
local shifted = bit32.lshift(1, 7)           -- 0x80
sys.print(string.format("0x%X", shifted))
```

---

## Recording and Screenshots

### Screenshot

```lua
rec.screenshot("frame.png")
```

Screenshots are pure Go (PNG encoding) — no external tools required.

### Recording

```lua
rec.start("demo.mp4")
sys.wait_frames(300)
rec.stop()
```

Notes:

- FFmpeg must be available in `PATH`.
- Recording uses compositor dimensions/refresh settings.
- Audio is captured via a sample tap on the sound chip — no double-ticking occurs.
- Resolution is locked for the duration of a recording session.
- Recording works in headless mode (`-headless -script render.ies`).

### Full demo recording workflow

```lua
-- Load a program and record a 10-second demo
cpu.load("demo.ie32")
cpu.start()
sys.wait_frames(30)  -- let the demo initialise

rec.start("output.mp4")
sys.wait_frames(600)  -- 10 seconds at 60 fps
rec.stop()

rec.screenshot("final_frame.png")
sys.quit()
```

## Lua REPL Overlay (F8)

Press `F8` to open/close the Lua REPL overlay. The REPL shares the same Lua API as scripts.

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `F8` | Toggle overlay open/close |
| `Esc` | Close overlay |
| `Enter` | Execute current line |
| `Up` / `Down` | Command history navigation |
| `Ctrl+A` | Move cursor to start of line |
| `Ctrl+E` | Move cursor to end of line |
| `Ctrl+K` | Kill text from cursor to end of line |
| `Ctrl+U` | Kill text from start of line to cursor |
| `PgUp` / `PgDn` | Scroll output buffer |
| `Ctrl+Shift+V` | Paste from clipboard |

### Expression shortcut

Type `=expr` as a shortcut for `return expr`:

```
> =sys.fps()
60
> =cpu.mode()
ie64
```

### Multiline input

Incomplete chunks (e.g. an unclosed `function ... end` block) trigger a continuation prompt, allowing multiline input.

### Headless builds

The REPL overlay is not available in headless builds (no display backend). Use `-script` for headless automation instead.

## EhBASIC Integration

`RUN "file.ies"` routes through ProgramExecutor `.ies` detection and then into the external launcher dispatch. This allows script execution without firmware keyword changes.

## Worked Examples

### Basic automation and monitor commands

```lua
term.type_line('PRINT "HELLO FROM LUA"')
term.wait_output("HELLO FROM LUA", 2000)

dbg.open()
dbg.command("r")
dbg.close()
```

### Visual wait

```lua
local ok = video.wait_pixel(10, 10, 255, 0, 0, 3000)
if not ok then
  error("pixel did not reach target colour in time")
end
```

### Voodoo quick draw

```lua
video.voodoo_enable(true)
video.voodoo_resolution(320, 240)
video.voodoo_clear(0, 0, 64)
video.voodoo_vertex(160, 10, 10, 230, 310, 230)
video.voodoo_color(0, 255, 0, 0, 255)
video.voodoo_color(1, 0, 255, 0, 255)
video.voodoo_color(2, 0, 0, 255, 255)
video.voodoo_draw()
video.voodoo_swap()
```

### Full demo recording

```lua
cpu.load("demo.ie32")
cpu.start()
sys.wait_frames(30)

rec.start("demo.mp4")
sys.wait_frames(600)
rec.stop()

rec.screenshot("final.png")
sys.quit()
```

### Monitor debugging workflow

```lua
dbg.open()

-- Set a breakpoint and run to it
dbg.set_bp(0x1000)
dbg.continue()

-- Inspect state at breakpoint
local pc = dbg.get_pc()
sys.print("Hit breakpoint at:", string.format("$%04X", pc))

-- Disassemble around the breakpoint
local dis = dbg.disasm(pc, 10)
for _, d in ipairs(dis) do
  sys.print(string.format("  $%04X  %-12s  %s", d.addr, d.hex, d.mnemonic))
end

-- Read registers
local regs = dbg.get_regs()
for name, val in pairs(regs) do
  sys.print(string.format("  %s = $%X", name, val))
end

-- Single-step a few instructions
dbg.step(3)
sys.print("After 3 steps, PC =", string.format("$%04X", dbg.get_pc()))

-- Clean up
dbg.clear_all_bp()
dbg.close()
```

## Troubleshooting

### `raw memory access requires cpu.freeze()`

Wrap RAM operations with `cpu.freeze()` and `cpu.resume()`.

### `ffmpeg not found in PATH`

Install FFmpeg and ensure the executable is resolvable from your shell session.

### Script appears stalled

- check waits and timeouts (`wait_frames`, `wait_ms`, visual waits)
- print state periodically with `sys.print`
- inspect monitor state via `dbg.command(...)`

### REPL prints but script output not visible

Use `sys.print` for host console output and keep REPL open for in-overlay logs.

### REPL overlay not appearing

The overlay requires a display backend. It is not available in headless builds (`-tags headless` or `make headless`). Use `-script` for headless automation.

### Recording stops unexpectedly

Recording relies on an FFmpeg subprocess. If FFmpeg crashes or is killed, the recording stops. Check FFmpeg stderr output for encoding errors. Common causes: unsupported resolution, disk full, or codec issues.

## Quick Reference

Compact reference for all 217 API functions.

### sys (9)

| Function | Returns |
|----------|---------|
| `sys.wait_frames(n)` | — |
| `sys.wait_ms(ms)` | — |
| `sys.print(...)` | — |
| `sys.log(...)` | — |
| `sys.time_ms()` | number |
| `sys.frame_count()` | number |
| `sys.frame_time()` | number |
| `sys.fps()` | number |
| `sys.quit()` | — |

### cpu (8)

| Function | Returns |
|----------|---------|
| `cpu.load(path)` | — |
| `cpu.reset()` | — |
| `cpu.freeze()` | — |
| `cpu.resume()` | — |
| `cpu.start()` | — |
| `cpu.stop()` | — |
| `cpu.is_running()` | boolean |
| `cpu.mode()` | string |

### mem (9)

| Function | Returns |
|----------|---------|
| `mem.read8(addr)` | number |
| `mem.read16(addr)` | number |
| `mem.read32(addr)` | number |
| `mem.write8(addr, value)` | — |
| `mem.write16(addr, value)` | — |
| `mem.write32(addr, value)` | — |
| `mem.read_block(addr, len)` | string |
| `mem.write_block(addr, bytes)` | — |
| `mem.fill(addr, len, value)` | — |

### term (6)

| Function | Returns |
|----------|---------|
| `term.type(str)` | — |
| `term.type_line(str)` | — |
| `term.read()` | string |
| `term.clear()` | — |
| `term.echo(on)` | — |
| `term.wait_output(pattern, timeout_ms)` | boolean |

### audio (28)

| Function | Returns |
|----------|---------|
| `audio.start()` | — |
| `audio.stop()` | — |
| `audio.reset()` | — |
| `audio.freeze()` | — |
| `audio.resume()` | — |
| `audio.write_reg(addr, value)` | — |
| `audio.psg_load(path)` | — |
| `audio.psg_play()` | — |
| `audio.psg_stop()` | — |
| `audio.psg_is_playing()` | boolean |
| `audio.psg_metadata()` | table |
| `audio.sid_load(path [, subsong])` | — |
| `audio.sid_play()` | — |
| `audio.sid_stop()` | — |
| `audio.sid_is_playing()` | boolean |
| `audio.sid_metadata()` | table |
| `audio.ted_load(path)` | — |
| `audio.ted_play()` | — |
| `audio.ted_stop()` | — |
| `audio.ted_is_playing()` | boolean |
| `audio.pokey_load(path)` | — |
| `audio.pokey_play()` | — |
| `audio.pokey_stop()` | — |
| `audio.pokey_is_playing()` | boolean |
| `audio.ahx_load(path)` | — |
| `audio.ahx_play()` | — |
| `audio.ahx_stop()` | — |
| `audio.ahx_is_playing()` | boolean |

### video (65)

| Function | Returns |
|----------|---------|
| `video.write_reg(addr, value)` | — |
| `video.read_reg(addr)` | number |
| `video.get_dimensions()` | width, height |
| `video.is_enabled()` | boolean |
| `video.vga_enable(on)` | — |
| `video.vga_set_mode(mode)` | — |
| `video.vga_set_palette(idx, r, g, b)` | — |
| `video.vga_get_palette(idx)` | r, g, b |
| `video.vga_get_dimensions()` | width, height |
| `video.ula_enable(on)` | — |
| `video.ula_is_enabled()` | boolean |
| `video.ula_border(colour)` | — |
| `video.ula_get_dimensions()` | width, height |
| `video.antic_enable(on)` | — |
| `video.antic_is_enabled()` | boolean |
| `video.antic_dlist(addr)` | — |
| `video.antic_dma(flags)` | — |
| `video.antic_scroll(h, v)` | — |
| `video.antic_charset(page)` | — |
| `video.antic_pmbase(page)` | — |
| `video.antic_get_dimensions()` | width, height |
| `video.gtia_color(reg, value)` | — |
| `video.gtia_player_pos(player, x)` | — |
| `video.gtia_player_size(player, size)` | — |
| `video.gtia_player_gfx(player, data)` | — |
| `video.gtia_priority(value)` | — |
| `video.ted_enable(on)` | — |
| `video.ted_is_enabled()` | boolean |
| `video.ted_mode(ctrl1, ctrl2)` | — |
| `video.ted_colors(bg0, bg1, bg2, bg3, border)` | — |
| `video.ted_charset(page)` | — |
| `video.ted_video_base(page)` | — |
| `video.ted_cursor(pos, colour)` | — |
| `video.ted_get_dimensions()` | width, height |
| `video.voodoo_enable(on)` | — |
| `video.voodoo_is_enabled()` | boolean |
| `video.voodoo_resolution(w, h)` | — |
| `video.voodoo_vertex(ax, ay, bx, by, cx, cy)` | — |
| `video.voodoo_color(idx, r, g, b, a)` | — |
| `video.voodoo_depth(z)` | — |
| `video.voodoo_texcoord(s, t, w)` | — |
| `video.voodoo_draw()` | — |
| `video.voodoo_swap()` | — |
| `video.voodoo_clear(r, g, b)` | — |
| `video.voodoo_fog(on, r, g, b)` | — |
| `video.voodoo_alpha(mode)` | — |
| `video.voodoo_zbuffer(mode)` | — |
| `video.voodoo_clip(left, right, top, bottom)` | — |
| `video.voodoo_texture(w, h, data)` | — |
| `video.voodoo_chromakey(on, r, g, b)` | — |
| `video.voodoo_dither(on)` | — |
| `video.voodoo_get_dimensions()` | width, height |
| `video.copper_enable(on)` | — |
| `video.copper_set_program(addr)` | — |
| `video.copper_is_running()` | boolean |
| `video.blit_copy(src, dst, w, h, src_stride, dst_stride)` | — |
| `video.blit_fill(dst, w, h, colour, dst_stride)` | — |
| `video.blit_line(x0, y0, x1, y1, colour)` | — |
| `video.blit_wait()` | — |
| `video.get_pixel(x, y)` | r, g, b, a |
| `video.get_region(x, y, w, h)` | string |
| `video.frame_hash()` | number |
| `video.wait_pixel(x, y, r, g, b, timeout_ms)` | boolean |
| `video.wait_stable(n_frames, timeout_ms)` | boolean |
| `video.wait_condition(fn, timeout_ms)` | boolean |

### repl (8)

| Function | Returns |
|----------|---------|
| `repl.show()` | — |
| `repl.hide()` | — |
| `repl.is_open()` | boolean |
| `repl.print(text)` | — |
| `repl.clear()` | — |
| `repl.scroll_up(n)` | — |
| `repl.scroll_down(n)` | — |
| `repl.line_count()` | number |

### rec (5)

| Function | Returns |
|----------|---------|
| `rec.screenshot(path)` | — |
| `rec.start(path)` | — |
| `rec.stop()` | — |
| `rec.is_recording()` | boolean |
| `rec.frame_count()` | number |

### dbg (56)

| Function | Returns |
|----------|---------|
| `dbg.open()` | — |
| `dbg.close()` | — |
| `dbg.is_open()` | boolean |
| `dbg.freeze()` | — |
| `dbg.resume()` | — |
| `dbg.step([n])` | — |
| `dbg.continue()` | — |
| `dbg.run_until(addr)` | — |
| `dbg.backstep()` | — |
| `dbg.set_bp(addr)` | — |
| `dbg.set_conditional_bp(addr, condition)` | — |
| `dbg.clear_bp(addr)` | — |
| `dbg.clear_all_bp()` | — |
| `dbg.list_bp()` | table |
| `dbg.set_wp(addr)` | — |
| `dbg.clear_wp(addr)` | — |
| `dbg.clear_all_wp()` | — |
| `dbg.list_wp()` | table |
| `dbg.get_reg(name)` | number/nil |
| `dbg.set_reg(name, value)` | — |
| `dbg.get_regs()` | table |
| `dbg.get_pc()` | number |
| `dbg.set_pc(addr)` | — |
| `dbg.read_mem(addr, len)` | string |
| `dbg.write_mem(addr, data)` | — |
| `dbg.fill_mem(addr, len, value)` | — |
| `dbg.hunt_mem(start, len, pattern)` | table |
| `dbg.compare_mem(start, len, dest)` | table |
| `dbg.transfer_mem(start, len, dest)` | — |
| `dbg.backtrace([depth])` | table |
| `dbg.disasm(addr, count)` | table |
| `dbg.trace(n)` | table |
| `dbg.trace_file(path)` | — |
| `dbg.trace_file_off()` | — |
| `dbg.trace_watch_add(addr)` | — |
| `dbg.trace_watch_del(addr)` | — |
| `dbg.trace_watch_list()` | table |
| `dbg.trace_history(addr_str)` | table |
| `dbg.trace_history_clear(addr)` | — |
| `dbg.save_state(path)` | — |
| `dbg.load_state(path)` | — |
| `dbg.save_mem_file(start, length, path)` | — |
| `dbg.load_mem_file(path, addr)` | — |
| `dbg.cpu_list()` | table |
| `dbg.cpu_focus(id)` | — |
| `dbg.freeze_cpu(label)` | — |
| `dbg.thaw_cpu(label)` | — |
| `dbg.freeze_all()` | — |
| `dbg.thaw_all()` | — |
| `dbg.freeze_audio()` | — |
| `dbg.thaw_audio()` | — |
| `dbg.io_devices()` | table |
| `dbg.io(device)` | table |
| `dbg.run_script(path)` | — |
| `dbg.macro(name, cmds)` | — |
| `dbg.command(cmd)` | — |

### coproc (7)

| Function | Returns |
|----------|---------|
| `coproc.start(cpu_type, filename)` | — |
| `coproc.stop(cpu_type)` | — |
| `coproc.enqueue(cpu_type, op, request)` | number (ticket) |
| `coproc.poll(ticket)` | string |
| `coproc.wait(ticket, timeout_ms)` | string, string |
| `coproc.workers()` | table |
| `coproc.response(ticket)` | string |

### media (7)

| Function | Returns |
|----------|---------|
| `media.load(filename)` | — |
| `media.load_subsong(filename, subsong)` | — |
| `media.play()` | — |
| `media.stop()` | — |
| `media.status()` | string |
| `media.type()` | string |
| `media.error()` | number |

### bit32 (9)

| Function | Returns |
|----------|---------|
| `bit32.band(...)` | number |
| `bit32.bor(...)` | number |
| `bit32.bxor(...)` | number |
| `bit32.bnot(x)` | number |
| `bit32.lshift(x, disp)` | number |
| `bit32.rshift(x, disp)` | number |
| `bit32.arshift(x, disp)` | number |
| `bit32.lrotate(x, disp)` | number |
| `bit32.rrotate(x, disp)` | number |
