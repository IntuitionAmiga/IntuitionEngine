# IEDoom Plan Using Engine-Level MIDI/MUS

## Summary

First land the required engine/SDK ABI changes in this IntuitionEngine clone.
After that ABI is real and tested, create or use `../IEDoom` as a Chocolate
Doom-based x86 guest port for Intuition Engine. Use the engine-level SMF/MUS
support from commit `0ff06b22c9bc36f21e8fce2cc03a78c6c9aeeaba` for Doom music.
Do not add an IEDoom-specific MIDI synth or curated music override layer.

IEDoom owns Doom integration, WAD access, and menu/config choices. Intuition
Engine owns MIDI/MUS parsing, synthesis, playback MMIO, and SoundChip output.

## Implementation Status

- Intuition Engine Phase 0 ABI prerequisites are implemented in this repo:
  `RTC_MONO_USEC_LO`, `RTC_MONO_USEC_HI`, and `MIDI_STATUS_LOADING`.
- SDK include constants are updated across all six architecture include files:
  `ie32.inc`, `ie64.inc`, `ie65.inc`, `ie68.inc`, `ie80.inc`, and `ie86.inc`.
- ABI drift tests cover the new terminal/RTC and MIDI symbols. Runtime tests
  cover monotonic high-low-high reads, MIDI loading status clear behavior, and
  the `.ie86` load-at-0/start-at-0 contract.
- Refman docs were intentionally not edited per the repository task constraint.
- The Chocolate Doom guest port work remains external to this repo and should
  be implemented in `../IEDoom` or an equivalent IEDoom workspace.

## Key Changes

### Phase 0: Engine ABI Prerequisites

- Add monotonic microsecond time registers inside the existing terminal/input
  MMIO region handled by `TerminalMMIO`:
  - `RTC_MONO_USEC_LO = $F075C`
  - `RTC_MONO_USEC_HI = $F0760`
- Define the non-tearing read protocol for the split 64-bit counter as
  high-low-high retry: read `RTC_MONO_USEC_HI`, read `RTC_MONO_USEC_LO`, read
  `RTC_MONO_USEC_HI` again, and retry if the two high reads differ.
- Source `RTC_MONO_USEC_*` from host monotonic time, not wall-clock time. The
  epoch is engine process start, so the value is microseconds elapsed since the
  Intuition Engine process began.
- Add `MIDI_PLAY_STATUS` bit `3` as `MIDI_STATUS_LOADING`, set only while
  asynchronous parse/load is in progress.
- Update SDK includes, refman docs, and ABI drift tests for the new time
  registers and MIDI loading bit.
- Verify current x86 `.ie86` behavior remains load-at-0/start-at-0 and document
  that the guest image must include a reset trampoline at address `0`.

### Chocolate Doom Port

- Build IEDoom from Chocolate Doom as a flat x86 `.ie86` guest target whose
  first byte is loaded at address `0`.
- Put a reset trampoline at address `0` because current IE x86 execution loads
  and starts `.ie86` programs at `0`.
- Link the main Doom image at `PROGRAM_START = 0x1000`; the trampoline sets
  `ESP = STACK_TOP`, initializes the C runtime, and jumps to the IEDoom entry.
- Add a freestanding C runtime shim for the IE guest environment.
- Add IE MMIO shims for files, input, timing, video, sound effects, and music.
- Implement Doom's `I_GetTime` and tic loop from `RTC_MONO_USEC_*` using the
  documented high-low-high retry read protocol.

### Video Path

- Use native IE VideoChip CLUT8 as the v1 display path.
- Enable the VideoChip with `VIDEO_CTRL = 1`.
- Configure `VIDEO_MODE = MODE_320x200`.
- Configure `VIDEO_COLOR_MODE = 1` for CLUT8.
- Point `VIDEO_FB_BASE` at Doom's stable 64,000-byte screen buffer in guest
  memory.
- Write Doom palette updates to the VideoChip 256-entry palette table.
- Pack each Doom palette byte triple `(r, g, b)` as a 32-bit VideoChip palette
  entry `0x00RRGGBB`, using `(r << 16) | (g << 8) | b`.
- Do not expand indexed pixels to RGB inside the guest.
- Treat this as a thin framebuffer adapter around Doom's existing software
  renderer, not a new renderer.

### Music Backend

- Use original WAD MUS lump bytes directly through the IE MIDI/MUS MMIO player
  after Phase 0 adds the required loading-status bit.
- Use these MIDI registers:
  - `MIDI_PLAY_PTR = $F0BA0`
  - `MIDI_PLAY_LEN = $F0BA4`
  - `MIDI_PLAY_CTRL = $F0BA8`
  - `MIDI_PLAY_STATUS = $F0BAC`
  - `MIDI_VOLUME = $F0BB4`
- Start playback with `MIDI_PLAY_CTRL = 1`.
- Start looping playback with `MIDI_PLAY_CTRL = 5`.
- Stop playback with `MIDI_PLAY_CTRL = 2`.
- Pause playback with `MIDI_PLAY_CTRL = 8`.
- Resume playback by writing `MIDI_PLAY_CTRL = 0`, which clears the pause bit
  without restarting from the staged pointer.
- MIDI load is asynchronous. After start, poll until `MIDI_STATUS_LOADING`
  clears, then treat status bit `1` as parse/load failure and fall back to
  silence.

### Sound Effects Backend

- Use IE's SFX trigger block for Doom sound effects, not Paula DMA.
- Use Chocolate Doom's existing DMX sound lump loader/parser where possible.
- DMX sound lumps provide a sample-rate field, a PCM payload length, and PCM
  sample bytes after the lump header; the SFX backend consumes those parsed
  values rather than treating the whole lump as sample data.
- Set `SFX_PTR` to the first PCM sample byte, not the start of the DMX lump.
- Set `SFX_LEN` to the PCM payload length, not the full lump length.
- Set `SFX_FREQ` directly from the Doom sound lump sample-rate field, in Hz.
- Scale Doom effect volume into IE's 0..65535 SFX range as
  `SFX_VOL = doomVolume * 65535 / maxDoomVolume`.
- Allocate one of `IE_SFX_CHANNELS` channels and address it as
  `IE_SFX_CH_BASE + channel * IE_SFX_CH_STRIDE`.
- Trigger playback through the SFX channel register offsets:
  - `SFX_PTR`
  - `SFX_LEN`
  - `SFX_FREQ`
  - `SFX_VOL`
  - `SFX_FORMAT`
  - `SFX_CTRL`
- The effective MMIO address for each field is
  `IE_SFX_CH_BASE + channel * IE_SFX_CH_STRIDE + offset`.
- Use `SFX_FORMAT_UNSIGNED8 = 1` for Doom's native DMX sound lumps.
- Use `SFX_FORMAT_SIGNED8 = 0` only if the port explicitly converts sample data
  to signed 8-bit PCM.
- Trigger playback with `SFX_CTRL_TRIGGER = 1`.
- Use IE SFX channels for overlapping one-shot effects.

### Menu And Config

- Add music mode options:
  - `Original MUS`
  - `None`
- Default music mode: `Original MUS`.

## Test Plan

### Phase 0 Engine Tests

- `RTC_MONO_USEC_*` reads are monotonic across repeated high-low-high reads.
- SDK include constants match Go constants for `RTC_MONO_USEC_LO`,
  `RTC_MONO_USEC_HI`, and `MIDI_STATUS_LOADING`.
- `MIDI_PLAY_STATUS` sets bit `3` while asynchronous parse/load is in progress,
  clears it after success, and clears it after parse failure.
- x86 `.ie86` execution still loads the first byte at address `0` and starts
  execution at address `0`.

### IEDoom Music Tests

- WAD lump `d_e1m1` is passed to MIDI MMIO as MUS data.
- Looped music writes `MIDI_PLAY_CTRL = 5`.
- Stop writes `MIDI_PLAY_CTRL = 2`.
- Pause writes `MIDI_PLAY_CTRL = 8`.
- Resume writes `MIDI_PLAY_CTRL = 0` and does not restart the song.
- `MIDI_VOLUME` follows Doom music volume settings.
- MIDI start waits for `MIDI_STATUS_LOADING` to clear before
  checking error status.
- MIDI status bit `1` after load completion falls back to silence without
  crashing.
- `None` mode suppresses all MIDI playback writes.

### IEDoom Sound Effect Tests

- Firing a Doom sound effect writes the expected `SFX_PTR`.
- Firing a Doom sound effect writes the expected `SFX_LEN`.
- `SFX_PTR` points to the first decoded PCM payload byte, not the DMX lump
  header.
- `SFX_LEN` is the decoded PCM payload length, not the full lump length.
- `SFX_FREQ` is copied from the Doom sound lump sample-rate field in Hz.
- `SFX_VOL` is scaled as `doomVolume * 65535 / maxDoomVolume`.
- Maximum Doom effect volume maps to `SFX_VOL = 65535`.
- Firing a Doom sound effect writes `SFX_FORMAT_UNSIGNED8 = 1` to the channel
  `SFX_FORMAT` offset for native DMX sound lumps.
- Firing a Doom sound effect writes `SFX_CTRL_TRIGGER = 1` to the channel
  `SFX_CTRL` offset.
- Tests compute SFX MMIO addresses as
  `IE_SFX_CH_BASE + channel * IE_SFX_CH_STRIDE + offset`.
- Overlapping one-shot sound effects use separate IE SFX channels when
  available.

### IEDoom Video Tests

- Startup writes `VIDEO_CTRL = 1`.
- Startup configures `VIDEO_MODE = MODE_320x200`.
- Startup configures `VIDEO_COLOR_MODE = 1`.
- Startup writes `VIDEO_FB_BASE` to Doom's stable screen buffer address.
- Doom palette updates are propagated to the VideoChip palette table.
- One known Doom palette entry is packed as `0x00RRGGBB`, using
  `(r << 16) | (g << 8) | b`.
- The `320x200x8` Doom framebuffer remains byte-identical before presentation.

### Engine Dependency Checks

- Require an Intuition Engine build that includes the Phase 0 ABI plus the
  MIDI/MUS parser support from commit
  `0ff06b22c9bc36f21e8fce2cc03a78c6c9aeeaba`, or equivalent support.
- Assert SDK constants for:
  - `PROGRAM_START = 0x1000`
  - `STACK_TOP`
  - `RTC_MONO_USEC_LO = $F075C`
  - `RTC_MONO_USEC_HI = $F0760`
  - `TERM_KEY_IN`
  - `TERM_KEY_STATUS`
  - `SCAN_CODE`
  - `SCAN_STATUS`
  - `SCAN_MODIFIERS`
  - `MOUSE_CTRL`
  - `MOUSE_DX`
  - `MOUSE_DY`
  - `FILE_NAME_PTR`
  - `FILE_DATA_PTR`
  - `FILE_DATA_LEN`
  - `FILE_CTRL`
  - `FILE_STATUS`
  - `FILE_RESULT_LEN`
  - `VIDEO_CTRL`
  - `VIDEO_MODE`
  - `MODE_320x200`
  - `VIDEO_COLOR_MODE`
  - `VIDEO_FB_BASE`
  - `VIDEO_PAL_TABLE`
  - `MIDI_PLAY_PTR`
  - `MIDI_PLAY_LEN`
  - `MIDI_PLAY_CTRL`
  - `MIDI_PLAY_STATUS`
  - `MIDI_STATUS_LOADING = 0x08`
  - `MIDI_VOLUME`
  - `MEDIA_TYPE_MIDI = 8`
  - `IE_SFX_CH_BASE`
  - `IE_SFX_CH_STRIDE = 0x20`
  - `IE_SFX_CHANNELS = 4`
  - `SFX_PTR`
  - `SFX_LEN`
  - `SFX_FREQ`
  - `SFX_VOL`
  - `SFX_FORMAT`
  - `SFX_CTRL`
  - `SFX_FORMAT_UNSIGNED8 = 1`
  - `SFX_FORMAT_SIGNED8 = 0`
  - `SFX_CTRL_TRIGGER = 1`

## Assumptions

- No curated SID/music pack support is added to IEDoom.
- No MIDI synth code is added to IEDoom.
- IEDoom uses raw MIDI MMIO for WAD-resident MUS lumps because the music bytes
  are already in guest memory.
- External music file playback is out of scope for v1.
- RawlandMini's IE-native SoundChip rendering is the default Doom music path.
- The current x86 loader contract remains load-at-0/start-at-0, so IEDoom owns
  the reset trampoline rather than requiring an engine launcher change.
- The monotonic microsecond MMIO ABI is required because `RTC_EPOCH` has only
  second precision and legacy `TIMER_*` MMIO is deprecated.
