# Intuition Engine Programmer's Reference Guide - Author Style Guide

Copyright (c) 2026 Zayn Otley. All rights reserved.

Author-only. Stripped at publish time. Never shipped to readers.

This file is the binding plan for the Programmer's Reference Guide. If
a chapter conflicts with this file, the chapter is wrong.

## Canonical Source Rule

All technical claims about Intuition Engine, its CPUs, buses, memory
model, devices, BASIC behaviour, monitor commands, MMIO registers,
data formats, status bits, errors, timing, and examples must be
adversarially checked against the code and source assets on disk before
they are published.

The repository is the canonical source of truth. Relevant Go code,
assembly include files, tests, constants, generated tables, runtime
assets, and existing verified examples outrank prose reviews, older
manual text, assumptions, historical chip behaviour, and author memory.
When an outside review or user comment raises a possible correction,
treat it as a hypothesis, find the source-owned implementation, and
then update the plan, manual, ledger, and tests to match the code.

If source code and reader-facing documentation disagree, the
documentation is wrong unless the code itself is being changed in the
same pass and verified. Record the exact files checked in
`verify/CLAIM_LEDGER.txt`.

## Book Identity

The published title is **Intuition Engine Programmer's Reference
Guide**.

## Copyright And Licence

The Programmer's Reference Guide is proprietary book text, not GPL and
not free or open licence documentation. Every reader-facing source file
and every published Markdown file must carry the notice shown at the
top of this file.

Place the notice after stripped front matter and before the first
reader-facing section. Do not replace it with project licence wording.
Do not imply that the book text is covered by the repository licence.

The book is a guide and a reference, not a cold hardware dump. Its
central premise is that Intuition Engine is one shared-memory
backplane computer. The reader is not moving between unrelated
machines. The reader is programming another card, engine, processor,
or bus master attached to the same computer.

Intuition Engine is described as a modern `64`-bit RISC machine built
as an homage to `1980s` and `1990s` home computing, re-imagining ideas
from Commodore/Atari/Sinclair/BBC/Amstrad/IBM `8`/`16`/`32`-bit home
computers. Do not frame it as one preserved historical machine or as an
`8`-bit-only nostalgia machine. Voodoo, x86, IE64, high RAM, Lua script
automation, and other later features are part of the premise, not
exceptions to apologise for.

The system bus rule is architectural, and every chapter that touches
memory, MMIO, CPU access, DMA, or diagrams must follow it:

- Intuition Engine has one `64`-bit physical bus.
- The bus carries `64`-bit physical addresses and supports `8`, `16`,
  `32`, and `64`-bit transfers.
- The low `32`-bit window contains the fixed legacy memory map, MMIO
  registers, video apertures, compatibility CPU working space, and most
  typed examples.
- IE64 can reach the full `64`-bit physical range through its wide
  physical path and MMU. Its page-table entries use a `52`-bit physical
  page number plus the `4` KB page offset, yielding a full `64`-bit
  physical address.
- IE32, M68K, and x86 are `32`-bit bus clients and see the low `4` GB
  window directly. The M68K chapter must describe the Intuition Engine
  CPU as a full MC68020-class `32`-bit address/data machine, not a
  `24`-bit 68EC020 or `16`-bit 68000 bus.
- The 6502 and Z80 are `16`-bit clients with adapters and banked
  apertures into the wider bus.
- `8`-bit and `16`-bit hardware registers are device-width adapters on
  the same bus. Do not imply that a byte-wide device makes the system
  bus byte-wide.

Every chapter must support that premise:

- Explain where the feature sits in the shared machine.
- Explain what other parts of the machine can see or drive it.
- Use examples that make the shared bus visible, audible, or
  inspectable wherever practical.
- Prefer "this chip on the machine" language over isolated
  device-manual language.
- When several processors or chips can use the same feature, make that
  shared access explicit.

The editorial rule is: explain the idea first, then give the exact
register truth. If prose reads like a specification before the reader
knows why the feature matters, rewrite the idea section or move the
detail into a table, notes-and-limits section, or appendix.

The current editorial pass is driven by the final review:

- Add a short first-session path before the vocabulary wall. A new
  reader should type one arithmetic line, one stored program, one
  visible graphics action, one audible action, and one save/list action
  before being sent into the full alphabetical keyword reference.
- Keep Chapter 2 as a reference, but remove reader-facing tokeniser
  leaks. Internal token names, token-byte aliases, and parser escape
  details belong in Appendix A unless the spelling is genuinely typed by
  the reader. Every reader-visible keyword or function named by an
  appendix must have a Chapter 2 entry, and every Chapter 2 hardware
  verb must match the owning chapter's syntax.
- Reduce repeated audio Plus-mode prose. Chapter 11 owns the shared
  explanation of Plus processing as a mixer/output path. Individual
  chip chapters should document only the chip-specific register, BASIC
  verb, audible difference, and limits.
- Add a small number of cohesive "build a thing" through-lines where
  chapters naturally meet, especially across graphics, audio, and
  cross-CPU work. Do not add unrelated examples to narrow lookup
  chapters.
- Clarify trap and exception tables. Do not use vague sibling-fault
  wording. If a CPU has no exact equivalent for a cause, say so.
- Make appendix trust non-negotiable. Appendices must mechanically
  match source-owned constants, main-chapter register names, ranges,
  status meanings, and opcode summaries. If an appendix simplifies, it
  must not rename, shorten, or contradict the main chapter.

Current controlled polish pass:

- Put the `DEF` / `TROFF` token-collision note in Chapter 2 as well
  as Appendix A, because it affects what a reader sees after `LIST`.
- Add a Chapter 11 comparison table that starts with the IE-native
  SoundChip and SFX features, then compares the legacy tone chips,
  tracker engines, sample players, and Paula DMA.
- Add short framing before dense VideoChip and Voodoo sections so the
  reader knows why the feature matters before the register truth.
- In Chapter 9, document the Voodoo command pipeline against
  `video_voodoo.go` and `voodoo_constants.go`: whether `TRIANGLE_CMD`
  waits or queues, when pixels become visible, what flushes the queue,
  what happens when the batch is full, and what `FBI_BUSY`, `SST_BUSY`,
  `MEMFIFO`, and `PCIFIFO` actually mean in Intuition Engine.
- Fix the appendix consistency review items: Appendix B must describe
  TED text colour as the 8-bit TED colour byte used by Chapter 6,
  Chapter 7 must not send GTIA colour lookup to Appendix B, Appendix D
  must list the exact `VIDEO_MODE` value map from `video_chip.go`, and
  Appendix K's compositor diagram must match the Chapter 3 layer order
  and the source layer constants.
- Fix the final consistency review items: Appendix D must describe
  `VIDEO_STATUS` with the `HAS_CONTENT`, `VBLANK`, and `FB_ERR` bits
  from `video_chip.go`; Chapter 24, Appendix D, and Appendix J must use
  the TED video range ending at `$F0F6B`; Chapter 24 must not label the
  `$F0C40` and `$F0D40` SoundChip flex blocks as real SID2/SID3
  registers; Appendix E must use the TED `1024 - register` pitch model
  from Chapter 16 and `ted_engine.go`; and Appendix L must include the
  common register-level lookup terms raised by review.
- Integrate the IE64 monitor assembler added in commit `9868100`.
  This is an IE-native monitor feature, not a host toolchain. Chapter
  25 and Chapter 33 may teach `A addr` as the readable way to enter
  IE64 one-instruction-at-a-time code, but the book must keep emitted
  bytes, `d` disassembly, and run/inspection results as the proof path.
  Non-IE64 CPU chapters remain byte-entry chapters unless IE Mon gains
  native assemblers for those CPUs.
  `A` mode is interactive only and cannot be fed by IE Script or
  monitor wrappers, so published `A` transcripts are marked as text and
  verified against `debug_asm` and `internal/asm/ie64` tests. The
  paired byte-entry transcript remains the runnable PRM sweep path.
- Integrate SMF, MUS, and the RawlandMini GM synth path added in
  commit `0ff06b2`. This is a first-class audio player/synth path, not
  a footnote under the media loader. Insert a new Chapter 21,
  "MIDI/MUS and RawlandMini GM Synth", after WAV and before Paula DMA.
  Renumber Paula and every following chapter by one, update all
  reader-facing chapter references, update the preface contents, and
  regenerate the publish tree and PDFs only after the source tree is
  internally consistent. Claims about this feature must be checked
  against `midi_constants.go`, `midi_parser.go`, `midi_engine.go`,
  `midi_player.go`, `media_loader.go`, `media_loader_constants.go`,
  `script_engine.go`, `registers.go`, the SDK include files, and the
  MIDI/media ABI tests. The book may describe the built-in table as
  `RawlandMini`, with GM program and drum mapping, but must not imply
  external soundfont loading or exact external GM hardware emulation
  unless the source implements it.
- Add a small whole-machine capstone chapter that touches graphics,
  audio, file I/O, and the coprocessor status path from BASIC.
- Add a traditional lookup index appendix and include it in the
  preface contents.
- Integrate the ABI changes from commits `f8c3570` and `3b9c91d`.
  MIDI/MUS status bit `3` is now `MIDI_STATUS_LOADING`, set while an
  asynchronous parse/load request is still in progress. The terminal
  input block exposes `RTC_MONO_USEC_LO` and `RTC_MONO_USEC_HI` as a
  monotonic microsecond timer since engine start. The x86 flat image
  start contract is `EIP = 0` for `.ie86` images. File reads ignore
  stale `FILE_DATA_LEN`; successful reads report the actual byte count
  in `FILE_RESULT_LEN`, and accepted-path read failures clear
  `FILE_RESULT_LEN` to `0`. Update Chapters 21, 24, 30, 35, and 37,
  then Appendices D, H, I, and L, checking the wording against
  `midi_constants.go`, `midi_player.go`, `registers.go`,
  `terminal_io.go`, `file_io.go`, `file_io_test.go`, the SDK include
  files, and the ABI drift tests.

  Execute this ABI pass in this order:

  1. Chapter 21: add `MIDI_STATUS_LOADING` to the register and status
     explanation; add a native BASIC polling example that waits for bit
     `3` to clear and checks the error bit.
  2. Chapter 24: add `RTC_MONO_USEC_LO` and `RTC_MONO_USEC_HI` to the
     terminal block; mention MIDI/MUS loading status in the player map
     only where a status summary is already present.
  3. Chapter 30: clarify the x86 `.ie86` flat-image start contract:
     loaded images start at `EIP = 0`; monitor examples may still set
     `EIP` to another address by hand.
  4. Chapter 35: clarify that `FILE_DATA_LEN` is write-side state and is
     ignored by reads; successful reads set `FILE_RESULT_LEN` to the
     actual byte count, and accepted-path read failures clear it to `0`.
  5. Chapter 37: add a monotonic elapsed-time section after `RTC_EPOCH`,
     with high-low-high read guidance and a typed BASIC example.
  6. Appendix D: update Terminal/Input, MIDI/MUS, and File I/O rows.
  7. Appendix H: add the new shared terminal timing symbols and the x86
     image start note.
  8. Appendix I: record the File I/O failed-read `FILE_RESULT_LEN = 0`
     behaviour where file block errors are summarised.
  9. Appendix L: add lookup entries for `MIDI_STATUS_LOADING`,
     `RTC_MONO_USEC_LO`, `RTC_MONO_USEC_HI`, and `.ie86`.
  10. Claim ledger: record the checked canonical sources and the
      reader-facing examples affected by this pass.
  11. Publish and print PDFs only after the source pass and checks are
      complete.
- Integrate the VideoChip blitter MEMCOPY change from commit
  `72fd188`. This commit added a demo program, but the reader-facing
  book must not mention that demo, its title, its asset paths, or its
  host-side run instructions. The book-relevant claim is only the
  VideoChip ABI change: `BLT_OP = 8` is a distinct byte-counted linear
  memory-copy operation, exposed from BASIC as `BLIT MEMCOPY` and
  `BLIT M`.

  Execute this MEMCOPY pass in this order:

  1. Check `video_chip.go`, `video_blitter_test.go`,
     `sdk/include/ehbasic_hw_system.inc`, the SDK include files, and
     the BASIC BLIT tests before writing claims.
  2. Chapter 2: make sure `BLIT MEMCOPY` and `BLIT M` are described as
     byte-span operations, not pixel rectangles.
  3. Chapter 4: document `MEMCOPY` as operation `8`, separate it from
     rectangular `COPY`, state that `BLT_WIDTH` is the byte count for
     this operation, state which registers matter, and add a small
     IE-native BASIC example that copies an off-screen buffer into the
     visible framebuffer and reads `BLT_STATUS`.
  4. Appendix D: make the VideoChip blitter map and operation summary
     include `MEMCOPY`.
  5. Appendix L: add lookup entries for `BLIT MEMCOPY`, `BLIT M`, and
     `BLT_OP_MEMCOPY`.
  6. Claim ledger: record the canonical sources checked and the typed
     reader example.
  7. Publish and print PDFs only after the source pass and checks are
     complete.
- Integrate the x86 and backed-RAM behaviour changes from commit
  `794d368`. This commit also contains runtime diagnostics and
  compatibility-oriented fixes that are not book features. Do not add
  file-format lore or game-specific prose while documenting this pass.
  The reader-facing changes are:

  - x86 implements `CMOVcc` (`0F 40`-`0F 4F`) in the flat-mode
    instruction set. The source operand is still read when the
    condition is false.
  - x86 data accesses can reach native MMIO addresses at
    `$000F0000`-`$000FFFFF` directly, and the `$F000`-`$FFFF`
    compatibility mirror remains a data-access mirror only.
    Instruction fetch at `$F000` reads flat program RAM at `$0000F000`.
  - Backed RAM above the low memory slice is ordinary active RAM, but
    scalar word and long bus accesses must fit wholly inside low RAM or
    wholly inside backed RAM. A scalar word or long access that
    straddles the seam is unmapped and does not partly update either
    side. Byte-by-byte copies, including File I/O reads, may still cross
    the seam when every byte lies inside active RAM.

  Execute this x86/backed-RAM pass in this order:

  1. Check `cpu_x86.go`, `cpu_x86_ops.go`, `cpu_x86_runner.go`,
     `cpu_x86_test.go`, `machine_bus.go`, `file_io.go`,
     `file_io_test.go`, and `debug_access_test.go` before writing
     claims.
  2. Chapter 24: clarify that ordinary byte access may live in backed
     active RAM, and that scalar word and long accesses may live there
     only when the whole access is contained on one side of the low-RAM
     to backed-RAM seam.
  3. Chapter 30: update the x86 overview, memory model, and instruction
     list for `CMOVcc`, native MMIO data access, and fetch-vs-data
     treatment of the `$F000` compatibility mirror.
  4. Chapter 35: state that `FILE_DATA_PTR` may point to any valid
     active-RAM destination span and that reads may cross the low/backed
     RAM boundary because the file block copies one byte at a time.
  5. Appendix G: add `CMOVcc` to the x86 opcode quick reference.
  6. Appendix H: add the x86 MMIO/fetch-address rule.
  7. Appendix L: add lookup entries for backed RAM, `CMOVcc`,
     `FILE_DATA_PTR`, and x86 MMIO access.
  8. Claim ledger: record the canonical sources checked and the
     affected reader-facing examples.
  9. Publish and print PDFs only after the source pass and checks are
     complete.
- Integrate the later backed-RAM seam correction. The book must no
  longer imply that all multi-byte RAM accesses can straddle the seam
  between the low memory slice and backed RAM. Check `machine_bus.go`,
  `machine_bus_test.go`, `file_io.go`, and `file_io_test.go` before
  writing claims. Chapter 24 owns the scalar bus-access rule. Chapter
  35 owns the File I/O byte-copy exception. The claim ledger must record
  both facts together so the File I/O exception is not mistaken for a
  general scalar bus rule.
- Run a full source-tree editorial audit after any manually edited
  refman Markdown. Classify every `.md` file under `sdk/docs/refman/`
  before checking it:
  - Reader-facing files are `00-Preface.md`, numbered chapter files, and
    `appA` through `appL`. They must pass the forbidden-term scan with
    front matter stripped, the no-em/en-dash rule, British-English prose
    checks, valid chapter/appendix cross-reference checks, and publish
    consistency checks.
  - Author-only files are `STYLE.md`, `AUTHOR_PROVENANCE.md`, and
    files under `verify/`. They may contain source paths, implementation
    notes, and external provenance where the plan allows it, but they
    must not be copied to the publish tree. Do not rewrite author-only
    evidence files merely to satisfy reader-facing wording rules.
  - If this pass changes any reader-facing source file, regenerate the
    publish tree and PDFs only after the source tree is clean.
- Integrate the documentation-facing changes from commit `1300567`.
  This is a focused consistency pass, not a renumbering or feature
  expansion pass. Check `cpu_ie32.go`, `cpu_ie64.go`,
  `debug_commands.go`, `debug_snapshot.go`, `script_engine.go`,
  `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
  `sdk/docs/iemon.md`, and `sdk/docs/iescript.md` before writing
  claims. Execute this pass in this order:

  1. Chapter 25: state that IE64 `TIMER_PERIOD` and `TIMER_COUNT`
     use decoded-instruction timer-step units, not host cycles or
     wall-clock time. State that `MTCR` to `CR_RAM_SIZE_BYTES`
     raises `FAULT_ILLEGAL_INSTRUCTION`. State that `TLBINVAL Rs`
     treats `Rs` as a virtual address and invalidates that address's
     VPN. State that nested trap preservation is architectural through
     the trap-frame stack, so a normal handler need not save
     `CR_FAULT_PC` or `CR_SAVED_SUA` merely to survive nesting.
  2. Chapter 26: state that IE32 `WAIT n` waits approximately `n`
     microseconds during normal execution. Also state that IE Mon
     single-step advances past `WAIT` without sleeping.
  3. Chapter 31: replace IE64 cycle-timer wording with
     decoded-instruction-step timer wording, while keeping heritage CPU
     cycle-count prose separate from IE64 control-register timing.
  4. Chapter 33: state that IE Mon `ss` and `sl` are CPU-local
     snapshots, not whole-machine save states. Point whole-machine
     reverse-history work at `rg`, `rt`, `tl`, and `history`. Add the
     `trace mmio <region> [count]` monitor command where bus/MMIO
     inspection is summarised.
  5. Chapter 34: add the monitor-parity IE Script helpers for history
     configuration, device snapshots and diffs, trace rings,
     structured backtraces, and CPU-local state save/load. State that
     `dbg.save_state` and `dbg.load_state` follow IE Mon `ss`/`sl`
     scope and do not save the whole machine.
  6. Appendices G, H, I, and L: update IE32 `WAIT`, IE64 illegal
     instruction wording, and lookup summaries to match the chapters.
  7. Claim ledger: record the checked canonical sources and the
     reader-facing claims changed by this pass.
  8. Publish and print PDFs only after the source pass and checks are
     complete.

## Reader Contract

The book is for developing **on Intuition Engine for Intuition Engine**.
The reader-facing workflow is:

- Type BASIC in direct mode or as numbered BASIC lines.
- Use `PEEK`, `POKE`, BASIC graphics/audio/file commands, and ordinary
  BASIC variables for first contact with hardware.
- Enter IE Mon with `MON`.
- Use IE Mon `w` to write machine-code bytes, `d` to inspect the
  disassembly, `r` to set or read registers, `s` to step, `g` to run,
  and `b`/`bc` for breakpoints.
- For IE64 only, use IE Mon `A addr` when a readable mnemonic entry
  path helps the reader. `A` is part of the machine monitor and accepts
  one IE64 instruction per line. It does not change the requirement to
  show the emitted bytes, confirm them with `d`, and run or inspect the
  result.
- Inspect results through registers, memory dumps, visible screen
  changes, terminal output, or documented status registers.

Reader-facing examples must not require a host SDK assembler, a build
command, a source path, a local checkout, an external toolchain, or an
external manual. Author-side tools may be used to verify bytes and
claims, but the chapter must present the IE-native workflow.

## Voice

One human voice runs through the whole book: a programmer at the
machine, explaining what to try, what it means, and what exact hardware
rule is underneath it. The tone changes by part, but the book must not
turn into generated contract text or cleaned-up engineering notes.

Avoid mechanical repetition in example explanations. "Expected result",
"Line X does", and "Try changing" are useful tools, not required
headings for every listing. Vary the prose so the reader feels guided,
not processed through a template.

Two registers in this book:

- **Parts I, II, III (BASIC, Graphics, Sound)** - 1982 tutorial voice. Short paragraphs. Numbered example programs. "Try this:", "Type this:", "NOTE:". Imperatives. `POKE` and `PEEK` are the working idiom. Plain English at all times.
- **Parts IV, V (Machine Language, I/O)** - modern technical reference voice. ISA tables. ABI sections. MMIO bit-fields. Still terse, still readable.

Appendices take whichever voice belongs to the Part they support.

## Language and Punctuation

Reader-facing chapters and appendices use British English.

- Use British spellings in prose: colour, behaviour, centre, metre,
  initialise, recognise, summarise, tokenised, serialised, grey,
  neighbour, and similar forms.
- Use `program` for computer code. Do not change it to `programme`
  when referring to BASIC, machine code, scripts, loaded images, or
  executable text.
- Do not alter identifiers, BASIC keywords, register names, status
  names, opcodes, quoted output, filenames, or command transcripts to
  force British spelling. `BLT_COLOR`, `COLOR_MODE`, and `PALETTE`
  remain exact.
- No em dash or en dash characters are allowed in reader-facing
  Markdown. Use a comma, colon, semicolon, parentheses, or a spaced
  hyphen instead. Numeric ranges use a plain hyphen: `0-255`.

## Notation

- Numeric literals: hex written `$1F00`. Decimal written without prefix.
- Bit fields: `D7 D6 D5 D4 D3 D2 D1 D0`, MSB on the left.
- Cross-references: "see Chapter NN" or "see Appendix X". Never paths. Never links.
- Example programs: numbered listings with BASIC line numbers (`10 PRINT "HELLO"`).
- Monitor sessions: shown as transcripts, prompt and response.
- Error messages: quoted exactly, in monospace.

## IE-Native Examples

Every chapter needs at least one example that can be typed directly
into Intuition Engine. Choose the simplest native path that exercises
the feature:

- BASIC chapters use numbered BASIC listings and direct-mode commands.
- MMIO and device chapters start with BASIC `POKE`/`PEEK` examples
  before machine-language examples.
- IE Mon chapters and machine-language chapters use monitor
  transcripts.
- Script/file chapters may use machine-visible filenames, but must not
  turn those examples into host setup or build instructions.

Examples should be worth typing. A first example may be small, but the
chapter's main examples should draw a picture, animate a visible
effect, make sound, move data through a real device, or show two
machine parts cooperating. Avoid examples whose only result is a
sentinel byte unless the feature has no visible or audible surface.

Every substantial example must teach, not merely dump code. Use this
shape unless the example is only a two-line direct-mode check:

1. A short "what this does" paragraph before the listing.
2. Comments inside the listing when they help the reader keep their
   place. In BASIC listings, prefer sparse `REM` lines for phase
   markers rather than comments on every line.
3. A "how it works" paragraph or compact line-range notes after the
   listing. Explain the setup lines, the data-format lines, the control
   write that starts the device, and the status/readback line.
4. A small "try changing" note when the example has an obvious safe
   variation, such as a colour, divider, volume, period, channel,
   pitch, stride, or buffer address.

Do not count a listing as complete if a reader can type it but cannot
explain why it works. The examples are part of the guide voice. Tables
are the reference voice. A chapter needs both.

Substantial runnable chapters should also teach that Intuition Engine is
one shared machine, not a pile of isolated devices. Do not impose a
mechanical "one audio and one graphics listing everywhere" quota, since
that would bloat lookup chapters and distort narrow topics. Instead use
this rule:

- CPU chapters must have both an audio proof and a graphics showcase.
- BASIC tutorial and cookbook chapters should include both visible and
  audible examples when the chapter is teaching programming technique.
- Video chapters should include an audio or timing companion when it
  clarifies synchronisation, shared memory, events, or presentation.
- Audio chapters should include a visual companion when it naturally
  helps the reader inspect state, timing, levels, envelopes, or playback.
- File, serial, host, monitor, error, token, opcode, symbol, and lookup
  chapters should not be padded with unrelated audio/graphics material;
  they need examples that prove their own feature and may point to a
  neighbouring chapter for cross-media use.

When a chapter does include both audio and graphics examples, vary the
chips and features across the book. The result should feel like one
computer with many cards on a common bus, not repeated boilerplate.

BASIC `WAIT` is not a delay statement. It is only `WAIT addr,mask[,xor]`
and polls a 32-bit memory-visible value until `((value EOR xor) AND
mask)` is non-zero, or until the built-in timeout expires. Do not use
single-argument `WAIT n` in BASIC listings. Use device status polling,
`VSYNC` where appropriate, or a plain counted `FOR ... NEXT` busy loop
when an audio or video example merely needs time to pass.

Machine-language examples must include all three of these parts:

1. The bytes to enter with IE Mon `w`.
2. The expected `d` disassembly.
3. The expected result after `s`, `g`, or a breakpoint-assisted run.

IE64 examples may include an `A addr` transcript before the byte-entry
form. Use it to make the program readable, especially when the old
byte stream would be hard to follow. The `A` transcript must be
native to IE Mon and must show the monitor's emitted bytes for each
instruction shown. Do not present standalone source-file assembly as
the reader workflow, and do not remove the byte-entry proof unless the
  example is a tiny local demonstration of `A` itself in Chapter 33.

CPU chapter examples should do visible and audible machine tasks, not
only store a sentinel byte in RAM. Each CPU chapter needs two native
monitor-entered programmes unless the implementation makes one
impossible and the ledger records why:

1. A compact audio proof that uses a sound engine.
2. A graphics showcase that uses a distinct video chip or a distinct
   hardware feature of a video chip.

The graphics showcase must be more than a colour poke. It should draw,
animate, scroll, fill, copy, texture, change raster state, or otherwise
show a characteristic hardware capability. It must include bytes,
expected disassembly, expected visible or memory result, and practical
commentary for every instruction group and data table. The text should
tell the reader what they should see, what memory or registers prove it,
and what one safe visual parameter they can change.

Use this target spread for Chapters 25-30 unless source truth forces a
better assignment:

| Chapter | CPU  | Audio proof target | Graphics showcase target |
|---------|------|--------------------|--------------------------|
| 25 | IE64 | SoundChip chord | VideoChip Mode 7 affine texture or, if that is too large for hand entry, VideoChip blitter/copper with visible raster output |
| 26 | IE32 | SN76489 chord | VGA text/attribute or palette display |
| 27 | 6502 | POKEY chord | ULA bitmap plus attribute memory |
| 28 | Z80 | PSG chord | ANTIC/GTIA display-list or playfield-colour setup |
| 29 | M68K | SID voice | Voodoo textured or shaded primitive |
| 30 | x86 | TED audio | TED video colour or raster feature |

Across the CPU chapters, vary both the sound engines and the video
chips where practical so the examples teach the shared hardware map.
Document byte groups with the same practical commentary an assembly
listing would have given: what register or port is being written, what
value is being encoded, and what the reader should see, hear, or
inspect afterward.

The reader is not assumed to have a host assembler. For IE64, IE Mon's
`A` command is an allowed native convenience because it runs inside the
monitor and immediately prints bytes. For IE32, 6502, Z80, M68K, and
x86, longer assembly listings may appear only when they are clearly
labelled as explanatory mnemonics and are paired with byte entry, or
when they are moved to author verification notes outside the published
reader path.

For each CPU ISA chapter, document enough encoding for hand entry of
small programs: instruction size, byte order, operand byte layout,
immediate format, branch displacement rules, and at least the opcode
bytes or opcode words used by the chapter's runnable example.

## Execution Order

Execute the rewrite in ascending chapter order, then appendices in
letter order. Do not jump ahead because a later chapter is more
interesting or because a nearby file is already open.

Allowed exceptions:

- A user explicitly asks for a specific later chapter.
- A blocking cross-reference, shared rule, or publication guard must be
  fixed before the current chapter can be verified.
- A mechanical global style fix is needed by the plan, such as removing
  em/en dash characters.

When an exception is used, record it in the working summary and return
to the ascending pass immediately after the blocking fix.

Each chapter pass starts by checking this file and ends by updating the
claim ledger and running the chapter scan. A chapter is not "done"
because one section improved; it is done only when every programmable
feature in that chapter satisfies the feature contract.

Structural changes are allowed only when they serve the ascending pass
and are recorded here before the chapter text is rewritten. Current
book-level structural targets:

- Add or strengthen a preface that defines Intuition Engine as one
  shared bus/backplane computer.
- Add a "first session" path before Chapter 2's vocabulary reference,
  either in the preface, Chapter 1, or both. It should be runnable from
  the BASIC prompt without external setup.
- Make Chapter 2 explicitly skimmable if it remains near the front, so
  the beginner path can continue into display, sound, and memory.
- Move Chapter 2 internals such as token aliases, untyped token names,
  and parser implementation notes into Appendix A unless they are
  necessary to type a valid program.
- Split Chapter 4 internally into VideoChip basics and advanced
  raster/blitter/copper/Mode 7 hardware before considering a chapter
  renumbering.
- Turn Chapter 10 into a whole-machine graphics cookbook.
- Make Chapter 11 the owner of common audio architecture, including
  Plus processing as a shared pattern and the top-level audio engine
  comparison. The comparison must include IE-native SoundChip/SFX,
  MIDI/MUS with RawlandMini, legacy tone chips, tracker engines,
  sample players, and Paula DMA. Per-chip Plus sections should be
  concise and non-repetitive.
- Insert the MIDI/MUS chapter as Chapter 21, then renumber the former
  Chapter 21 Paula DMA through the whole-machine capstone by one.
  Cross-references, section numbers, Appendix G CPU chapter labels,
  Appendix L index entries, publish filenames, and generated PDFs must
  agree with the new numbering.
- Rewrite Chapter 32 as an identity chapter about cross-CPU work on
  one bus before documenting the ticket protocol.
- Add examples where multiple CPUs and cards cooperate.
- Run a strict appendix consistency pass against source-owned
  constants, handlers, and disassembler output.
- Normalise heading style and chapter templates without making every
  chapter sound identical.

## Uniform Chapter Taxonomy

Name hardware chapters after the hardware block, not after every file
format, player, or helper layered on top of it. A chapter about POKEY is
`POKEY`; SAP playback is a section inside that chapter. A chapter about
SID is `The SID Family`; SID player details are a section inside it.
Apply the same pattern to PSG, SN76489, TED audio, AHX, MOD, WAV,
MIDI/MUS, VideoChip, VGA, ANTIC/GTIA, ULA, and Voodoo.

Use the same section order for programmable chips unless a chapter has a
specific reason to differ:

1. What the chip is for.
2. What it can produce.
3. Register/port map.
4. Setup order from reset.
5. Data formats and timing.
6. Status, errors, and limits.
7. BASIC `POKE`/`PEEK` or keyword example.
8. Optional player/file-format sections.

## Feature Contract

Every externally programmable feature section must explain:

- What the feature does and when to use it.
- Setup order from a clean state.
- Commands, registers, ports, or control words needed by the reader.
- Data formats, byte order, fixed-point formats, masks, strides, and
  address alignment.
- Side effects and lifetime rules.
- Status bits, completion tests, and error behaviour.
- Limitations and unsupported cases.
- One typed IE-native example that exercises the feature.

For feature-rich chips, one chapter-level example is not enough. Each
major feature group needs its own useful example. A useful example
should make the machine visibly or audibly do something characteristic
of that feature, not merely write a sentinel byte and print that it
changed. For example, a VideoChip chapter must cover framebuffer,
palette, raster band, blitter operations, copper lists, and Mode 7 with
typed examples or monitor transcripts as appropriate.

Every chapter pass must also check continuity with the shared-machine
premise:

- Name the bus-visible block or private memory the feature uses.
- State whether BASIC, MMIO, IE Mon, or another CPU can reach it.
- State what status or memory location proves the feature changed.
- Cross-reference neighbouring chips as parts of the same machine when
  that helps the reader choose a path.

## What the reader is told

The reader owns a real computer named Intuition Engine. The book never tells the reader otherwise. Forbidden terms are listed in the plan and enforced by `scripts/refman-scan-forbidden.sh`. Read them. Internalize them.

## What the reader is never told

- That IE is software running on another machine.
- That an architecture has an implementation in any particular language.
- Anything about build profiles, packaging, host platforms, or shipped guest software.
- Anything about external development tools.

## Workflow per chapter

1. Read the appropriate canonical source(s) - `.inc` files, EhBASIC asm, Go source, primary CPU manual for Ch 26-29.
2. Compose in the appropriate voice.
3. Pick the reader workflow first: BASIC prompt, `POKE`/`PEEK`, IE
   Mon byte entry, or IE64 `A` mode paired with byte proof. Do not
   start from a host assembler workflow.
4. Adversarially check every technical claim against its canonical
   source. If a prose doc was reused, fix the prose doc first in its
   own PR.
5. Record the checked sources, reader example, and author verification
   in `verify/CLAIM_LEDGER.txt`.
6. Run `scripts/refman-scan-forbidden.sh <chapter>` before considering
   the chapter done.

## Completion Checklist

A chapter is not complete until all of these are true:

- It has a typed IE-native example.
- Every machine-code example has bytes, disassembly, and result.
- Device/MMIO material includes setup, data format, status/error, side
  effects, and limitations.
- No reader-facing prose tells the programmer to use SDK assemblers,
  host build commands, source files, external manuals, or external
  toolchains as the normal workflow.
- All numeric constants and instruction encodings were checked against
  code-owned constants, disassemblers, tests, or checked primary ISA
  references.
- The claim ledger records both reader workflow and author verification
  workflow.
- The forbidden-term scan passes for the chapter, or any remaining hit
  is author-only and stripped before publication.
- British English has been applied to prose, with exact identifiers left
  untouched.
- The chapter contains no em dash or en dash characters.

## Cross-reference style

Within a chapter:

> The accumulator is described in Chapter 25.
> See Appendix G for the full opcode table.

Never:

> See `IE64_ISA.md`.
> See https:// to .
> See file `foo.inc`.
