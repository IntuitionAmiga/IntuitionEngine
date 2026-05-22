
# Appendix K - Block Diagrams

Schematic-level layout of the three internal buses that connect
the chips inside the Intuition Engine: the video compositor, the
audio mixer, and the main system bus. Each diagram is drawn at
the granularity a programmer needs to reason about cross-chip
interactions, not at the granularity an engineer would use to
fabricate the silicon.

## K.1 The video compositor

The compositor has six layers. Each layer comes from one of the
six video engines. Layers are listed in back-to-front order; a
higher layer covers the lower ones where it draws non-transparent
pixels.

```
                 +-----------------------------+
                 |  HDMI / display output      |
                 +--------------^--------------+
                                |
                 +--------------+--------------+
                 |   Video compositor          |
                 |   (per-pixel layer mux,     |
                 |    palette, blend modes)    |
                 +-+----+----+----+----+----+--+
                   |    |    |    |    |    |
        layer 20   |    |    |    |    |    |   (front)
                   |    |    |    |    |    +-- Voodoo 3D
                   |    |    |    |    +------- TED video
                   |    |    |    +------------ ANTIC + GTIA
                   |    |    +----------------- ULA
                   |    +---------------------- VGA
                   +--------------------------- VideoChip (layer 0, back)
```

Each engine writes into its own framebuffer (in main VRAM or in
its private aperture); the compositor reads all six on every
output line and resolves them into a single stream of RGBA
pixels.

## K.2 The audio mixer

The audio mixer has ten engine inputs (plus the SFX channels) and
one stereo output stream.

```
   SoundChip --+
   PSG / AY --+
   SN76489 ---+
   SID  ------+
   SID2 ------+
   SID3 ------+
   POKEY -----+--->  Mixer  --->  Filter ---> Reverb ---> Overdrive ---> Output
   TED audio -+      (sum,         (per-       (global)    (global)
   AHX -------+       per-chip      voice)
   MOD -------+       gain,
   WAV -------+       per-chip
   Paula DMA -+       mute)
              |
   SFX ch 0-3 +
```

The SoundChip's own filter, the SID family's resonant filter, and
the engine-internal effects all feed the mix before the global
filter / reverb / overdrive stage; the global effects apply once
per output sample to the summed signal.

## K.3 The system bus

Every CPU and every device hangs off one shared 32-bit bus. The
bus arbitrates simultaneous accesses on a round-robin basis;
there is no priority encoding above the CPU / DMA distinction.

```
                  +--------------------+
                  |    Main RAM        |
                  +---------^----------+
                            |
                            |
   +------+   +------+   +--+--+   +------+   +------+   +------+
   | IE64 |   | IE32 |   | M68K|   | 6502 |   | Z80  |   | x86  |
   +--+---+   +--+---+   +--+--+   +--+---+   +--+---+   +--+---+
      |          |          |         |          |          |
      +----------+----------+--+------+----------+----------+
                              |
                  +-----------+-----------+
                  |   System bus           |
                  |   (32-bit data,        |
                  |    24-bit address,     |
                  |    arbitrated)         |
                  +-+---+---+---+---+---+--+
                    |   |   |   |   |   |
              MMIO  |   |   |   |   |   |  DMA engines:
              regs  |   |   |   |   |   |   - blitter
                    |   |   |   |   |   |   - Amiga Paula DMA
              VGA --+   |   |   |   |   +-- - SFX sample DMA
              Video chip|   |   |   |       - copper
              ULA ------+   |   |   |
              Audio --------+   |   |
              File I/O ---------+   |
              Coprocessor ---------+
```

The 8-bit CPUs (6502, Z80) reach the bus through an address
translator that turns their 16-bit address space into the bus's
24-bit form, with the bank registers described in Chapters 26
and 27 selecting which translation applies.

## K.4 Coprocessor channels

The coprocessor block (Chapter 31) is a many-to-many channel
between the main CPU and a pool of worker CPUs. Each worker
listens on its own ring buffer; the main CPU posts work items
into the ring and reads the result back.

```
   Main CPU
   |
   |  COSTART / COCALL  -->  +----------------+
   |                         |  Ring buffer   |
   |  COSTATUS / COWAIT  <-- |  (per-worker)  |
   |                         +-------+--------+
   |                                 |
   |                         +-------+--------+
   |                         |  Worker CPU    |
   |                         |  (one of 6     |
   |                         |   types)       |
   |                         +----------------+
```

Six worker types share the protocol: an IE64 worker, an IE32
worker, a 6502 worker, a Z80 worker, an M68K worker, and an x86
worker. The main CPU is whichever one boots the machine; all the
others are available as workers when not booted.

## K.5 Bus translation for the small CPUs

The 6502 and Z80 see the same overall bus through a 16-bit-to-
24-bit translator. The translator has two independent functions:

```
   6502 / Z80 address (16-bit)
        |
        v
   +----+------------------------+
   | Decode page                 |
   |  - bank registers $F7xx? --> intercepted, never reach bus
   |  - MMIO mirror $F0xx-$FFF9? -> +0xF0000 -> bus
   |  - $E000-$EFFF banked window-> + bank * 4 KB -> bus
   |  - everything else --------- > main RAM page
   +----+------------------------+
        |
        v
   bus address (24-bit)
```

The bank registers at `$F700`-`$F705` (low/hi pairs for
apertures 1, 2, 3) and `$F7F0` (aperture 0) are captured before the
translator runs. A write to `$F700` does not reach
`0xF0700`; it loads the low byte of bank register 1.
