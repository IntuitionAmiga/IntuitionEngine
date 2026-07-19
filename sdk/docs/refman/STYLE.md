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

## Current M68K DBcc Correctness Pass

Chapter 29 must describe `DBcc` using the implemented MC68020
condition-first rule. If the selected condition is true, execution
continues without changing the counter. If the condition is false, the
processor decrements only the low word of `Dn`, preserves the upper
word, and branches while the decremented low word is not `$FFFF`.

Do not shorten this to "decrement and branch if condition". That
wording reverses the condition test and hides the word-sized counter.
Adversarially check the reader-facing wording against both M68K
interpreter paths and focused DBcc tests. Then update Chapter 29 and the
claim ledger, run the focused PRG check, publish strictly, and print the
PDFs only after the canonical source is correct.

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

## Current Coprocessor ABI Editorial Pass

This pass documents the final multi-instance coprocessor contract after
the implementation itself is complete and verified. The instance limits
are deliberately asymmetric:

- M68K, x86, and IE64 support worker instances `0` and `1`.
- IE32, 6502, and Z80 support worker instance `0` only.
- `COPROC_INSTANCE_LIMIT` and BASIC `COCAPS(cpuType)` are the canonical
  discovery paths. Do not describe the limits as a temporary JIT detail
  or imply that every CPU type has two workers.

The reader-facing contract includes the uniform twelve-slot mailbox,
the `$400` ring stride, the `(cpuTypeIndex * 2) + instance` ring rule,
the capability and selected-instance register block, the layout-version
handshake, instance-aware BASIC and IE Script forms, monitor labels, and
the final worker windows. A service-writing section must explain both
ring-header version bytes and the assigned-ring bootstrap convention
used by M68K, x86, and IE64 workers.

Execute this pass in ascending order over Chapters 2, 24, 32, 33, 34,
41, and 42, then Appendices A, D, H, I, J, and L in letter order. Check
all intervening published files for stale coprocessor addresses,
instance rules, errors, BASIC syntax, and cross-references even when no
edit is expected.

Chapter 42 remains a positive, IE-native 6502 example. Its entered
worker must acknowledge the mailbox layout version before polling the
ring, and its bytes, disassembly, allocation length, DATA statements,
mailbox addresses, expected result, and explanatory prose must agree.

Part VIII, Chapters 56 through 65, follows the checked porting tree and
must be revised after that tree has migrated to this ABI. The current
case-study architecture is:

- the main M68020+FPU owns game state, simulation, input, display-list
  production, and audio command production;
- M68K worker instance `0` owns Fast3D display-list translation, Voodoo
  submission, and frame pacing;
- M68K worker instance `1` owns sequence processing, envelopes, pitch,
  note allocation, and native IE voice control;
- IE64 worker instance `0` owns batched vertex transformation and
  lighting; and
- Voodoo owns triangle rasterisation, texturing, blending, and the
  framebuffer.

The self-contained pack carries the three service images used by those
workers. Each optional service has a checked local fallback. Explain the
division of labour and the shared-memory pipeline without exposing host
build commands, diagnostic environment switches, internal bring-up
traces, or obsolete ring layouts. The current Intuition Engine code owns
mailbox facts; the current `../mk64-ie` clients and services own the port's
division of labour. Older engineering prose is evidence only where it
still agrees with those implementations.

Execute the Part VIII migration in ascending order from Chapter 56
through Chapter 65, then replace the matching claim-ledger entries in the
same order. Preserve the fixed opening boundary and each chapter's
`The General IE Lesson` ending. This is an architecture case study, not a
host build guide or a source-file tour.

Author verification for this pass must use the code and include files
on disk as canonical truth, targeted coprocessor and BASIC tests, the
checked port's service and pack tests, the PRG example harness for
affected runnable chapters, strict publication, and PDF generation only
after the canonical source pass is complete.

Before this pass is complete, perform a final consistency repair in
ascending reader order:

- Chapter 32 must describe the version gate as a worker copying its
  assigned ring's layout byte at `+$03` to the acknowledgement byte at
  `+$04`. `COPROC_MAILBOX_VERSION` is the main-CPU discovery register,
  not the value a worker reads through its ring.
- Chapter 56 must keep quantities in its teaching prose consistent with
  the numbered rules that follow.
- Appendix E author provenance must name the implementation files that
  exist on disk. Provenance paths are evidence, not approximate module
  names.
- Appendix K must show a pool of worker instances rather than implying
  one worker per CPU type. It must state the asymmetric instance limits,
  the twelve reserved ring slots, the uniform `$400` stride, and the
  per-instance request and response path without turning the diagram
  into a duplicate register table.

Do not add the optional per-swap diagnostic hash controls to the guide.
They are host-enabled verification instrumentation rather than part of
the IE-native reader workflow. Internal worker JIT repairs likewise do
not require reader-facing prose unless they change a documented
programming contract.

Current controlled polish pass:

- Add the full game port case-study course as Part VIII, chapters `56`
  through `65`, and update `00-Preface.md` contents so the case study is
  visible to readers. Do not edit `sdk/docs/refman.publish/`, print PDFs,
  or commit during this draft pass unless explicitly instructed.

  The fixed opening boundary for Part VIII is:

  > This part studies an open-source decompilation-derived porting tree
  > and original Intuition Engine integration work as an IE systems case
  > study. It does not reproduce ROM assets, require the reader to build
  > the port, or teach commercial-game asset extraction.

  The Part VIII chapters are:

  1. Chapter 56, Why This Port Is Different.
  2. Chapter 57, Separating Game Code From Platform Code.
  3. Chapter 58, The IE Runtime Layer.
  4. Chapter 59, Fast3D To Voodoo.
  5. Chapter 60, Hardware TnL With The Coprocessor System.
  6. Chapter 61, Native IE Audio Instead Of RSP-Style Mixing.
  7. Chapter 62, Assets, ROM Data, And Build Hygiene.
  8. Chapter 63, Performance Work On A Real Port.
  9. Chapter 64, Input, Save Data, And Player-Facing Polish.
  10. Chapter 65, Lessons For Your Own IE Ports.

  Canonical sources to check before and while writing:

  - The checked `../mk64-ie` tree, especially `IE-PORT-NOTES.md`,
    `src/platform/platform.h`, `src/gfx/gfx_pc.c`,
    `src/gfx/gfx_fast3d.c`, `src/audio/audio_api.h`,
    `ie/ie_runtime.c`, `ie/game.ld`, `ie/loader_main.c`,
    `ie/ie_memory_layout.h`, `ie/ie_pack.h`, `ie/ie_mmio.h`,
    `ie/ie_platform_asset.c`, `ie/ie_platform_audio.c`,
    `ie/ie_platform_input.c`, `ie/ie_platform_log.c`,
    `ie/ie_platform_save.c`, `ie/ie_platform_time.c`,
    `ie/ie_gfx_voodoo.c`, `ie/ie_gfx_svc_client.c`,
    `ie/ie_audio_svc_client.c`, `ie/coproc/gfx_svc_main.c`,
    `ie/coproc/audio_svc_main.c`, `ie/coproc/coproc_layout.h`,
    `ie/coproc/ie_coproc.c`,
    `ie/coproc/ie_coproc.h`, `ie/coproc/tnl_proto.h`,
    `ie/coproc/tnl_service_ie64.asm`, and the matching tests.
  - Intuition Engine's PRG chapters for Voodoo, coprocessor calls,
    File I/O, input, audio, IE Mon, IE Script, performance counters,
    and memory mapping whenever the case study names a machine feature.

  Drafting rules:

  - Part VIII is an advanced IE systems case study, not a porting recipe
    for a commercial game and not an asset-extraction manual.
  - Do not reproduce ROM assets, copyrighted game data, long
    decompilation-derived listings, extraction steps, ROM file names,
    checksums, or host build commands.
  - The checked source tree is author evidence. Reader-facing text may
    name the design contracts and machine devices, but must not send the
    reader to host source paths, host SDK tools, or external toolchains
    as the normal path.
  - Treat code on disk as canonical. If a proposal says one CPU is used
    but the checked port starts a different coprocessor type, document
    the checked implementation and do not preserve the proposal's
    wording.
  - The chapter voice remains the PRG voice: idea first, exact machine
    contract second, then a compact diagram, table, or typed inspection
    pattern where useful.
  - Every Part VIII chapter ends with a section titled
    `The General IE Lesson`, which turns the case study back into advice
    for the reader's own IE software.
  - Add claim-ledger entries for each new chapter.

- Add the guided demo-programming course as Part VII, chapters `45`
  through `55`, and update `00-Preface.md` contents so the new course is
  visible to readers. This pass may also list chapters `40` through `44`
  in the contents because they are the workflow bridge into Part VII.
  Do not edit `sdk/docs/refman.publish/`, print PDFs, or commit during
  this draft pass unless explicitly instructed.

  The Part VII chapters are:

  1. Chapter 45, Your First Frame Loop.
  2. Chapter 46, The Rotozoomer In BASIC.
  3. Chapter 47, Driving The Hardware From IE Script.
  4. Chapter 48, From Floating Point To Tables.
  5. Chapter 49, The Rotozoomer In IE64 And IE32.
  6. Chapter 50, One Effect, Six CPUs.
  7. Chapter 51, Wobble, Texture Building, And Logo Motion.
  8. Chapter 52, Music-Synchronised Effects.
  9. Chapter 53, Copper, Raster Bands, And Layered Presentation.
  10. Chapter 54, Building A Complete Intro.
  11. Chapter 55, When BASIC Is Not Enough.

  Canonical sources to check before and while writing:

  - `sdk/examples/basic/rotozoomer_basic.bas`,
    `sdk/scripts/rotozoomer_ies.ies`,
    `sdk/examples/basic/wobble_zoom.bas`,
    `sdk/examples/basic/resonance.bas`,
    `sdk/examples/asm/rotozoomer_ie64.asm`,
    `sdk/examples/asm/rotozoomer.asm`,
    `sdk/examples/asm/rotozoomer_65.asm`,
    `sdk/examples/asm/rotozoomer_z80.asm`,
    `sdk/examples/asm/rotozoomer_68k.asm`,
    `sdk/examples/asm/rotozoomer_x86.asm`,
    `sdk/examples/asm/rotating_cube_copper_68k.asm`, and the matching
    demo tests for behaviour and file presence.
  - `video_chip.go`, `sdk/include/ehbasic_hw_video.inc`,
    `sdk/include/ehbasic_hw_system.inc`, `registers.go`, Chapter 4, and
    Appendix D for VideoChip, blitter, Mode 7, MEMCOPY, copper, VBlank,
    and status claims.
  - `midi_player.go`, `midi_engine.go`, `sdk/include/ehbasic_hw_audio.inc`,
    Chapter 21, and Chapter 23 for MIDI position, media playback, and
    music-timing claims.
  - `script_engine.go`, Chapter 34, and `script_rotozoomer_ies_test.go`
    for IE Script memory, file, CPU freeze, frame wait, and automation
    claims.
  - The per-CPU chapters and include files for CPU idioms used in the
    six-CPU comparison.

  Drafting rules:

  - Part VII is a guided demoscene course, not another register dump.
    Each chapter must teach a workflow step: frame loop, BASIC
    rotozoomer, IE Script lab bench, table conversion, native CPU ports,
    cross-CPU comparison, wobble texture work, music timelines, copper
    presentation, intro structure, and upgrade path.
  - Use curated excerpts from the shipped demos. Do not print full
    listings. Explain each excerpt before and after the code.
  - The shipped demo source files and their tests are author evidence.
    Reader-facing text must not present host build commands, host source
    paths, repository paths, or external assemblers as the programming
    path.
  - It is acceptable to name supplied demo files as disk-volume examples,
    but the normal teaching path remains BASIC, IE Mon, IE Script,
    in-machine assembly, MMIO, and shared bus reasoning.
  - If OS-hosted demo variants are mentioned, keep them as advanced
    companion material after the bare-machine path. Do not document guest
    operating-system internals, HostFS details, or host launch policy in
    the PRG.
  - Add claim-ledger entries for each new chapter.

- Draft the first-edition workflow-depth extension chapters as append-only
  source chapters `40` through `44`. Do not renumber existing chapters,
  update `00-Preface.md`, edit `sdk/docs/refman.publish/`, print PDFs, or
  commit during the draft pass unless explicitly instructed.

  The five draft chapters are:

  1. Chapter 40, Interrupts, Raster Timing, and Polling.
  2. Chapter 41, Building, Loading, and Laying Out Programmes.
  3. Chapter 42, Coprocessor Positive Cookbook.
  4. Chapter 43, Debugging and Profiling Cookbook.
  5. Chapter 44, A Larger Whole-Machine Example.

  Canonical sources to check before and while writing:

  - `bus_interrupt_sink.go`, `cpu_ie64_extirq_test.go`,
    `antic_constants.go`, `antic_dlist.go`, `video_chip.go`,
    `video_ula.go`, `video_ted.go`, `cpu_wait_mmio.go`, and the owning
    video tests for interrupt, VBlank, raster, blitter, and polling
    behaviour.
  - `program_executor.go`, `cpu_ie64.go`, `cpu_ie32.go`,
    `cpu_6502_runner.go`, `cpu_z80_runner.go`, `cpu_m68k.go`,
    `cpu_x86.go`, `sdk/include/*.inc`, and Chapters 24 through 35 for
    IE-native build, load, file type, symbol, and layout claims.
  - `coprocessor_constants.go`, `coprocessor_manager.go`,
    `coproc_worker_6502.go`, `sdk/include/ehbasic_hw_coproc.inc`,
    and coprocessor tests for the positive 6502 worker cookbook.
  - `debug_commands.go`, `debug_access.go`, `debug_ioview.go`,
    `script_engine.go`, `perf_accounting*.go`, Chapter 33, and
    Chapter 34 for debugging, watchpoint, access-log, I/O view,
    reverse-step, symbol, frame-hash, and `sys.perf_report()` claims.
  - The existing feature chapters for the larger whole-machine example:
    input Chapter 37, VBlank and timing Chapter 31, VideoChip Chapter 4,
    SoundChip Chapter 12, File I/O Chapter 35, and coprocessor Chapter 32.

  Drafting rules:

  - Keep the chapters practical and task-first. They are connective
    guide chapters, not new chip reference dumps.
  - Every example must be IE-native and typed from BASIC, IE Mon, or
    IE Script. Do not send the reader to host SDK assemblers, source
    paths, build commands, test commands, or external toolchains.
  - Where a long positive example depends on a compact service image,
    enter the service as bytes in BASIC or IE Mon and explain what the
    bytes do. Do not pretend there is a hidden prebuilt worker unless
    the reader has just created it inside IE.
  - Show when polling is the correct path. Do not imply that every
    device is interrupt-driven.
  - Use the same house voice as the existing chapters: idea first,
    then exact register truth, then typed example, expected result,
    line notes, side effects, limits, and what comes next.
  - Add claim-ledger entries for each new chapter. Record the exact
    canonical files checked and whether the chapter is a draft that has
    not yet been inserted into the published contents.

- Integrate the reader-facing machine-contract changes after committed
  PRG refresh `5feeb8e5`. Check every commit from `09c4821e` through
  `15489ba0` before editing. This is a narrow public-contract pass:
  document new MMIO command values, status/error text, memory-discovery
  registers, and visible BASIC prompt behaviour. Do not document JIT
  range invalidation, JIT region policy, compositor copy leases,
  graphics backend details, host launch defaults, guest operating
  systems, or test-only rationale in the PRG.

  Canonical sources to check before writing:

  - `sdk/examples/asm/ehbasic_ie64.asm`, `sdk/include/ehbasic_compiler_driver.inc`,
    `sdk/include/ehbasic_file_io.inc`, and AOT/File I/O tests for the
    startup memory banner, empty-programme compile rejection, and
    generated-image command behaviour.
  - `registers.go`, `sysinfo_mmio.go`, `sdk/include/ie64.inc`, and
    SysInfo tests for `SYSINFO_LOW_WINDOW_LO` and
    `SYSINFO_LOW_WINDOW_HI`.
  - `coprocessor_constants.go`, `coprocessor_manager.go`, and
    `coprocessor_startmem_test.go` for `COPROC_CMD_START_MEM = 6`.
  - `voodoo_constants.go`, `video_voodoo.go`, and
    `video_voodoo_command_stream_test.go` for `VOODOO_CMD_PTR`,
    `VOODOO_CMD_COUNT`, `VOODOO_CMD_SUBMIT`, selectable big-endian or
    little-endian guest-RAM address/value pairs, replay values `1` and
    `2`, and the `65536` write limit.
  - `cpu_m68k.go` for the M68K flat-image raw deposit rule: loading
    skips MMIO apertures and the stack guard hole instead of causing
    device side effects.
  - `video_chip.go` and blitter tests for the existing blitter-start
    contract. Record that this remains a correctness clarification, not
    a new reader feature.

  Execute this pass in book order:

  1. Chapter 1: explain the startup memory banner without hard-coding
     one machine's RAM values.
  2. Chapter 2 and Chapter 35: add `?NO CODE TO COMPILE` for
     `RUN AOT`, `COMPILE`, and `TRANSPILE` on an empty stored
     programme.
  3. Chapter 9 and Appendix D/L: document the Voodoo command-stream
     MMIO registers at programmer level only.
  4. Chapter 24 and Appendices D/H/L: add the SysInfo low-window
     register pair and explain how it differs from total and active RAM.
  5. Chapter 29 or Appendix H: add a short M68K flat-image loader note
     saying load deposits do not fire MMIO side effects and skip the
     stack guard hole.
  6. Chapter 32 and Appendices D/I/L: add `COPROC_CMD_START_MEM = 6`
     and its `COPROC_REQ_PTR` / `COPROC_REQ_LEN` inputs.
  7. Claim ledger: record the checked canonical sources and mark
     blitter-shadow hydration as preserving the existing documented
     blitter start contract.
  8. Run scans for forbidden non-PRG topics, dash punctuation, stale
     command/register wording, and changed public constants. Publish and
     print PDFs only after the source files pass.
- Add RawlandMini MIDI lookup material without turning the PRG into an
  internal synth-source dump. Check `midi_constants.go`,
  `midi_engine.go`, `midi_parser.go`, `midi_live.go`, and focused MIDI
  tests before writing. The reader-facing contract is that
  RawlandMini is the fixed IE-native patch table for MIDI/MUS and live
  MIDI, with `128` melodic programme numbers, channel `9` percussion,
  drum notes `35`-`81`, a `10`-voice pool, and no reader-facing
  replacement patch-table loader in this register block.

  Execute this pass in book order:

  1. Chapter 21: keep the RawlandMini section short, add a compact
     family/range table, and point readers to Appendix E for the full
     programme and drum-number lookup. Do not list internal waveform,
     envelope, volume, priority, or voice-stealing implementation
     details beyond the existing public limits.
  2. Appendix E: add a dense RawlandMini programme-number table and a
     drum-note table. Label them as GM-style selection numbers used by
     RawlandMini, not as a promise of sampled General MIDI instrument
     identity.
  3. Appendix L: add lookup entries for the RawlandMini programme and
     drum tables.
  4. Claim ledger: record the checked canonical sources and the changed
     RawlandMini lookup claim.
  5. Run scans for forbidden non-PRG topics, dash punctuation, and stale
     replacement-patch-loader wording. Publish and print PDFs only after
     the source files pass.
- Integrate the PRG-facing coprocessor-status precision from commit
  `27ca532c570a8364e923f8f710c45eee589f52f1`. Treat this as a narrow
  source-backed wording pass, not as a performance-architecture tour.
  Check `coprocessor_manager.go`, `coprocessor_constants.go`,
  `machine_bus_phys.go`, Chapter 32, Appendix D, and the claim ledger
  before editing. Do not document JIT region policy, deopt accounting,
  ExecMem arenas, performance profiles, video frame leases, audio block
  rendering internals, benchmark workflow, or environment-variable
  tuning in the PRG unless they become a stable reader-facing machine
  contract.

  The reader-facing effect is limited to the coprocessor monitor
  registers:

  - `COPROC_BUSY_PCT` reports the busy percentage over the rolling
    coprocessor accounting window. In source this is ten `100` ms
    buckets, which is about one second.
  - `COPROC_STATS_RESET` clears operation and byte counters and restarts
    the busy-percentage accounting window when written with `1`.
  - Completion-wake and physical-write notification changes improve
    responsiveness of the existing ticket and IRQ contract; they do not
    introduce a new BASIC or MMIO programming route.

  Execute this branch pass in this order:

  1. Update this plan entry before any reader-facing chapter edits.
  2. Chapter 32: tighten the `COPROC_BUSY_PCT` table entry and add a
     short note after the register table about `COPROC_STATS_RESET`.
  3. Appendix D: mirror the same extended-monitor wording.
  4. Claim ledger: record the checked canonical sources and the changed
     coprocessor claim.
  5. Run stale-term and exclusion scans, then publish and print PDFs.
- Integrate the public PRG-facing changes from the `voodoo-opti`
  branch after the last broad docs refresh. Treat these as a focused
  source-backed consistency pass, not as a new feature tour. Check
  `sfx_constants.go`, `sfx_trigger.go`, `registers.go`,
  `debug_ioview.go`, `sdk/include/ie64.inc`,
  `sdk/include/ie65.inc`, `sdk/include/ie80.inc`,
  `video_voodoo.go`, and `voodoo_constants.go` before writing claims.
  The reader-facing effects are:

  - SFX now has a 32-channel extended trigger window at
    `$F2600`-`$F29FF`; the old `$F0E80`-`$F0EFF` window remains as
    legacy aliases for channels `0`-`3`.
  - `SFX_VOL` is a 16-bit field, but the sample mixer clamps playback
    volume to `0`-`255`; do not describe `65535` as a louder reader
    setting.
  - 6502 and Z80 include files expose `TERM_IO_BANK`,
    `SET_TERMINAL_BANK`, and `$2700`-`$27FF` terminal/input/RTC aliases.
    Chapters 24, 27, and 28 already carry the teaching text; Appendix H
    must carry the lookup terms as well.
  - Voodoo triangle submission now flushes a full `4096`-triangle batch
    as a render-only mid-frame job and then accepts more triangles. Do
    not say further `TRIANGLE_CMD` writes are ignored while the batch is
    full. `SWAP_BUFFER_CMD` hands the frame to the rasteriser and
    returns while that work may still be in progress. Current source
    allows two Voodoo swap jobs in flight; a later swap waits only when
    that pipeline is full.
  - Voodoo `FBI_BUSY` and `SST_BUSY` mean render/swap work is in
    progress, `SWAPBUF` means a publish swap is pending, `MEMFIFO` is a
    coarse ready field for the current batch, and `PCIFIFO` is the
    high-level command-space field.

  Execute this branch pass in this order:

  1. Update this plan entry before any reader-facing chapter edits.
  2. Chapters 11, 12, 23, and 24: update SFX channel counts, ranges,
     legacy aliases, extended window, 8-bit bank-window access, volume
     range wording, and typed examples only where needed.
  3. Chapter 33: clarify that `io sfx` shows the 32 extended trigger
     channels.
  4. Chapter 9 and Appendix D: update Voodoo batch, overflow, swap,
     two-job pipeline, and status wording against `video_voodoo.go` and
     `voodoo_constants.go`.
  5. Appendices D, H, J, K, and L: update SFX ranges, terminal aliases,
     Voodoo lookup terms, and the audio diagram.
  6. Claim ledger: record the checked canonical sources and the changed
     claims.
  7. Run targeted stale-term scans for SFX four-channel wording, stale
     Voodoo ignored-batch wording, and missing terminal aliases. Publish
     and print PDFs only after those checks pass.
- Integrate the current `3dfx` branch reader-facing Voodoo and IE
  Script changes as a narrow PRG pass. This pass is about the machine
  contract visible from BASIC, documented MMIO, IE Mon, and IE Script.
  Do not mention graphics backend selection, host render APIs,
  descriptor pools, staging buffers, cache hashes, command-buffer
  internals, JIT internals, diagnostic environment variables, guest
  operating systems, packaging scripts, or benchmark workflow in the
  PRG.

  Canonical sources to check before writing:

  - `video_voodoo.go`, `voodoo_constants.go`, `voodoo_software.go`,
    `voodoo_vulkan.go`, `voodoo_shaders.go`, and Voodoo tests for
    triangle submission, texture/state snapshots, full-batch overflow
    flushes, no-clear continuation, swap pipeline depth, and status
    bits.
  - `script_engine.go` and `perf_accounting_subsys.go` for
    `sys.perf_report()` and `sys.perf_reset()`.

  Reader-facing claims to preserve:

  - `TRIANGLE_CMD` queues a triangle and latches the current vertices,
    per-vertex attributes, raster state, and currently uploaded texture.
    Later mode, texture, clip, fog, chroma, stipple, slope, or upload
    writes affect later triangles only.
  - A full `4096`-triangle batch is rendered into the current drawing
    buffer as a render-only flush, without publishing the frame. Later
    triangles continue the same frame until `SWAP_BUFFER_CMD`.
  - `SWAP_BUFFER_CMD` hands the current batch to the rasteriser,
    returns while render or publish work may still be active, and may
    run with up to two Voodoo swap jobs in flight. A later swap waits
    only when that pipeline is full.
  - `VOODOO_STATUS` reports `FBI_BUSY` and `SST_BUSY` while any render
    or publish job is active; `SWAPBUF` reports a pending publish swap;
    `MEMFIFO` is a coarse current-batch ready field; `PCIFIFO` is the
    high-level command-space field.
  - `sys.perf_report()` returns a subsystem performance report string
    and `sys.perf_reset()` clears the subsystem counters. The report is
    empty when performance accounting is disabled or when no
    instrumented path ran.

  Execute this pass in book order:

  1. Chapter 9: tighten Voodoo submission, texture upload, full-batch
     overflow, no-clear continuation, swap pipeline, status, and limits
     wording. Keep the prose at programmer level.
  2. Chapter 34: add `sys.perf_report()` and `sys.perf_reset()` to the
     system module and explain the report shape briefly.
  3. Appendix D: mirror the Voodoo MMIO side effects and add the script
     performance helpers only if a relevant lookup row already exists.
  4. Appendix L: add lookup entries for the new script performance
     helpers and Voodoo swap/status terms.
  5. Claim ledger: record the checked canonical sources and the
     reader-facing contracts changed.
  6. Run targeted stale-term, forbidden-term, dash, and publish
     consistency scans, then publish and print PDFs.
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
     `sdk/include/ehbasic_compiler_driver.inc`,
     `sdk/include/ehbasic_file_io.inc`,
     `sdk/include/ehbasic_lineeditor.inc`, `sdk/include/ie64.inc`,
     `file_io.go`, `file_io_constants.go`, `cpu_ie64.go`,
     `program_executor.go`, `ehbasic_aot_test.go`,
     `file_io_test.go`,
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
     `sdk/include/ehbasic_compiler_driver.inc`, `sdk/include/ie64.inc`,
     `sdk/include/ehbasic_assembler_consttab.inc`, `file_io.go`,
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
     `sdk/include/ehbasic_compiler_driver.inc`, `sdk/include/ehbasic_assembler_consttab.inc`,
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

- Integrate the BASIC AOT, File I/O, and blitter contract changes from
  commit `7daace29` as a narrow correctness pass. Check
  `file_io.go`, `file_io_constants.go`, `main.go`, `video_chip.go`,
  `sdk/include/ehbasic_compiler_driver.inc`, `sdk/include/ie64.inc`,
  `ehbasic_aot_test.go`, `file_io_test.go`, and
  `video_blitter_test.go` before changing reader-facing claims.
  Do not add a new chapter, do not reference the Voodoo mega demo as a
  required reader example, and do not mention the resonance demo.

  Reader-facing claims to preserve:

  - `FILE_DATA_PTR64` at `$F22B0` is an IE64-only `64`-bit
    data-buffer pointer for File I/O reads and writes. It extends the
    legacy block; it does not replace `FILE_DATA_PTR`.
  - The legacy `FILE_DATA_PTR` ABI remains the cross-CPU `32`-bit
    path. Legacy staged transfers must stay below the sign-extended
    alias guard at `$FFFF0000` and inside active guest RAM.
  - `FILE_DATA_PTR64` may deliberately name high backed guest RAM for
    read or write data buffers, but a `64`-bit span must not wrap, run
    beyond active backing, or start low and cross into the
    sign-extended alias guard.
  - Directory listing remains a low staged File I/O operation unless
    the implementation is changed to route it through the `64`-bit
    pointer path.
  - Blitter operations that would write outside backed writable guest
    memory set `BLT_STATUS.ERR`. They must not be described as
    wrapping, clipping silently, or falling back to another operation.
  - The BASIC AOT section may say that the supported native integer
    subset is broader and uses active guest RAM for generated buffers,
    but it must not expose private AOT workspace addresses as the
    reader programming interface.

  Execute this pass in this order:

  1. Chapter 35: tighten the `FILE_DATA_PTR` and `FILE_DATA_PTR64`
     read/write/list range wording and keep the typed example on the
     legacy path.
  2. Chapter 4: add the short blitter destination-range error rule to
     the existing `BLT_STATUS` section.
  3. Appendix D: correct `FILE_DATA_PTR64` from write-source pointer
     to read/write data-buffer pointer.
  4. Appendix H and Appendix L: make sure `FILE_DATA_PTR64` is
     discoverable as the IE64 File I/O extension.
  5. Chapter 25 and Chapter 2: review the existing AOT wording against
     the new lowering tests, changing only stale or over-specific
     wording.
  6. Claim ledger: record the canonical sources checked and the
     reader-facing contract updated by this pass.
  7. Run reader-facing scans, publish strictly, and print PDFs after
     the source tree is consistent.

- Integrate the generic live-MIDI MMIO port from commit `e1aaa9b5` as
  a focused MIDI chapter and lookup pass. This is not a new chapter and
  does not change the file-backed MIDI/MUS player ABI. Check
  `midi_constants.go`, `midi_live.go`, `midi_engine.go`,
  `runtime_status.go`, `debug_ioview.go`,
  `sdk/include/ehbasic_hw_audio.inc`,
  `sdk/include/ehbasic_tokens.inc`,
  `sdk/include/ehbasic_tokenizer.inc`,
  `sdk/include/ehbasic_exec.inc`, the six CPU include files, and the
  live MIDI, BASIC, mixer, and monitor I/O view tests before changing
  reader-facing claims. Do not import source comments about DOS,
  MPU-401, or any game-specific MUS origin into the book.

  Reader-facing claims to preserve:

  - `IE_MIDI_LIVE_DATA` at `$F0BF4` is a byte-wide write port for raw
    MIDI channel-voice bytes. Reading it returns `0`.
  - `IE_MIDI_LIVE_STATUS` at `$F0BF5` is byte-wide read status. Bit
    `0` means the live port is active.
  - `IE_MIDI_LIVE_CTRL` at `$F0BF6` is byte-wide control. Writing bit
    `0` resets the live port and turns off live notes.
  - BASIC `MIDI NOTE`, `MIDI PROG`, `MIDI CTRL`, `MIDI SEND`, and
    `MIDI RESET` drive the live port from inside IE64 BASIC.
  - The live port accepts running-status channel-voice streams and
    drives the same RawlandMini synth engine and `10`-voice pool as
    file-backed MIDI/MUS playback.
  - Live voices have priority over file-player voices when the shared
    pool must steal a voice, but the two paths remain separate control
    surfaces.
  - IE Mon exposes a `midilive` I/O register view alongside
    `midiplay`.

  Execute this pass in this order:

  1. Chapter 2: add the `MIDI` BASIC vocabulary entry with the five
     live subverbs and a pointer to Chapter 21.
  2. Chapter 11: update the engine comparison and BASIC/direct access
     map so MIDI covers both file playback and live note events.
  3. Chapter 21: retitle the chapter to include Live MIDI, add a
     typed live-MIDI BASIC example, document the live register block,
     running-status stream behaviour, reset/status bits, setup order,
     voice-sharing rule, and limits.
  4. Chapter 23: add live MIDI to the BASIC verbs table, full-address
     CPU map, 6502 mirror map, and Z80 memory-mirror note.
  5. Chapter 33: add the `midilive` IE Mon I/O view and a short
     transcript.
  6. Chapter 34: note that `dbg.io("midilive")` inspects the live
     port, while `audio.write_reg` can write the byte data/control
     registers if a script deliberately drives MMIO.
  7. Appendices A, D, H, J, K, and L: add the `EXT_MIDI` token, live
     MIDI register rows, per-CPU symbol lookup, memory-map row, mixer
     diagram branch, and index terms.
  8. Claim ledger: record the canonical sources checked and the
     reader-facing examples changed by this pass.
  9. Run stale-term, dash, forbidden-term, and publish consistency
     scans, publish strictly, and print PDFs only after the source
     tree is consistent.

- Execute the post-graphics, live-MIDI, and M68K FPU PRG pass for the
  current codebase. This pass is limited to reader-facing machine
  contracts. Do not mention compatibility shims, guest operating
  systems, guest filesystem bridges, packaging scripts, diagnostic
  scripts, or other material that is not part of programming Intuition
  Engine from BASIC, IE Mon, IE Script, or documented MMIO.

  Canonical sources to check before writing:

  - `video_chip.go` and `video_blitter_test.go` for `BLT_FLAGS` bits
    `11` and `12`, mask source offset and row stride, alpha-template
    source stride, and Mode 7 CLUT8 behaviour.
  - `midi_live.go`, `midi_constants.go`, `debug_ioview.go`, and live
    MIDI tests for the live MIDI byte registers and their no-shadow
    read/write contract.
  - `cpu_m68k.go` and `fpu_integration_test.go` for 68881-style FPU
    decode, precision-qualified opmodes, immediate operands,
    `FMOVEM`, `FSAVE`, and `FRESTORE`.

  Execute this pass in book order:

  1. Chapter 4: make `ALPHA_COPY` wording match the current
     alpha-template flag, document MSB-first masked copy sampling,
     `BLT_MASK_SRCX`, `BLT_MASK_MOD`, alpha-template source format,
     and CLUT8 Mode 7 limits with typed examples or concise
     register-level examples where the chapter already has a reader
     path.
  2. Chapter 21: add the live MIDI no-shadow rule. The reader should
     inspect live state through `IE_MIDI_LIVE_STATUS` or `io midilive`,
     not by expecting `IE_MIDI_LIVE_DATA` or `IE_MIDI_LIVE_CTRL` writes
     to appear as RAM bytes.
  3. Chapter 29: update the M68K floating-point section for the current
     68881-style support: immediate sources, precision-qualified
     single/double result forms, `FMOVEM`, control-register moves,
     `FSAVE`, `FRESTORE`, and the existing packed-decimal boundary.
  4. Appendices D, G, H, and L: update only the corresponding lookup
     rows and index terms.
  5. Claim ledger: record the exact source files checked and the
     reader-facing contracts changed.
  6. Run forbidden-term, dash, and targeted consistency scans, publish
     the stripped tree, and print PDFs only after the source tree is
     consistent.

- Execute the post-`75f2d7cd` Voodoo state-binding and M68K FPU
  operand pass for the current codebase. This is a focused
  machine-contract update, not a backend, acceleration, guest system,
  packaging, or diagnostic pass. Do not mention hardware compositor
  acceleration, graphics backend selection, JIT internals, guest
  appliances, guest filesystem bridges, probe scripts, demo names, or
  other material that is not part of programming Intuition Engine from
  BASIC, IE Mon, IE Script, or documented MMIO.

  Canonical sources to check before writing:

  - `video_voodoo.go`, `voodoo_software.go`, `voodoo_vulkan.go`,
    `voodoo_constants.go`, `sdk/docs/ie_voodoo_abi.md`, and
    `video_voodoo_state_batch_test.go` for `TRIANGLE_CMD` queuing,
    state binding, current-texture binding, swap-time rasterisation,
    and status effects.
  - `cpu_m68k.go`, `fpu_integration_test.go`, and M68K FPU tests for
    68881-style data-register direct operands, legal operand formats,
    `FPn` to `Dn` stores, and the existing packed-decimal boundary.

  Execute this pass in book order:

  1. Chapter 9: state that `TRIANGLE_CMD` latches raster state and the
     currently uploaded texture at submission time. Later mode,
     texture, clip, fog, chroma, stipple, or slope writes affect later
     triangles only. Preserve the existing queue, busy-bit, and swap
     wording.
  2. Chapter 29: add the FPU data-register direct operand rule without
     expanding the chapter into an FPU tutorial.
  3. Appendices D, G, and L: add only compact lookup notes and index
     routes for the state-binding and FPU operand rules. Do not add
     new register rows where no address changed.
  4. Claim ledger: record the exact source files checked and the
     reader-facing contracts changed.
  5. Run forbidden-term, dash, and targeted consistency scans, publish
     the stripped tree, and print PDFs only after the source tree is
     consistent.

- Execute the source-backed PRG consistency pass from the July 2026
  full-manual review. This pass is limited to reader-facing corrections
  found by adversarial comparison with code on disk. Do not add
  unrelated branch notes, guest-system details, backend details, host
  build paths, repository paths, or non-PRG material.

  Canonical sources to check before writing:

  - `video_antic.go`, `antic_constants.go`, and Chapter 7 for
    `ANTIC_NMIST` latch acknowledgement semantics.
  - `registers.go`, `terminal_io.go`, and Chapters 37 and 44 for the
    terminal raw-key register block.
  - `../mk64-ie/ie/coproc/ie_coproc.c`,
    `../mk64-ie/ie/coproc/ie_coproc.h`,
    `../mk64-ie/ie/coproc/tnl_proto.h`,
    `../mk64-ie/ie/coproc/tnl_service_ie64.asm`, and Chapter 60 for
    the checked transform-and-lighting coprocessor boundary.
  - Chapters 40 through 65 and Appendix L for index coverage after the
    workflow, demoscene, and case-study chapters are added.

  Execute this pass in book order:

  1. Chapter 37: describe `TERM_KEY_IN` and `TERM_KEY_STATUS` as the
     raw key queue used by the terminal MMIO path.
  2. Chapter 40: state that writing any value to `ANTIC_NMIST` clears
     the pending DLI/VBI latches. Do not describe the write value as a
     selective bit mask.
  3. Chapter 60: keep the coprocessor lesson tied to the checked IE64
     worker. If the chapter uses the phrase "hardware TnL", clarify
     that this means another IE bus CPU doing the work, not a
     fixed-function Voodoo TnL unit.
  4. Appendix L: add practical lookup entries for Chapters 40 through
     65, including interrupts, frame loops, rotozoomers, IE Script
     lab work, Fast3D, TnL, pack layout, save data, profiling, Voodoo
     case-study work, and the game-port case study.
  5. Claim ledger: record the exact source files checked and remove
     stale draft-only wording once the chapters are listed and
     published.
  6. Run forbidden-topic scans, dash scans, source/publish consistency
     checks, then publish and print PDFs.

- Execute the post-`771fad88` Voodoo texture-slot pass. This pass is
  limited to the guest-visible texture residency contract and its
  discoverable SysInfo feature bit. Do not mention browser execution,
  host SIMD, frame publication internals, backend caches, diagnostic
  environment variables, websites, deployment, or other material that
  is not part of programming Intuition Engine.

  Canonical sources to check before writing:

  - `voodoo_constants.go` for `VOODOO_TEX_SLOT` at `$F8350` and
    `VOODOO_TEX_BIND` at `$F8354`.
  - `video_voodoo.go` for slot selection, the `0` through `65535`
    identifier range, `$FFFFFFFF` no-slot selection, upload-time
    immutable texture retention, bind behaviour, empty-slot behaviour,
    and triangle state binding.
  - `registers.go` and `sysinfo_mmio.go` for
    `SYSINFO_FEATURE_VOODOO_TEX_SLOTS` at bit `3` of
    `SYSINFO_FEATURES`.
  - `../mk64-ie/ie/ie_gfx_voodoo.c` and
    `../mk64-ie/ie/ie_mmio.h` only for the Part VIII case-study use of
    feature detection, generation-aware upload, and bind-by-identifier.
  - `sdk/include/ie64.inc`, `sdk/include/ie32.inc`,
    `sdk/include/ie65.inc`, `sdk/include/ie80.inc`,
    `sdk/include/ie68.inc`, and `sdk/include/ie86.inc` for the public
    assembly symbols. Add the feature bit, both register symbols, and
    the no-slot value in each assembler's native notation. Preserve the
    6502 include's bank-page convention and provide explicit low-byte
    offsets for the two registers.

  Execute this pass in book order after the shared include symbols are
  verified:

  1. Chapter 9: add the two texture-slot registers, feature detection,
     setup order, lifetime and state-binding rules, invalid and empty
     slot behaviour, limits, and an explained IE-native BASIC example
     that stores and rebinds visible textures without a second upload.
  2. Chapter 24: add `SYSINFO_FEATURES` to the system-information table
     and define bits `0` through `3`; correct the prose count so it
     matches the seven implemented read-only words.
  3. Chapter 59: describe the case-study's generation-aware first
     upload, later bind, and feature-bit fallback without introducing a
     repository or host-tool reader path.
  4. Chapter 63: describe texture-slot residency as a measured traffic
     reduction, while retaining retransmission as the fallback.
  5. Appendices D, H, and L: add the compact SysInfo, register, symbol,
     and lookup entries. Appendix J needs no new row because the Voodoo
     register range is unchanged.
  6. Claim ledger: record the canonical implementation files, the
     include-symbol verification, the reader workflow, and the exact
     example used.
  7. Run include consistency checks, forbidden-topic and dash scans,
     targeted PRG checks, strict publication, and PDF generation only
     after the canonical source tree is consistent.

- Execute the post-`b5b368fd` IE Mon media-freeze pass. This pass is
  limited to the reader-visible monitor and IE Script debugging
  contract. Do not mention recording implementation, media pumps,
  encoder settings, sample-ring concurrency, diagnostic memory
  overrides, websites, host delivery, or other material that is not
  part of programming and debugging Intuition Engine.

  Canonical sources to check before writing:

  - `debug_monitor.go` for normal activation, breakpoint entry,
    deactivation, overlay exit, pre-entry audio-state restoration, and
    explicit in-session audio-command persistence.
  - `debug_commands.go` for `fa` and `ta` marking an explicit session
    choice.
  - `debug_overlay.go` for the shared Escape and command-exit path.
  - `script_engine.go` for `dbg.open()` and `dbg.freeze()` activation,
    nested open/close behaviour, and the `dbg.freeze_audio()` and
    `dbg.thaw_audio()` command paths.
  - `debug_monitor_media_freeze_test.go` for the executable contract
    across activation, breakpoint entry, restoration, explicit `fa` or
    `ta`, and overlay exit.

  Execute this pass in book order:

  1. Chapter 33: state that entering IE Mon freezes all guest CPUs and
     the audio clock, holding player positions and silencing output.
     Correct the byte-entry audio workflow so a breakpoint re-entry
     requires `ta` before the programmed tone can be heard. Explain
     pre-entry restoration and the persistence of an explicit `fa` or
     `ta` issued during the session. Keep `freeze *` described as a CPU
     command, not as another whole-machine monitor entry.
  2. Chapter 34: state that the first `dbg.open()` or its `dbg.freeze()`
     alias activates the same monitor freeze. Nested opens do not create
     additional machine transitions; the final matching close restores
     the pre-entry audio state. `dbg.freeze_audio()` and
     `dbg.thaw_audio()` alter the gate during the session but do not
     override that final restoration. Only monitor `fa` or `ta`, including
     `dbg.command("fa")` or `dbg.command("ta")`, establishes an audio
     state that survives monitor exit.
  3. Chapter 43: add a compact task-first recipe for listening to sound
     hardware while CPUs remain stopped, using `ta` to run the audio
     clock and `fa` to hold it again.
  4. Appendix L: add lookup routes for monitor audio freeze, `fa`, `ta`,
     `dbg.open`, `dbg.freeze_audio`, and `dbg.thaw_audio`.
  5. Claim ledger: record the canonical files, reader workflow, expected
     state transitions, and targeted tests. Supersede the direct edit to
     `sdk/docs/refman.publish/33-iemon.md` by editing the canonical
     source and publishing normally.
  6. Run targeted monitor tests, chapter scans, forbidden-topic and dash
     scans, strict publication, source/publish comparison, and PDF
     generation only after the canonical source tree is consistent.

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

## Replacement IE64 BASIC Compiler Pass

Integrate the completed replacement IE64 BASIC compiler from commits
`994274ed` through `449cff72`. This is a focused BASIC native-compilation
pass. Do not add a compiler-internals chapter and do not expose compiler IR,
helper masks, optimisation passes, JIT controls, host assemblers, repository
paths, tests, or build commands in reader-facing prose. The reader-facing
idea remains: BASIC can make native IE64 programs from inside the machine.

Adversarially check every claim against the compiler `.inc` files, the
compiler target-classification inventory, and the focused compiler tests.
Older prose about an integer-only subset, a resident runtime blob, or
interpreter delegation is not canonical.

Reader-facing facts from the completed compiler replacement:

- `RUN AOT` compiles the stored program as typed native IE64 code and runs it
  from the retained arena. Its compiled language includes integer and
  floating-point numeric values, strings, arrays, structured control flow,
  file operations, and supported hardware statements.
- `COMPILE` and `TRANSPILE` target a standalone flat IE64 image. Generated
  source contains every typed helper required by that program. Generated
  statements and expressions do not delegate to the resident BASIC
  interpreter.
- Stored `LOAD` is an arena-only operation. It may be compiled by `RUN AOT`,
  where it replaces the stored program, but it is rejected for standalone
  `COMPILE` and `TRANSPILE` output.
- Prompt commands such as `RUN`, `NEW`, and `CONT` remain direct-only and are
  not compiled as stored-program statements.
- Reader examples must keep `RUN AOT`, `COMPILE`, `TRANSPILE`, and `ASSEMBLE`
  at the prompt. Do not place direct-mode build commands inside a numbered
  BASIC listing.

Execute this pass in ascending reader order:

1. Chapter 1: check the introductory `RUN AOT` wording and clarify the typed
   native result only where needed.
2. Chapter 2: keep the compile commands direct-only, remove the host assembler
   from the reader workflow, and distinguish retained-arena from standalone
   targets.
3. Chapter 25: replace the obsolete integer-subset and hot-path discussion
   with the reader-visible typed compiler model and its target distinction.
4. Chapter 35: remove resident-interpreter fallback wording, document the
   self-contained typed helper closure, and state the standalone `LOAD`
   limitation precisely.
5. Chapter 41: make the native-image example a real prompt transcript rather
   than a numbered listing containing direct-mode commands.
6. Appendix I: record the target-specific stored `LOAD` compile error.
7. Appendix L: add lookup entries for compiled BASIC, standalone BASIC images,
   and stored `LOAD` under `RUN AOT`.
8. Claim ledger: record canonical sources, target classifications, reader
   workflow, runnable examples, expected results, and author verification.
9. Run focused compiler tests, the PRG example harness for affected chapters,
   forbidden-term and dash scans, strict publication, and PDF generation in
   that order. PDFs are always the final generated artefact.

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
