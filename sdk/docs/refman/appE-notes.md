---
title: "Music Note and Frequency Tables"
---

Copyright (c) 2026 Zayn Otley. All rights reserved.

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
- **MIDI/MUS:** use MIDI note numbers. The player converts note `69`
  to A4 = `440` Hz and applies pitch bend internally.

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

## E.3 SoundChip, MIDI/MUS, and modern engines

The SoundChip channels and the sample-based engines (WAV, MOD, SFX,
Amiga Paula DMA) take frequency in Hertz directly through the `FREQ`
register or period model owned by that engine. A program writes
`int(round(f))` into a SoundChip `FREQ` register and the channel plays
at that frequency. No table is needed: middle C is `262`, A4 is `440`.

MIDI/MUS does not expose note pitch as a divider register. The file
stores note numbers, programme changes, volume/expression controllers,
tempo, and pitch bend. The RawlandMini synth converts those note
numbers to frequency internally, with note `60` as middle C and note
`69` as A4.

## E.4 RawlandMini programme numbers

RawlandMini accepts `128` melodic programme numbers. They follow the
GM-style numbering and family order, but they select RawlandMini's
built-in IE-native patches. They are not a promise that sampled
General MIDI instruments are present.

| No. | Selection | No. | Selection |
|-----|-----------|-----|-----------|
| `0` | Acoustic grand piano | `64` | Soprano sax |
| `1` | Bright acoustic piano | `65` | Alto sax |
| `2` | Electric grand piano | `66` | Tenor sax |
| `3` | Honky-tonk piano | `67` | Baritone sax |
| `4` | Electric piano 1 | `68` | Oboe |
| `5` | Electric piano 2 | `69` | English horn |
| `6` | Harpsichord | `70` | Bassoon |
| `7` | Clavinet | `71` | Clarinet |
| `8` | Celesta | `72` | Piccolo |
| `9` | Glockenspiel | `73` | Flute |
| `10` | Music box | `74` | Recorder |
| `11` | Vibraphone | `75` | Pan flute |
| `12` | Marimba | `76` | Blown bottle |
| `13` | Xylophone | `77` | Shakuhachi |
| `14` | Tubular bells | `78` | Whistle |
| `15` | Dulcimer | `79` | Ocarina |
| `16` | Drawbar organ | `80` | Lead 1 square |
| `17` | Percussive organ | `81` | Lead 2 sawtooth |
| `18` | Rock organ | `82` | Lead 3 calliope |
| `19` | Church organ | `83` | Lead 4 chiff |
| `20` | Reed organ | `84` | Lead 5 charang |
| `21` | Accordion | `85` | Lead 6 voice |
| `22` | Harmonica | `86` | Lead 7 fifths |
| `23` | Tango accordion | `87` | Lead 8 bass and lead |
| `24` | Nylon guitar | `88` | Pad 1 new age |
| `25` | Steel guitar | `89` | Pad 2 warm |
| `26` | Jazz guitar | `90` | Pad 3 polysynth |
| `27` | Clean guitar | `91` | Pad 4 choir |
| `28` | Muted guitar | `92` | Pad 5 bowed |
| `29` | Overdriven guitar | `93` | Pad 6 metallic |
| `30` | Distortion guitar | `94` | Pad 7 halo |
| `31` | Guitar harmonics | `95` | Pad 8 sweep |
| `32` | Acoustic bass | `96` | FX 1 rain |
| `33` | Fingered bass | `97` | FX 2 soundtrack |
| `34` | Picked bass | `98` | FX 3 crystal |
| `35` | Fretless bass | `99` | FX 4 atmosphere |
| `36` | Slap bass 1 | `100` | FX 5 brightness |
| `37` | Slap bass 2 | `101` | FX 6 sound effect |
| `38` | Synth bass 1 | `102` | FX 7 echoes |
| `39` | Synth bass 2 | `103` | FX 8 sci-fi |
| `40` | Violin | `104` | Sitar |
| `41` | Viola | `105` | Banjo |
| `42` | Cello | `106` | Shamisen |
| `43` | Contrabass | `107` | Koto |
| `44` | Tremolo strings | `108` | Kalimba |
| `45` | Pizzicato strings | `109` | Bag pipe |
| `46` | Orchestral harp | `110` | Fiddle |
| `47` | Timpani | `111` | Shanai |
| `48` | String ensemble 1 | `112` | Tinkle bell |
| `49` | String ensemble 2 | `113` | Agogo |
| `50` | Synth strings 1 | `114` | Steel drums |
| `51` | Synth strings 2 | `115` | Woodblock |
| `52` | Choir aahs | `116` | Taiko drum |
| `53` | Voice oohs | `117` | Melodic tom |
| `54` | Synth voice | `118` | Synth drum |
| `55` | Orchestra hit | `119` | Reverse cymbal |
| `56` | Trumpet | `120` | Guitar fret noise |
| `57` | Trombone | `121` | Breath noise |
| `58` | Tuba | `122` | Seashore |
| `59` | Muted trumpet | `123` | Bird tweet |
| `60` | French horn | `124` | Telephone ring |
| `61` | Brass section | `125` | Helicopter |
| `62` | Synth brass 1 | `126` | Applause |
| `63` | Synth brass 2 | `127` | Gunshot |

## E.5 RawlandMini drum notes

On MIDI channel `9`, note numbers select the RawlandMini drum table.
Notes `35` through `81` have distinct GM-style drum selections. Other
drum-channel notes use a default noise hit.

| Note | Drum selection | Note | Drum selection |
|------|----------------|------|----------------|
| `35` | Acoustic bass drum | `59` | Ride cymbal 2 |
| `36` | Bass drum 1 | `60` | High bongo |
| `37` | Side stick | `61` | Low bongo |
| `38` | Acoustic snare | `62` | Mute high conga |
| `39` | Hand clap | `63` | Open high conga |
| `40` | Electric snare | `64` | Low conga |
| `41` | Low floor tom | `65` | High timbale |
| `42` | Closed hi-hat | `66` | Low timbale |
| `43` | High floor tom | `67` | High agogo |
| `44` | Pedal hi-hat | `68` | Low agogo |
| `45` | Low tom | `69` | Cabasa |
| `46` | Open hi-hat | `70` | Maracas |
| `47` | Low-mid tom | `71` | Short whistle |
| `48` | High-mid tom | `72` | Long whistle |
| `49` | Crash cymbal 1 | `73` | Short guiro |
| `50` | High tom | `74` | Long guiro |
| `51` | Ride cymbal 1 | `75` | Claves |
| `52` | Chinese cymbal | `76` | High woodblock |
| `53` | Ride bell | `77` | Low woodblock |
| `54` | Tambourine | `78` | Mute cuica |
| `55` | Splash cymbal | `79` | Open cuica |
| `56` | Cowbell | `80` | Mute triangle |
| `57` | Crash cymbal 2 | `81` | Open triangle |
| `58` | Vibraslap |  |  |

## E.6 Tuning notes

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
