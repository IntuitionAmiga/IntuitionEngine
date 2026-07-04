# IE SFX Trigger MMIO

The IE SFX trigger block is a native sample playback path for short effects.
It is independent from the SoundChip FLEX channels used by MOD/WAV playback:
triggered SFX keep their own per-channel state and are mixed additively into
the final mono SoundChip sample.

Legacy base range: `0xF0E80-0xF0EFF` (channels 0-3)

Extended base range: `0xF2600-0xF29FF` (channels 0-31)

| Offset | Name | Width | Description |
|--------|------|-------|-------------|
| `+0x00` | `SFX_PTR` | u32 | Sample base pointer in guest bus memory |
| `+0x04` | `SFX_LEN` | u32 | Sample length in bytes |
| `+0x08` | `SFX_LOOP_PTR` | u32 | Loop start pointer, or `0` for no loop |
| `+0x0C` | `SFX_LOOP_LEN` | u32 | Loop length in bytes |
| `+0x10` | `SFX_FREQ` | u32 | Source sample rate in Hz |
| `+0x14` | `SFX_VOL` | u16 | Volume, `0..255` |
| `+0x16` | `SFX_PAN_RESERVED` | u16 | Reserved; accepted but ignored while output is mono |
| `+0x18` | `SFX_FORMAT` | u8 | `0` signed 8-bit, `1` unsigned 8-bit, `2` signed 16-bit little-endian |
| `+0x1C` | `SFX_CTRL` | u32 | Write control bits, read status bits |

There are 32 channels with a `0x20` byte stride in the extended window. The legacy window aliases channels 0-3 for existing guests.

SDK includes keep `IE_SFX_CH_BASE` on the legacy window for compatibility and expose `IE_SFX_EXT_CH_BASE` for the 32-channel window. On flat-address cores, `IE_SFX_EXT_CH_BASE` is `0xF2600`. On 6502/Z80, set `BANK1` to `IE_SFX_EXT_BANK` (`0x79`) and use `IE_SFX_EXT_CH_BASE` (`0x2600`) inside that banked window; channel `n` is still `IE_SFX_EXT_CH_BASE + n * IE_SFX_CH_STRIDE`.

| Channel | Extended Base | Legacy Alias |
|---------|---------------|--------------|
| 0 | `0xF2600` | `0xF0E80` |
| 1 | `0xF2620` | `0xF0EA0` |
| 2 | `0xF2640` | `0xF0EC0` |
| 3 | `0xF2660` | `0xF0EE0` |
| 4..31 | `0xF2600 + channel * 0x20` | none |

`SFX_CTRL` write bits:

| Bit | Name | Meaning |
|-----|------|---------|
| 0 | `SFX_CTRL_TRIGGER` | Start playback from `SFX_PTR` |
| 1 | `SFX_CTRL_STOP` | Stop playback immediately |
| 2 | `SFX_CTRL_LOOP_EN` | Enable looping via `SFX_LOOP_PTR/LEN` |

`SFX_CTRL` read status bits:

| Bit | Name | Meaning |
|-----|------|---------|
| 0 | `SFX_STATUS_PLAYING` | Channel is currently active |
| 1 | `SFX_STATUS_ERROR` | Trigger rejected because the sample range was invalid |

Guest code may use byte, word, or long writes. Multi-byte fields are assembled
little-endian in the MMIO shadow, matching the rest of the IE audio register
surface. `SFX_VOL` and `SFX_PAN_RESERVED` form one 32-bit register pair: a long
write at `+0x14` applies volume from the low half and pan from the high half.
