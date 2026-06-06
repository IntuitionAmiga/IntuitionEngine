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
- Integrate the architecture-remediation commit `7d4fe8f2` as a
  focused consistency pass, not as a new feature chapter. Check
  `internal/ie64meta/table.go`, `cmd/gen_ie64_opmeta/main.go`,
  `cpu_ie64_opcodes_gen.go`, `debug_disasm_ie64_opcodes_gen.go`,
  `assembler/ie64asm_opcodes_gen.go`,
  `assembler/ie64dis_opcodes_gen.go`,
  `internal/asm/ie64/opcodes_gen.go`, `music_common.go`,
  `ahx_player.go`, `mod_player.go`, `wav_player.go`,
  `midi_player.go`, `psg_player.go`, `sid_player.go`,
  `ted_player.go`, `pokey_player.go`, `video_compositor.go`,
  `mmu_ie64.go`, `bootstrap_hostfs.go`, `machine_lifecycle.go`,
  and the related opcode, playback, video scheduler, lifecycle, and
  MMU tests before changing reader-facing claims. The reader-facing
  effects are narrow:

  - IE64 opcode values and mnemonic spellings remain the same, but the
    canonical opcode source is now the shared metadata table and its
    generated outputs. Chapter 25 and Appendix G source metadata and
    ledger entries must reflect that.
  - Register-mapped file players share the same staged
    pointer/length/start/stop/loop/busy/error rhythm. State this once
    at overview level, then keep individual chapters focused on their
    engine-specific fields.
  - Video sources are advanced from one `60` Hz frame cadence. The
    guide may say this at reader level; it must not expose scheduler
    implementation names as normal programming vocabulary.
  - IE64 page-table walking is the shared translation rule used by the
    CPU and machine services that honour guest user pointers. Keep the
    existing MMU table explanation and add only the short
    programmer-visible consequence.
  - Machine reset and lifecycle refactors do not change the ordinary
    reset contract. Do not add implementation orchestration prose to
    reader-facing chapters unless a public reset or load behaviour
    changes.

  Execute this remediation pass in this order:

  1. Update this plan entry before any chapter edits.
  2. Chapter 11: add a short shared file-player register rule after
     the engine comparison or media-loader introduction.
  3. Chapter 3 and Appendix K: state the common `60` Hz frame cadence
     and preserve the existing layer-order and scanline rules.
  4. Chapter 25 and Appendix G: update source metadata for the shared
     IE64 opcode table, without inventing new opcodes or changing
     byte-entry examples.
  5. Chapter 25: add the concise shared MMU translation note.
  6. Claim ledger: record the checked canonical sources and explain
     why no chapter renumbering or reader workflow change was needed.
  7. Run reader-facing scans, publish with strict mode, and print PDFs
     only after source and publish trees agree.
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
- Integrate the IE64 BASIC FP64 and dynamic memory layout changes from
  commit `c8e987c`. This is a book-wide correctness pass because the
  old manual described BASIC as FP32 and fixed-layout in several
  places. Claims must be checked against `sdk/include/ie64.inc`,
  `sdk/include/ehbasic_expr.inc`, `sdk/include/ehbasic_vars.inc`,
  `sdk/include/ehbasic_exec.inc`, `sdk/include/ehbasic_file_io.inc`,
  `sdk/include/ehbasic_tokens.inc`, `cpu_ie64.go`, `fpu_ie64.go`,
  `debug_disasm_ie64.go`, `assembler/ie64asm.go`,
  `assembler/ie64dis.go`, `video_chip.go`, `registers.go`, and the
  relevant FP64, assembler, memory-layout, VideoChip, and refman
  tests. Reader-facing wording must state that BASIC numbers are
  double precision, that exact qword payloads are preserved by the
  explicit 64-bit memory helpers where the implementation does so, and
  that `MEMALLOC(size[,align])` allocates public low32 buffers for
  MMIO, copper, coprocessor, and DMA examples. Do not expose private
  internal names such as `EHBASIC_PRIV_*` in reader prose.

  Execute this FP64/dynamic-layout pass in this order:

  1. Chapter 1: replace the FP32 numeric model with the FP64 model and
     keep the integer-truncation rule for integer-only operations.
  2. Chapter 2: add the missing `MEMALLOC` vocabulary entry and remove
     stale POKE64 wording that says ordinary variables are FP32.
  3. Chapter 4: document current raster-band behaviour for configured
     framebuffer bases, direct VRAM, and compositor-managed high
     framebuffers.
  4. Chapter 24: update the MMIO map wording, width table, program
     executor label, and BASIC public allocation notes against current
     constants.
  5. Chapter 25 and Appendix G: add the IE64 FP64 load/store,
     arithmetic, conversion, and transcendental instruction families,
     including `DSIN`, `DCOS`, `DTAN`, `DATAN`, `DLOG`, `DEXP`, and
     `DPOW`.
  6. Appendices F, H, I, and J: remove stale FP32 BASIC wording and add
     `MEMALLOC` or dynamic-layout lookup notes only where they belong.
  7. `verify/CLAIM_LEDGER.txt`: update claims and canonical sources for
     this pass.
  8. Run stale-term scans for FP32/single-precision BASIC claims, run
     the forbidden-term and dash scans, publish, and print PDFs only
     after the source tree is consistent.
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
- Integrate the IE64 BASIC migration wording cleanup from commit
  `4e6a9fe4`. This is a focused reader-facing consistency pass, not a
  new hardware feature pass. Check `sdk/docs/ehbasic_ie64.md`,
  `sdk/include/ie64.inc`, `sdk/include/ehbasic_vars.inc`,
  `sdk/include/ehbasic_lineeditor.inc`, `sdk/include/ehbasic_expr.inc`,
  `sdk/include/ehbasic_tokens.inc`, the BASIC AOT/runtime tests, and
  the relevant refman files before writing claims. Reader-facing prose
  should call the current prompt language and runtime `IE64 BASIC`
  unless it is explicitly discussing historical 68K EhBASIC ancestry or
  an author-only source file. Keep architectural `FP32` wording in the
  IE64 FPU chapter and CPU symbol appendix where it describes the
  single-precision `F` register path. Do not change BASIC bitwise
  operator width claims merely because an internal comment says
  "integer"; verify the actual instruction width first.

  Execute this cleanup pass in this order:

  1. Chapter 2: correct the public `VARPTR` numeric cell so tag `1` is
     `FP64` and tag `2` is `I64`.
  2. Chapter 25: use `IE64 BASIC` for current runtime conventions while
     leaving IE64 FPU `FP32` architectural wording intact.
  3. Appendices A, C, I, and L plus the Preface table of contents:
     replace current-runtime `EhBASIC` wording with `IE64 BASIC`, while
     preserving the historical 68K EhBASIC ancestry note in Appendix A.
  4. Appendix A: verify the stored-line layout against
     `ehbasic_lineeditor.inc`; document the 16-byte line header, 8-byte
     next-line pointer, 4-byte line number, 4-byte reserved field,
     null-terminated token stream, 8-byte alignment, and 8-byte
     terminator qword.
  5. Appendix I: replace the stale 32-bit floating-point overflow
     wording with the current double-precision BASIC numeric model.
  6. Update `verify/CLAIM_LEDGER.txt` with the canonical sources
     checked.
  7. Run stale-term scans for unintended reader-facing `EhBASIC` and
     stale BASIC `FP32` claims. Architectural IE64 FPU `FP32` references
     and the historical Appendix A ancestry note are allowed.
  8. Publish the stripped tree and print PDFs only after the source pass
     and scans are complete.
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
- Integrate the documentation-facing monitor changes from commits
  `50ea299` and `2558371`. This is a focused IE Mon and IE Script
  pass, not a new hardware chapter and not a reader-facing host bridge
  programming route. Check `debug_ioview.go`, `debug_ioview_read.go`,
  `debug_commands.go`, `debug_monitor_test.go`, `script_engine.go`,
  `script_engine_test.go`, `sdk/docs/iemon.md`, and
  `sdk/docs/iescript.md` before writing claims. Execute this pass in
  this order:

  1. Chapter 33: update the `io` command as the I/O register viewer,
     including `io`, `io all`, `io <device>`, native-width MMIO reads,
     and the player, sample, DMA, and bridge/profile inspection views.
     Update monitor address-expression wording for `list`, `sym add`,
     `sym resolve`, `sym loadlbl`, `addr`, `pg add`, and `who`.
  2. Chapter 34: add `dbg.io_devices()` and `dbg.io(device)` to the
     debug module, including the empty-table behaviour for unknown
     names and the shared native-width MMIO read path.
  3. Appendix L: add lookup entries for the monitor `io` command, the
     script I/O helpers, the new player/DMA/sample views, and the
     bridge/profile inspection views exposed by IE Mon.
  4. Do not add new reader examples to the audio/player chapters unless
     a source claim in those chapters is now wrong. The commits expose
     inspection surfaces; they do not change the underlying player
     programming ABIs.
  5. Claim ledger: record the checked canonical sources and the
     affected reader-facing examples.
  6. Publish and print PDFs only after the source pass and checks are
     complete.
- Integrate the BASIC native-compilation and File I/O range changes
  from commit `9e58b6b6`. This is a focused BASIC, IE64 loader, and
  File I/O pass. Do not add a new chapter. Do not expose private
  runtime-blob filenames, generator tools, source paths, build
  commands, or implementation scaffolding in reader-facing prose. The
  reader-facing idea is: BASIC can make native IE64 programs from
  inside the machine.

  Reader-facing changes from this commit:

  - `RUN AOT` is a direct-mode form that compiles the current stored
    BASIC program to native IE64 code in a top-of-RAM arena, then runs
    it immediately.
  - `COMPILE "name"` is a direct-mode form that writes a standalone
    flat `.ie64` image. The `.ie64` suffix is appended when absent, and
    output is written beside the most recently `LOAD`ed program, or to
    the File I/O root if no program has been loaded.
  - `STOP` under `RUN AOT` saves a native continuation. `CONT`
    re-enters the compiled code unless the program was edited, `NEW`,
    `LOAD`, or a fresh `RUN` / `RUN AOT` discarded the continuation.
    A `STOP` reached inside active compiled `GOSUB` nesting is not a
    resumable subroutine state.
  - `BLOAD` still uses the File I/O MMIO path and rejects destinations
    that cannot be represented by the `32`-bit `FILE_DATA_PTR` ABI.
  - File I/O error code `4` is `FILE_ERR_RANGE`: the staged transfer
    span overflows the `32`-bit File I/O address contract, reaches the
    sign-extended alias guard, or exceeds active RAM. The transfer is
    refused whole.
  - Oversized flat IE64 images are rejected before loading. A rejected
    flat image does not partially overwrite RAM and does not change
    the IE64 program counter.

  Execute this AOT / COMPILE pass in this order:

  1. Check `sdk/examples/asm/ehbasic_ie64.asm`,
     `sdk/include/ehbasic_aot.inc`,
     `sdk/include/ehbasic_file_io.inc`,
     `sdk/include/ehbasic_lineeditor.inc`, `sdk/include/ie64.inc`,
     `file_io.go`, `file_io_constants.go`, `cpu_ie64.go`,
     `program_executor.go`, `ehbasic_aot_test.go`,
     `ehbasic_aot_runtime_blob_test.go`, `file_io_test.go`,
     `program_executor_test.go`, and `cpu_ie64_flat_load_test.go`
     before writing claims.
  2. Chapter 1: add a short first-session `RUN AOT` transcript after
     the ordinary `RUN` example, keeping it as a visible continuation
     of the beginner path rather than a compiler tutorial.
  3. Chapter 2: add `COMPILE`, expand `RUN`, and update `STOP` /
     `CONT` wording. Keep `RUN AOT` and `COMPILE` as direct-mode
     forms, not ordinary stored-program vocabulary.
  4. Chapter 24: add a small note that the File I/O staging guard also
     protects the sign-extended alias boundary and the active-RAM
     limit. Do not list private AOT workspaces.
  5. Chapter 25: state that BASIC `COMPILE` writes ordinary flat IE64
     images and that flat-image loads are rejected whole when the
     image cannot fit at `PROG_START`.
  6. Chapter 35: add `COMPILE` to the BASIC File I/O verbs, document
     output placement and suffix behaviour, add `FILE_ERR_RANGE = 4`,
     and explain the range refusal for reads, writes, and listings.
  7. Appendices A, D, H, I, and L: update direct-mode command notes,
     File I/O error tables, IE64 image notes, error summaries, and
     lookup entries.
  8. Claim ledger: record the checked canonical sources and the
     reader-facing examples affected by this pass.
  9. Run the reader-facing scans and targeted tests for the changed
     source behaviour.
  10. Publish and print PDFs only after the source pass and checks are
      complete.
- Integrate the split BASIC native-compilation pipeline and File I/O
  read-cap changes from commit `b5a60840`. This is a focused BASIC,
  IE64 source, and File I/O pass. Do not add a new chapter. Do not
  expose host SDK assembler commands, build commands, repository paths,
  generator internals, private workspace addresses, or implementation
  scaffolding in reader-facing prose. The reader-facing idea is:
  BASIC can make native IE64 programs from inside the machine.

  Reader-facing changes from this commit:

  - `TRANSPILE "name"` is a direct-mode form. It runs the first half
    of `COMPILE`, writes the generated IE64 assembly as `name.asm`, and
    does not write `name.ie64`.
  - `ASSEMBLE "name"` is a direct-mode form. It reads `name.asm`,
    assembles it inside the machine at `PROGRAM_START`, and writes
    `name.ie64`. It is independent of the stored BASIC program.
  - `COMPILE` and `TRANSPILE` now emit self-contained IE64 assembly.
    Runtime support, the number-print helper, and bundled tokenised
    program data appear as labelled `dc.b` data when required, so
    `TRANSPILE "x"` followed by `ASSEMBLE "x"` produces the same flat
    image as `COMPILE "x"` for the same program.
  - The in-machine assembler accepts IE64 instructions, labels,
    PC-relative branches and calls, `dc.b` / `dc.w` / `dc.l` / `dc.q`,
    `align`, named constants from `ie64.inc`, and
    `include "ie64.inc"` as a no-op compatibility line. Other include
    files, `org`, `equ`, macros, conditionals, unknown mnemonics, and
    unresolved symbols are errors.
  - `FILE_READ_MAX` at `$F221C` is a one-shot File I/O read cap. A
    larger file is refused with `FILE_ERR_RANGE` before any byte is
    copied, and the cap is consumed by the next read.

  Execute this TRANSPILE / ASSEMBLE pass in this order:

  1. Check `sdk/examples/asm/ehbasic_ie64.asm`,
     `sdk/include/ehbasic_aot.inc`, `sdk/include/ie64.inc`,
     `sdk/include/aot_consttab.inc`, `file_io.go`,
     `file_io_constants.go`, `ehbasic_aot_test.go`, and
     `file_io_test.go` before writing claims.
  2. Chapter 1: add `TRANSPILE` and `ASSEMBLE` to the direct-mode
     editing/build command table and keep the wording short.
  3. Chapter 2: add `ASSEMBLE` and `TRANSPILE` entries, and list both
     as direct-mode commands. Keep `COMPILE`, `TRANSPILE`, and
     `ASSEMBLE` as prompt commands, not stored-program statements.
  4. Chapter 25: state that BASIC can assemble IE64 source from inside
     the machine, that `ASSEMBLE` starts at `PROGRAM_START`, and that
     it is the inverse path for self-contained `TRANSPILE` output. Keep
     host SDK assemblers out of the reader workflow.
  5. Chapter 35: update the opening and BASIC verb section for
     `TRANSPILE` and `ASSEMBLE`; document output placement, suffix
     behaviour, supported in-machine assembly subset, source-size/file
     errors, and the `FILE_READ_MAX` reason.
  6. Appendices A, D, H, I, and L: update prompt-only command notes,
     File I/O register summaries, symbol summaries, error summaries,
     and lookup entries.
  7. Claim ledger: record the checked canonical sources and the
     reader-facing examples affected by this pass.
  8. Run reader-facing scans and targeted tests for the changed source
     behaviour.
  9. Publish and print PDFs only after the source pass and checks are
     complete.
- Integrate the BASIC `TYPE` command from commit `e4ab4a08`. This is
  a focused BASIC and File I/O pass. Do not add a new chapter. Do not
  describe it as a host command, a shell command, or a modern operating
  system feature. The reader-facing idea is: BASIC can print text files
  from the Intuition Engine disk volume at the prompt.

  Reader-facing changes from this commit:

  - `TYPE "path"` is a direct-mode form. It reads a text file from the
    File I/O volume and prints it to the terminal.
  - The quoted path is required. Path separators are allowed, and the
    File I/O device still enforces the volume boundary.
  - `TYPE` uses the resident File I/O data buffer and writes
    `FILE_READ_MAX` before the read. A file that is too large is
    refused before any bytes are staged and prints `?FILE TOO LARGE`.
  - Files containing binary control bytes are refused with
    `?NOT A TEXT FILE`. Tab, line feed, carriage return, printable
    ASCII, and bytes `$80` through `$FF` are accepted as text.
  - Line endings are normalised for terminal output. A final line break
    is supplied when the file does not already end with one, so the
    prompt resumes on a fresh line.
  - `TYPE` is direct-only. Stored lines that try to compile it report
    `?COMPILE ERROR IN <line>: TYPE is direct-only`, while `TYPE=...`
    and `TYPE(...)` remain valid implied-LET variable and array forms.

  Execute this `TYPE` pass in this order:

  1. Check `sdk/examples/asm/ehbasic_ie64.asm`,
     `ehbasic_aot_test.go`, `file_io.go`, `file_io_constants.go`, and
     `sdk/docs/ehbasic_ie64.md` before writing claims.
  2. Chapter 1: add `TYPE` to the direct-mode editing/file command
     table and to the direct-only sentence.
  3. Chapter 2: add `TYPE` to the direct-mode command list and add an
     alphabetical `TYPE` entry.
  4. Chapter 24: include `TYPE` in the File I/O users list.
  5. Chapter 35: update the opening, read-cap note, direct-only
     compile-rejection wording, and the BASIC file-command section.
     Add a `TYPE` subsection covering syntax, text validation, read-cap
     errors, and newline output behaviour.
  6. Appendices A, D, I, and L: update prompt-only command notes, File
     I/O user summaries, error summaries, and lookup entries. Update
     Appendix H only if a symbol lookup would otherwise become stale.
  7. Claim ledger: record the checked canonical sources and the
     reader-facing examples affected by this pass.
  8. Run reader-facing scans and targeted tests for the changed source
     behaviour.
  9. Publish and print PDFs only after the source pass and checks are
     complete.
- Integrate the BASIC `64`-bit state and dynamic line-scratch changes
  from commit `3face0bd`. This is a focused consistency pass, not a
  new feature chapter. Do not expose private `EHBASIC_PRIV_*` names or
  runtime placement lore in reader-facing prose. The reader-facing
  ideas are:

  - BASIC's old fixed `$041000`-`$041FFF` line buffer is no longer a
    public or fixed memory-map fact. BASIC owns a dynamic line/input
    scratch reservation described by its state fields.
  - In the normal low32 fallback layout, the dynamic input/list scratch
    begins at `$01000000`, has the default capacity published by the
    BASIC state, and the internal programme/variable/file bridge arena
    begins after that scratch reservation.
  - Reader programs still use `MEMALLOC(size[,align])` for public
    low32 buffers shared with MMIO, copper, coprocessor, DMA, and file
    examples. They must not depend on private BASIC workspace
    addresses.
  - The in-machine IE64 assembler accepts `MOVT` and the zero-test
    source forms `BEQZ`, `BNEZ`, `BLTZ`, `BGEZ`, `BGTZ`, and `BLEZ`.
    The zero-test forms are assembler conveniences that encode the
    existing compare-and-branch operations against `R0`; do not present
    them as new architectural opcodes.

  Execute this BASIC state / assembler-form pass in this order:

  1. Check `sdk/include/ie64.inc`, `sdk/include/ehbasic_exec.inc`,
     `sdk/include/ehbasic_vars.inc`, `sdk/include/ehbasic_strings.inc`,
     `sdk/include/ehbasic_lineeditor.inc`,
     `sdk/include/ehbasic_file_io.inc`,
     `sdk/include/ehbasic_aot.inc`, `sdk/include/aot_consttab.inc`,
     `sdk/examples/asm/ehbasic_ie64.asm`, and the BASIC/AOT tests
     before writing claims.
  2. Chapter 2: refine the `ASSEMBLE` entry so the supported
     in-machine IE64 source forms include `MOVT` and the zero-test
     branch forms.
  3. Chapter 24: add a short BASIC private-layout note that points
     readers to `MEMALLOC` for public buffers and says line/input
     scratch is described by BASIC state, not by a fixed old address.
  4. Chapter 25: add the same assembler-source forms in the IE64
     source-made-inside-the-machine section, with the compare-against-
     `R0` explanation.
  5. Chapter 35: update the `ASSEMBLE` subsection with the same source
     form list and keep generated-source size wording non-specific.
  6. Appendix G: add the zero-test branch forms as IE64 assembler
     forms below the architectural branch group.
  7. Appendix J: remove the stale `$041000`-`$041FFF` BASIC line-buffer
     row and replace it with the current state page, runtime area, and
     low32 scratch/arena layout.
  8. Appendix L: add lookup entries for the zero-test branch forms and
     the BASIC line/input scratch note.
  9. Claim ledger: record the canonical sources checked and the
     reader-facing claims changed by this pass.
  10. Run stale-address and assembler-form scans, run the reader-facing
      dash scan, publish, and print PDFs only after the source tree is
      consistent.

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
