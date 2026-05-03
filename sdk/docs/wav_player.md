# WAV Player ABI

The WAV player streams guest RAM `.wav` data through SoundChip FLEX DAC channels.
It accepts PCM 8-bit unsigned and 16-bit signed WAV files, plus
`WAVE_FORMAT_EXTENSIBLE` when the subformat is PCM and the valid/container bit
depth is 16-bit. ADPCM, float, A-law, mu-law, 24-bit, and 32-bit sources are
rejected.

## Registers

| Address | Name | Width | Description |
|---|---:|---:|---|
| `0xF0BD8` | `WAV_PLAY_PTR` | u32 | Low 32 bits of guest pointer |
| `0xF0BDC` | `WAV_PLAY_LEN` | u32 | WAV byte length |
| `0xF0BE0` | `WAV_PLAY_CTRL` | u32 | bit0 start, bit1 stop, bit2 loop, bit3 pause, bit4 apply loop only |
| `0xF0BE4` | `WAV_PLAY_STATUS` | u32 | bit0 busy, bit1 error, bit2 paused, bit3 stereo active |
| `0xF0BE8` | `WAV_POSITION` | u32 | Source sample-frame index |
| `0xF0BEC` | `WAV_PLAY_PTR_HI` | u32 | High 32 bits of guest pointer |
| `0xF0BF0` | `WAV_CHANNEL_BASE` | u8 | Left DAC channel; right uses base+1 |
| `0xF0BF1` | `WAV_VOLUME_L` | u8 | Left volume, 0-255 |
| `0xF0BF2` | `WAV_VOLUME_R` | u8 | Right volume, 0-255 |
| `0xF0BF3` | `WAV_FLAGS` | u8 | bit0 force mono |

`WAV_END` is an internal inclusive MMIO mapping bound and is not a guest ABI
register.

Reset defaults preserve legacy mono playback: `WAV_PLAY_PTR_HI=0`,
`WAV_CHANNEL_BASE=0`, `WAV_VOLUME_L=255`, `WAV_VOLUME_R=255`, and
`WAV_FLAGS.bit0=1`.

## Notes

Stereo playback requires clearing `WAV_FLAGS.bit0`; otherwise the player
averages left/right and writes the same mono sample to both DAC channels. Panning
is controlled by setting different left and right volume values.

High-RAM playback is available on buses with 64-bit backing by writing
`WAV_PLAY_PTR_HI` before start. Strict media reads reject out-of-range and
low/high seam-crossing spans instead of parsing zero-filled bytes.
