
Copyright (c) 2026 Zayn Otley. All rights reserved.

# Chapter 50 - One Effect, Six CPUs

The rotozoomer is a good comparison effect because the hardware job
does not change. The VideoChip still owns Mode 7. The blitter still
receives the same registers. The front buffer, texture, back buffer, and
VBlank rhythm stay on the same bus.

What changes is the CPU idiom.

## 50.1 The Common Contract

Every version must do these jobs:

1. Enable VideoChip.
2. Place or build a texture.
3. Start its chosen audio engine.
4. Advance angle and scale state.
5. Convert table values into six Mode 7 parameters.
6. Start the blitter.
7. Wait for completion and present the frame.

That is the contract. A CPU chapter teaches how to execute instructions.
This chapter teaches what remains the same after the instruction set
changes.

## 50.2 CPU Comparison

| CPU | Rotozoomer idiom | Main lesson |
|-----|------------------|-------------|
| IE64 | Wide native registers and fixed instruction width. | Keep many addresses and fixed-point values live at once. |
| IE32 | Compact RISC-style code with explicit fixed-point helpers. | A small native CPU can still drive the same MMIO contract. |
| 6502 | Banked access, byte arithmetic, helper routines. | Use the hardware blitter because software pixels are not practical. |
| Z80 | Register pairs, table copies, banked ranges. | Keep the bus contract visible through the adapter. |
| M68K | Orthogonal addressing and 68020-class integer work. | Use clean longword MMIO writes and table arithmetic. |
| x86 | Flat `32`-bit addressing and signed multiply. | Use familiar integer instructions against IE's bus. |

The table is not a ranking. It is a reading guide.

## 50.3 The Same Blitter Writes

Each CPU eventually performs this sequence:

```text
BLT_OP          = 5
BLT_SRC         = texture address
BLT_DST         = back buffer address
BLT_WIDTH       = 640
BLT_HEIGHT      = 480
BLT_SRC_STRIDE  = 1024
BLT_DST_STRIDE  = 2560
BLT_MODE7_U0    = computed U origin
BLT_MODE7_V0    = computed V origin
BLT_MODE7_DU... = computed deltas
BLT_CTRL        = 1
```

The 6502 may reach those registers through an adapter. The Z80 may use
its own mapped view. M68K and x86 may use absolute long addresses. The
device is still the same VideoChip.

## 50.4 Different Audio Choices

The shipped rotozoomers deliberately vary the sound engine. The point is
the same as the video comparison: the audio engines are cards on the
same machine.

| CPU version | Example audio path |
|-------------|--------------------|
| IE64 | POKEY/SAP-style playback path. |
| IE32 | AHX playback path. |
| 6502 | SID-style path. |
| Z80 | SID-style path through the shared audio block. |
| M68K | TED playback path. |
| x86 | PSG playback path. |

The CPU does not have to mix samples itself. It starts an audio engine
and returns to video work.

## 50.5 Reading A Port

When reading a port to another CPU, use this order:

1. Find the constants: framebuffer, texture, back buffer, stride.
2. Find the initialisation: video mode, framebuffer base, audio start.
3. Find the table lookups: angle, scale, sine, reciprocal.
4. Find the six Mode 7 parameters.
5. Find the presentation step.
6. Find the accumulator advance.

Only after that should you study the CPU-specific tricks.

## 50.6 What The Comparison Proves

The six versions are not six separate machines. They are six views of
one shared hardware contract. If you understand the BASIC version and
the Mode 7 register block, you can read every port by asking one
question:

```text
How does this CPU compute and write the same six values?
```

Chapter 51 returns to BASIC and makes the source texture move before
Mode 7 sees it.
