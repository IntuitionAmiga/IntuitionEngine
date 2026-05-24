
# Appendix E - Music Note and Frequency Tables

Each audio engine programs pitch through either a direct Hertz value,
a period/divisor, or a chip-specific frequency register. This appendix
gives the formula for each engine and a 12-note octave-4 table.

The reference frequencies are:

- **SoundChip / SFX / WAV / MOD / Amiga Paula DMA:** programmed in
  Hertz directly (the mixer runs at the system output sample
  rate). No divisor formula applies.
- **PSG / AY-3-8910:** master clock `2,000,000` Hz, period
  `clock / (16 * f)`.
- **SN76489:** master clock `3,579,545` Hz (NTSC), period
  `clock / (32 * f)`.
- **SID:** master clock `985,248` Hz (PAL), period
  `f * 16777216 / clock`.
- **TED audio:** PAL master clock `886,724` Hz, sound clock
  `110,840` Hz, register `1024 - sound_clock / f`.
- **POKEY:** master clock `1,789,773` Hz (NTSC) divided by `28`,
  divisor `clock / 28 / f - 1`.

## E.1 Octave 4 (middle C through B), equal temperament A4 = 440 Hz

| Note | f (Hz)  | PSG period | SN76489 period | SID period | TED register | POKEY divisor |
|------|---------|------------|----------------|------------|--------------|----------------|
| C    | 261.63  | 478        | 427            | 4455       | 600          | 243            |
| C#   | 277.18  | 451        | 403            | 4720       | 624          | 229            |
| D    | 293.66  | 426        | 381            | 5001       | 647          | 216            |
| D#   | 311.13  | 402        | 359            | 5298       | 668          | 204            |
| E    | 329.63  | 379        | 339            | 5612       | 688          | 192            |
| F    | 349.23  | 358        | 320            | 5947       | 707          | 182            |
| F#   | 369.99  | 338        | 302            | 6300       | 724          | 171            |
| G    | 392.00  | 319        | 285            | 6675       | 741          | 162            |
| G#   | 415.30  | 301        | 269            | 7073       | 757          | 153            |
| A    | 440.00  | 284        | 254            | 7493       | 772          | 144            |
| A#   | 466.16  | 268        | 240            | 7939       | 786          | 136            |
| B    | 493.88  | 253        | 226            | 8410       | 800          | 128            |

## E.2 Extending the table

To move up one octave (double the frequency): halve the PSG and
SN76489 periods, double the SID period, halve the POKEY divisor,
and for TED halve the internal divisor:

```
newTED = 1024 - INT((1024 - oldTED) / 2)
```

To move down one octave (halve the frequency): double the PSG and
SN76489 periods, halve the SID period, double the POKEY divisor,
and for TED double the internal divisor:

```
newTED = 1024 - 2 * (1024 - oldTED)
```

The relation is exact to within rounding error. Octave shifts of more
than four become noticeably flat or sharp on the small-divisor chips
(SN76489, POKEY, and high TED values) and need a tempered correction
table for accurate music.

## E.3 SoundChip and modern engines

The SoundChip channels and the sample-based engines (WAV, MOD,
SFX, Amiga Paula DMA) take frequency in Hertz directly through
the `FREQ` register of the channel block. A program writes
`int(round(f))` into that register and the engine plays at that
frequency. No table is needed: middle C is `262`, A4 is `440`.

## E.4 Tuning notes

The numbers in section E.1 are calculated, not measured. The
real-silicon SID, PSG, and POKEY chips drift with temperature and
with the precise master-clock crystal; an Intuition Engine
program that needs the same pitch on every machine should derive
its own period from the engine clock at startup rather than hard-
coding the values above.

The reference table assumes equal temperament. A program that
wants just intonation, meantone, or other historical tunings
generates the period table from the desired ratios using the same
divisor formula. The engines have no awareness of a "scale": each
channel plays whatever pitch its period register encodes.
