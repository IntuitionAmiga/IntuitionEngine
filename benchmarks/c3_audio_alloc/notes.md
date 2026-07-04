# C3 Audio Allocation Hoist Notes

Date: 2026-07-04

## Current Measurement

Correctness and allocation guard:

```bash
go test -tags headless -run 'TestReadSamples|TestReadSample_StillWorks_Compat' -count=1 .
```

The steady-state allocation guard is `TestReadSamples_ZeroAllocsSteadyState`.
Full before/after benchstat evidence remains pending for:

```bash
go test -tags headless -run '^$' -bench 'Benchmark(SoundChip_ReadSamples|ReadSamples_64Segment)' -benchtime=1s -count=10 .
```

## Implementation Notes

- `SoundChip` now owns reusable `audioBlockState` and mixer-capture scratch
  storage.
- The scratch slice grows only when a caller supplies a larger destination than
  previous pulls.
- The existing atomic publication and pending-flush path remain intact for
  register writes during block reads.
