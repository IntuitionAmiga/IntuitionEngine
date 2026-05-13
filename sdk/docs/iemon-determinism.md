# IEMon Determinism Audit

Whole-machine reverse debugging depends on restoring every source of guest-visible state and avoiding host-time drift during replay. IEMon has a whole-machine snapshot envelope for all registered CPUs, shared bus RAM, IE64 backing memory, and registered versioned device blobs. The production monitor registers the main video chip, sound chip, terminal MMIO, command-style host helpers, compatibility audio/video engines, and the AROS clipboard/audio-DMA host bridges when those bridges are present.

## Captured Today

| Source | Snapshot coverage |
|--------|-------------------|
| Registered CPUs | Monitor id, label, CPU type, registers, and sparse CPU memory pages |
| Shared bus RAM | Sparse non-zero pages |
| IE64 sparse backing | Advertised size plus allocated sparse pages |
| Snapshot chain | Full sparse checkpoints plus retained page deltas; restore materialises deltas before applying state |
| Registered devices | Versioned blobs keyed by stable device name; restore fails if a captured device is absent |
| Video chip | Mode registers, frame buffers, CLUT/palette state, copper state, blitter state, raster registers, and status latches |
| Sound chip | Main mixer/filter state, FLEX-register shadow, voice phase/envelope state, SN76489 voice state, delay/reverb buffers, and master compressor/normaliser state |
| Compatibility audio engines | PSG/AY, SN76489, SID/SID2/SID3, TED audio, and POKEY register shadows, playback cursors, loop state, enabled flags, chip-sync state, oscillator/noise state |
| Compatibility video engines | VGA, ULA, TED video, ANTIC/GTIA, and Voodoo register shadows, VRAM/texture memory, palette/colour state, scanline/compositor state, frame buffers, and status latches |
| Terminal MMIO | Input/output rings, raw-key/scancode queues, echo and line-mode flags, mouse state, modifiers, sentinel state |
| Command-style host helpers | File I/O, media loader, program executor, and coprocessor-manager MMIO shadow registers, result/status latches, and completion-ticket state |
| AROS audio DMA | Channel descriptors, latched/next buffers, DMA position and phase, status/interrupt latches |
| AROS clipboard bridge | Guest-visible request registers, status, result length, and format |
| Access timeline | Bounded event ring with sequence, CPU id, PC, address, width, kind, and write values |

## Device Contract

Mutable devices expose a versioned `DebugSnapshot()/DebugRestoreSnapshot()` pair whose blob contains all guest-visible state needed to resume deterministically. The top-level `DeviceStateBlob` envelope stores `Name`, `Version`, and opaque data. Devices are registered with `MachineMonitor.RegisterSnapshotDevice`; a missing device during restore is an error, not a silent partial restore.

Required device state includes:

| Area | State to capture |
|------|------------------|
| Video | Mode registers, palette, VRAM/direct-frame metadata, copper/blitter state, frame counters, latched error/status flags |
| Audio | Voice registers, envelopes, oscillator phase, filter state, DMA cursors, IRQ latches, output mute/freeze state |
| Timers/IRQs | Pending interrupts, masks, countdowns, scheduler-visible timer counters |
| DMA/MMIO helpers | In-flight descriptors, base/current pointers, status and acknowledgement latches |
| Host bridges | Guest-visible file/load intercept state, not host file descriptors themselves |

## Nondeterminism Sources

| Source | Policy |
|--------|--------|
| Host wall clock | Do not read during deterministic replay, or snapshot the guest-visible derived counter |
| RNG | Snapshot seed and stream position before guest-visible use |
| Audio callback timing | Snapshot generated sample phase and DMA cursors; replay should not depend on host callback cadence |
| Video frame pacing | Snapshot frame counters and vblank state; replay should advance by guest events, not wall time |
| Concurrent CPUs | Snapshot at monitor quiescence points; reverse replay should stop CPUs before restore |
| Unattributed bus activity | Record for timeline only unless attributed to a real CPU before guard/watch evaluation |

## Current Limits

`rg` and `rt` restore a retained predecessor snapshot and re-execute to the selected boundary when the predecessor is available; otherwise they restore the selected retained state directly. Monitor stop events capture reverse boundaries for breakpoints, watchpoints, guards, break-ins, and faults. Deterministic replay still depends on pinning the host-time sources above and on all guest-visible devices obeying the snapshot contract. Code that adds a new guest-visible timer, DMA engine, interrupt controller, or host bridge must register a device blob before it can be considered covered by whole-machine reverse debugging.

IEScript `dbg.history_config()` changes the same reverse-history retention values as `history config`. Deterministic scripts should pin `delta_interval`, `delta_mib`, `checkpoints`, and `snapshots` at startup so replay horizons do not vary with user rc files or interactive monitor changes.
