# SDK Document Audit Ledger

This ledger records technical audits for developer-facing SDK documents.
Its primary shipping target is the end-user PDF companion set for the
Programmer's Reference Guide.

## Control Plan

This section is the stable control plan for the five shipped SDK
companion manuals. Chronological audit entries below are evidence, not
authority. Later evidence entries may supersede earlier evidence when the
manual scope or source-backed contract changes.

### Locked Decisions

The five shipped manuals are:

- `sdk/docs/IE64_ISA.md`
- `sdk/docs/IE32_ISA.md`
- `sdk/docs/iemon.md`
- `sdk/docs/iescript.md`
- `sdk/docs/architecture.md`

These five manuals are the public SDK companion set for experienced
software engineers. They are maintained as user-facing reference manuals,
not as agent instructions, audit transcripts, plans, or implementation
scratch notes.

The executable source code on disk is canonical at all times. Data
tables and constants are canonical only when consumed by executable code,
tests, or exported SDK ABI paths. Source comments, prose documents,
plans, ledgers, generated PDFs, and audit inventories are not canonical.
If a source comment contradicts executable behaviour in a source-routed
area discovered by the audit, the comment must be fixed even when the
executable code is otherwise unchanged.

### Five Manual Contracts

- `IE64_ISA.md`: IE64 physical processor user's manual. It documents
  CPU ISA material only: programmer-visible state, instruction
  encodings, operand rules, condition-code state, traps, stopped
  processor state, CPU-visible timer and interrupt behaviour,
  CPU-visible MMU and control registers, reset state, stack behaviour,
  and instruction-by-instruction entries.
- `IE32_ISA.md`: IE32 physical processor user's manual. It documents
  CPU ISA material only: programmer-visible state, eight-byte
  instruction encoding, operand modes, instruction behaviour, stack
  behaviour, stopped-processor conditions, timer and interrupt state,
  reset state, and instruction-by-instruction entries.
- `iemon.md`: machine monitor reference. It documents command syntax,
  aliases, argument parsing, output expectations, side effects,
  breakpoint and watchpoint behaviour, reverse/timeline behaviour,
  snapshot scope, scripts/macros/rc files, multi-CPU behaviour, and
  examples.
- `iescript.md`: IE Script reference. It documents invocation, runtime
  rules, module and function coverage, argument validation, return
  shapes, errors, callbacks, file and path rules, debug integration, and
  runnable examples.
- `architecture.md`: whole-machine Intuition Engine architecture manual.
  It documents the stable public architecture and observable subsystem
  boundaries: bus and memory model, CPU relationships, MMIO principles,
  video and audio composition, file/media/terminal integration, timing,
  concurrency, snapshots, build-profile-visible behaviour, diagrams, and
  boundaries to OS-owned systems. It must not document every private
  helper or become an IntuitionOS syscall or kernel manual.

### Empirical Inventories

The audit uses source-derived inventories as verification artefacts, not
as authority:

- `sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md`
- `sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md`
- `sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`
- `sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`

Inventory fact rows must be generated or mechanically checked from
source parsers, source snippets, tests, or focused runnable probes.
Manual edits to factual rows are forbidden unless the check re-derives
or verifies the same row from source. Inventories must not contain
statuses, TODOs, inferred claims, review prose, or future-work notes.
Manual gaps are test failures, not inventory content.

### Source Routing

For IE64 and IE32 ISA claims, route facts through executable CPU source,
MMU source where applicable, source-consumed constants, relevant CPU/JIT
tests, and the empirical ISA inventory. Assembly syntax is reader-facing
only when it describes encoded instruction forms. Assembler-tool
implementation, monitor rendering, disassembler behaviour, loader
contracts, platform devices, host mechanics, and emulator/runtime
phrasing are outside the ISA manuals.

For IEMon claims, mechanically derive command names, aliases, syntax
forms, and dispatch paths from the monitor command registry and dispatch
code. Parser types, state mutation, stable output or usage text,
snapshot behaviour, reverse/timeline behaviour, watchpoints, page
guards, and side effects must be checked against command
implementations, tests, or focused monitor probes.

For IE Script claims, mechanically derive module and function names from
Lua binding registration. Argument validation, return shapes, callbacks,
error paths, monitor command filters, and file/path safety rules must be
checked against helper bodies, tests, or focused `.ies` probes.

For architecture claims, perform source discovery before judging
coverage. The discovery inventory must include root Go files by
subsystem prefix, build-tag variants, constants files defining mapped
addresses, constructors, registries, dispatch switches, exported
script/debug/monitor surfaces, CPU runners, JIT availability gates,
bus/device mappings, memory sizing, audio/video engines,
file/media/terminal integration, snapshots, and includes consumed by
code or tests.

Architecture inventory rows are classified as:

- public/stable architecture surface
- observable implementation boundary
- private implementation detail
- test-only or support-only code
- out-of-scope OS-owned material

`architecture.md` must cover the public/stable architecture surface and
observable implementation boundaries. It may mention private
implementation details only when needed to explain a stable boundary. It
must exclude test-only/support-only code and OS-manual material except
for boundary notes.

### Gates

Negative gates reject forbidden scope, stale wording, audit/process
prose, unshipped-document authority links, forbidden dash characters,
and manual-specific implementation-boundary leaks.

Positive gates compare each manual against its empirical inventory. A
new source registration, command, binding, subsystem, memory or MMIO
range, CPU/JIT availability path, opcode, side effect, or edge case must
fail focused tests until the proper manual covers it at the required
reference depth.

The fit-for-purpose gate remains mandatory. Accurate but shallow
material is a defect. Misstructured sections are defects. Tables,
examples, diagrams, and cross-references must support the actual
reference tasks of experienced software engineers. Audit/process
scaffolding must be removed or rewritten as reader-facing technical
reference material.

Manual-specific implementation-boundary tolerance:

- ISA manuals: no implementation-boundary prose in reader text except
  instruction syntax and encoding facts. Implementation evidence stays
  in audit files.
- IEMon and IE Script: implementation-boundary details are allowed only
  when they explain user-visible command or API behaviour.
- Architecture: implementation-boundary language is allowed, but stable
  architecture must be distinguished from current implementation detail.

### TDD and Code Bugs

The audit is source-outward and TDD-driven:

1. Add or update the inventory, test, or focused probe that exposes stale
   docs or source drift.
2. If the manual is wrong and executable source is correct, fix the
   manual and add or adjust the document gate.
3. If the manual exposes source behaviour that is wrong for the intended
   public contract, add a failing behavioural test or focused runnable
   probe first, fix every implementation path that owns the behaviour,
   update stale tests that pinned wrong behaviour, and only then update
   the manual.
4. If source comments in source-routed files are stale, fix them.
5. If generated tables or inventories drift, fix the generator or check
   rather than only the output.
6. Update audit evidence.
7. Run focused verification.
8. Render PDFs only after source, comments, tests, inventories, ledger,
   and manuals stop changing.

The intended public contract is determined in this order:

1. executable behaviour when it is intentional, tested, and source-backed
2. already accepted manual/audit contract or existing tests when source
   drift violates them
3. explicit product decision when neither source nor documents establish
   intent

If a behaviour is externally observable, assess compatibility before
changing it and preserve compatibility where feasible. If a breaking
correction is necessary, update source, tests, examples, manuals, and
audit evidence together. Code bugs in public behaviour covered by the
five manuals are in scope and must be fixed with full TDD. Defects
outside the five-manual public surface should be recorded separately or
fixed in a separate scoped pass unless they block accurate documentation
of the current manual claim.

The full `go test -tags headless ./...` suite is not a completion gate
for this audit while the full suite is known broken. It may be run only
as a diagnostic and must not block or certify the five-manual audit until
repaired separately.

### Final PDF Render Manifest

PDF rendering is the final delivery action. A valid render entry records
hashes or a source revision for source files discovered by inventories,
inventory files, audit tests, manuals, the render script, and generated
PDFs. It also records the exact render command, relevant tool version
when available, and output PDF sizes.

Any later source, source-comment, inventory, test, ledger, manual, or
render-script change invalidates the render and requires focused
verification plus PDF regeneration.

## Scope

The goal is to ship accurate and complete companion PDFs for:

- `sdk/docs/IE64_ISA.md`
- `sdk/docs/IE32_ISA.md`
- `sdk/docs/iemon.md`
- `sdk/docs/iescript.md`
- `sdk/docs/architecture.md`

These five documents are the public deliverable. They should be accurate
enough to stand beside the Programmer's Reference Guide as end-user
references for the IE64 and IE32 instruction sets, the machine monitor,
IE Script, and the system architecture.

Audience split: the Programmer's Reference Guide is the beginner-to-
intermediate book. These five companion manuals are for experienced
programmers and software engineers who need precise reference material,
complete public-surface coverage, and enough examples to remove
ambiguity without turning the manuals into tutorials.

Each shipped manual must be:

- technically accurate: every user-facing command, API, encoding,
  register, side effect, error, and example is checked against canonical
  source, tests, or runnable probes
- complete for purpose: it covers the full supported public surface for
  the thing it claims to document, while explicitly marking unsupported
  or out-of-scope behaviour
- fit for purpose: its structure, examples, tone, and level of detail
  match how the intended reader will use that specific manual
- actively repaired for purpose: if the source-outward audit shows that
  a manual is accurate but not useful enough for its stated role, the
  deficient material is rewritten, expanded, split, or replaced before
  the manual can pass
- relevant: obsolete plans, duplicate tutorial prose, historical notes,
  implementation trivia, unsupported feature promises, and material
  better owned by another maintained manual are removed, archived, or
  moved after useful claims are source-checked
- closed reference set: the five shipped manuals may refer to each other
  but must not refer readers to other books, support manuals, plan files,
  READMEs, external manuals, or unshipped documentation as required
  reading or authority
- style-clean: prose uses British English, with exact code identifiers,
  command names, file names, API names, quoted output, and source-owned
  symbols left unchanged; prose and tables contain no em dash or en dash
  characters

Purpose by manual:

- `IE64_ISA.md`: formal reference for the invented IE64 CPU ISA, including
  programmer's model, instruction encoding, operand rules, control
  registers, traps, memory/protection behaviour, assembler syntax, and
  instruction-by-instruction reference material.
- `IE32_ISA.md`: formal reference for the invented IE32 CPU ISA, including
  programmer's model, instruction encoding, operand rules, interrupts,
  timing/WAIT behaviour, stack behaviour, stopped-processor conditions,
  and instruction-by-instruction reference material.
- `iemon.md`: end-user reference for the machine monitor, including
  command syntax, output expectations, side effects, breakpoint and
  watchpoint behaviour, reverse/timeline behaviour, scripts/macros/rc
  files, multi-CPU behaviour, and examples.
- `iescript.md`: end-user reference for IE Script automation, including
  invocation, module/function coverage, arguments, return values, error
  behaviour, callbacks, file access rules, debug integration, and
  runnable examples.
- `architecture.md`: systems architecture manual for experienced
  programmers, covering the stable machine model: bus and memory model,
  CPU subsystem relationships, MMIO principles, video and audio
  subsystem composition, file/media/terminal integration points,
  timing/concurrency/snapshot boundaries, build-profile implications
  where they affect observable behaviour, and source-backed diagrams.
  It must explain how major components interact without becoming a
  dumping ground for transient implementation notes, old plans, or
  subsystem minutiae better covered by the four reference manuals.

The IE64 and IE32 ISA references should use the structural style of a
classic Motorola processor manual: formal programmer's model,
instruction formats, addressing/operand rules, condition and trap
behaviour, instruction-by-instruction entries, and precise notes. Do not
copy prose or tables from external manuals.

### IE ISA Manual Hard Gate

The IE32 and IE64 ISA manuals document CPU ISA material only. Valid
material includes the programmer model, instruction encodings,
addressing and operand rules, CPU registers, condition-code state, traps
or stopped-processor state, CPU-visible timer and interrupt behaviour,
CPU-visible MMU and control registers, reset state, stack behaviour, and
instruction-by-instruction entries.

The IE32 and IE64 ISA manuals must not document platform devices,
audio/video/peripheral catalogues, platform memory maps, loader helpers,
host, emulator, or virtual-machine mechanics, architecture-manual
handoffs, monitor rendering, disassembler behaviour, or assembler-tool
implementation notes. They must read as physical processor manuals, not
as implementation notes for Intuition Engine.

Every ISA instruction entry must use the manual's entry schema and must
include exact, source-backed `Instruction Fields` text for that entry.
Generic field prose is an audit defect. Phrases such as `where
applicable`, `when the form uses`, and `operand32 or branch target` are
not allowed in an instruction entry. A multi-form entry may describe
multiple encodings only by naming each concrete encoding and each field
consumed, reserved, or ignored by that encoding.

IE32 non-branch instructions must not mention branch targets. Loader or
programme-loading behaviour belongs outside the ISA manuals unless the
CPU itself implements it. Ledger entries cannot close an ISA manual scope
or Motorola-style finding unless the manual text and regression tests
changed, or the finding is proven not to be a defect with source-backed
evidence.

## Initial Five-Book Review Gate

Before rewriting, deleting, moving, or adding material, the audit must
perform an initial diagnostic review of all five shipped manuals as
books. This review is separate from the later claim-by-claim correction
work.

The review evidence must be chronological, not retrospective. A run must
record its starting source state, create one `OPEN` review entry for each
of the five manuals, and display the five review finding inventories in
the run output before non-blocking fixes, rewrites, removals, PDF
generation, or claim-group closure work begins. Evidence added after
fixes can explain history, but it cannot satisfy the initial-review gate
for that run; the run must be restarted or marked incomplete.

For each book, the initial review must record a problems/gaps inventory
covering:

- stated purpose and intended reader
- current structure and whether it supports that purpose
- missing public-surface areas discovered from source enumeration
- shallow, ambiguous, misleading, duplicate, obsolete, or wrong-manual
  material
- tables, examples, diagrams, appendices, and cross-references that need
  correction, expansion, removal, or replacement
- other SDK documents that may need to be mined as claim sources
- source, test, generated-table, or runnable-probe routes needed to
  verify the suspected gaps

The displayed review output must include all five manual names, the
concrete finding list for each manual, and the challenge answer for each
manual. A successful final response must also display the per-book
review findings, so the user can see what was found and how each finding
was closed without searching the ledger.

Before acting on any book-level review finding, the reviewer must
adversarially challenge the review by asking: "Do you disagree with this
review?" The answer must be recorded. If the challenge exposes weak
evidence, overreach, missing source enumeration, a premature rewrite, or
an alternative interpretation, the finding remains open until the review
is corrected and challenged again.

Each agreed review finding must have a closure pointer to a concrete
Markdown, source-code, source-comment, generated-table, or test change,
or to explicit not-a-defect evidence. A finding that is agreed as real
cannot be closed only by writing a ledger entry. If no shipped manual,
source, comment, generated artefact, or test changed after an agreed
finding, the finding remains `OPEN` until a real fix is made or the
challenge is corrected to show it was not a defect.

The audit may fix an immediately blocking typo or broken build/test
setup discovered while preparing the review, but it must not treat the
run as complete until the five-book problems/gaps inventory has been
recorded, adversarially challenged, and each recorded item has been
closed, dispositioned as not a defect with evidence, or moved into a
later explicit audit-scope entry.

## Fit-For-Purpose Rewrite Gate

The audit is not allowed to close a manual merely because the existing
sentences are technically correct. Each manual must be judged against its
purpose statement above and against the way an experienced programmer
would use that specific reference.

The five shipped manuals are user-facing engineering references for
experienced software engineers. The fit-for-purpose gate fails if a
manual contains agent instructions, audit process notes, ledger status,
review-display rules, run-state language, authoring checklists, or other
internal verification scaffolding, unless that material is rewritten as
direct reader-facing technical reference material.

If the audit finds a section that is accurate but too shallow,
misstructured, audience-mismatched, ambiguous, missing required examples,
missing required tables, or missing required reference detail, that is a
defect. The action is to rewrite, add, split, retitle, or remove material
until the section fits the manual's purpose. Leaving a known-fit defect
as future work is not successful completion.

For each manual, the audit entries must record the purpose judgement:

- what public surface was enumerated from source
- which reader task the section is meant to support
- whether the current structure supports that task
- what material was rewritten, added, moved, or removed when it did not
  support that task

This gate applies to every line, table, diagram, appendix, example, and
cross-reference in the five shipped manuals.

Other SDK documents may still be mined when they contain information
needed to make one of the five shipped manuals complete and fit for
purpose. They are claim mines only, not proof and not part of the public
PDF deliverable.

This applies to any SDK document outside the shipped five-manual set,
including support manuals, cookbooks, focused hardware/player documents,
getting-started material, include-file notes, toolchain notes, platform
notes, IntuitionOS notes, and archived or planning material. Text,
tables, examples, captions, and diagrams borrowed or paraphrased from
any of those sources must pass the same hard gates as newly written
material before they can ship:

- source-outward canonical verification
- fit-for-purpose rewrite judgement
- completeness and relevance checks
- implementation, test, source-comment, and example reconciliation
- closed-reference-set cleanup
- British-English and dash-style gates
- final PDF rendering

Fold still-useful, source-checked material from other SDK documents back
into the correct shipped manual where it belongs. After that, the source
document can remain outside this audit, be ignored, or be dispositioned
only after shipped-manual links and references to it have been cleaned
up, replaced with retained archive/redirect stubs, or explicitly
documented as intentional retained paths outside the shipped set.

Documents outside this audit set are not considered reliable SDK docs
until they are added here and verified. Mining a document for candidate
claims does not make that document reliable, expand the public PDF set,
or permit the shipped manuals to link to it.

## Final PDF Render Gate

PDF rendering is the final delivery action, not an exploratory step. The
five PDFs are valid only when rendered after all manual edits, source
fixes, source-comment fixes, generated-table fixes, test fixes, ledger
hard-gate updates, style gates, and focused verification have passed.

If any shipped manual, source file, source comment, generated table,
test, or ledger hard gate changes after PDF rendering, the PDFs from
that render are provisional and cannot satisfy completion. The run must
rerun the focused gates and render the PDFs again from the final
Markdown.

## Completion Criteria

A successful audit run is complete only when:

1. an initial five-book diagnostic review has been completed, a
   problems/gaps inventory has been recorded, and each book-level review
   has been adversarially challenged with "Do you disagree with this
   review?" before rewrite work is treated as complete
2. the five initial review finding inventories have been displayed in
   the run output before fixes and displayed again in the final response
3. the review chronology proves that per-book `OPEN` review entries and
   visible finding displays existed before non-blocking fixes, rewrites,
   removals, claim-group closures, or PDF generation
4. every agreed review finding has a closure pointer to a concrete
   Markdown, source-code, source-comment, generated-table, or test
   change, or to explicit not-a-defect evidence
5. every line of the five shipped manuals has been read and classified
   as technical claim, example, table/diagram data, cross-reference,
   structural prose, or style-only prose
6. every technical claim, example, table row, diagram fact, command,
   API, field, encoding, side effect, error case, and limitation in the
   five shipped manuals has a recorded source check, test,
   generated-table check, or runnable probe
7. each manual's claimed public surface has been enumerated from
   canonical source before judging completeness; missing supported
   commands, APIs, opcodes, registers, constants, side effects, examples,
   or edge cases are audit defects even when the existing prose is
   technically correct
8. all discovered prose defects, completeness defects, implementation
   defects, stale source comments, generated-table drift, and tests that
   pin wrong behaviour are fixed, with the original claim/example
   retained as a regression target where applicable
9. every section, table, diagram, appendix, and example has passed the
   fit-for-purpose rewrite gate for user-facing manuals aimed at
   experienced software engineers; accurate but insufficient material has
   been rewritten, expanded, split, retitled, replaced, or removed rather
   than left as a known gap, and agent/audit scaffolding has been removed
   or rewritten as direct reader-facing technical reference material
10. the IE32 and IE64 ISA manuals have passed the IE ISA Manual Hard
   Gate: CPU-only scope, physical-processor voice, Motorola-style entry
   schema, exact source-backed instruction fields, no generic field
   prose, no non-branch target leakage, and no loader/tooling/platform
   device material
11. irrelevant, obsolete, duplicate, misleading, audience-mismatched, or
   wrong-manual material has been removed, moved, or archived after any
   useful technical claims in it have been source-checked
12. every reference from one of the five shipped manuals to another book
   or document is either to one of the other four shipped manuals or has
   been removed after any useful claim was source-checked and folded into
   the correct shipped manual
13. every one of the five shipped manuals has passed the British-English
   prose check, preserving exact identifiers and source-owned spellings
14. every one of the five shipped manuals has passed the dash-style check:
   no em dash or en dash characters are allowed
15. every one of the five shipped manuals declares the current audit
   run's last-modified date on page 1
16. material folded into a shipped manual from any other SDK document has
   been source-checked, rewritten as needed, and had stale links cleaned
   up
17. the five shipped manuals have been printed/rendered to PDFs
   successfully as the final step of the run, after all hard-gate,
   Markdown, source, source-comment, generated-table, and test changes
   have stopped

## Out Of Scope

Leave these areas out of this SDK audit:

- `sdk/docs/platform-compatibility.md`
- `sdk/docs/include-files.md`
- `sdk/docs/sdk-getting-started.md`
- `sdk/docs/toolchains.md`
- `sdk/docs/IntuitionOS/`

Do not expand audit scope just because an audited document links to one
of these ignored areas. These documents may be mined for candidate
claims when required, but the shipped five manuals must not link to them;
source-check and fold any necessary claim into the correct shipped manual
instead.

## Document Disposition

Each audited document should also receive a disposition recommendation:

- `KEEP`: essential SDK surface; keep as a maintained document.
- `MERGE`: useful content, but better folded into another maintained
  document.
- `ARCHIVE`: historical or planning material that should leave the
  active SDK docs tree but may be useful as project history.
- `DELETE`: obsolete, redundant, or misleading after useful claims have
  been moved or verified elsewhere.

Disposition is not a substitute for claim verification. If a document is
marked `MERGE`, verify the claims before moving them into the target
document.

## Audit Rules

The code and source assets on disk are the canonical source of truth at
all times. Existing prose documentation, including this ledger, SDK docs,
and the Programmer's Reference Guide, is background material only. It is
never proof.

Every line in each of the five shipped manuals must be audited. Technical
claims must be adversarially double checked against the code and source
assets on disk. Non-technical lines still need style, link, structure,
and reader-purpose checks. There are no exceptions. This includes prose,
tables, diagrams, examples, command syntax, API signatures, encodings,
constants, status bits, error cases, side effects,
build-profile-visible behaviour, and stated limitations.

Completeness is audited from source outward, not from the existing manual
inward. For each manual, first enumerate the supported public surface
from canonical source files, registration tables, includes, generated
tables, tests, and runnable probes. Then verify that the manual covers
that surface at the level its purpose requires. A missing command,
binding, opcode, register, MMIO constant, side effect, error case,
example, or edge-case note is a defect even if every existing sentence is
accurate.

Relevance is audited as aggressively as correctness. If a line, section,
table, example, diagram, or appendix is not useful for the stated purpose
and audience of its manual, remove it, move it to the correct maintained
manual, or archive it outside the shipped five-book set. Before removing
material, mine it for any useful technical claims and source-check those
claims; do not preserve irrelevant prose merely because some sentence in
it is accurate.

The shipped five manuals are a closed reader-facing reference set. They
may cross-reference only each other:

- `sdk/docs/IE64_ISA.md`
- `sdk/docs/IE32_ISA.md`
- `sdk/docs/iemon.md`
- `sdk/docs/iescript.md`
- `sdk/docs/architecture.md`

Do not refer readers from these manuals to support docs, cookbooks,
plans, READMEs, external CPU manuals, or unshipped books as authority or
required reading. If such a reference contains useful material, verify
the underlying claim against source and fold it into the appropriate
shipped manual instead.

External manuals may guide structure or terminology where appropriate,
but they are not authority for Intuition Engine behaviour. If prose and
implementation disagree, audit the implementation path, then either fix
the prose or fix the implementation and keep the original claim or
example as a regression target.

Source comments and author-facing source notes are part of the
maintained source surface for this audit. If they contradict executable
behaviour, includes, tests, generated tables, or the corrected manual,
fix them in the same pass. Do not leave stale comments as known drift
just because the executable code is correct.

For each audited claim:

1. Check the claim against canonical source files, tests, generated
   tables, or runnable examples.
2. Record the exact files, tests, commands, or monitor/script probes
   used as evidence.
3. If the document is wrong, fix the document.
4. If the document exposes an implementation bug, fix the implementation
   and keep the original example or claim as the regression target.
5. If tests pin the wrong behaviour, correct the tests after the
   implementation and documented behaviour have been reconciled.
6. If source comments or author-facing source notes are stale, fix them.
7. If the material is irrelevant, obsolete, duplicate, misleading, or
   belongs in another maintained manual, remove it, move it, or archive
   it after source-checking any useful claims.
8. Do not replace a failing example with a nearby workaround merely to
   make the document pass.

SDK documents may mention repository paths, build commands, host tooling,
implementation details, and links where those are accurate and useful.
They are not constrained by the printed guide's standalone-book voice.

The five shipped manuals must still follow the SDK companion style gate:
British English in prose and no em dash or en dash characters anywhere in
reader-facing text, tables, examples, captions, or diagrams. Exact code,
path, command, API, register, constant, quoted-output, and source-owned
spellings are not rewritten merely to satisfy British-English prose.

## Source Trust Policy

Use existing SDK prose only as orientation. A claim, table row, example,
caption, diagram fact, or explanatory passage copied or paraphrased from
another document still needs the same canonical-source check,
fit-for-purpose judgement, relevance check, implementation/test/comment
reconciliation, closed-reference cleanup, and style gate as a new claim.

Prefer these sources, in order:

1. `sdk/include/*.inc` for exported SDK constants, MMIO addresses, token
   values, ABI symbols, and bit fields.
2. Go source and assembly source for actual runtime, CPU, monitor,
   script, file, audio, video, and tool behaviour.
3. Tests and generated tables for pinned edge cases and drift checks.
4. Runnable command, monitor, script, or example probes when source and
   tests do not already prove the user-visible behaviour clearly.

For IE-native ISA documents, use IE source, includes, tests, assemblers,
and disassemblers as the authority. If later audit scope adds
non-IE-native ISA reference documents, check architectural claims against
both the Intuition Engine implementation and an appropriate primary CPU
reference; record the consulted reference in the audit entry notes.

## Canonical Source Routing

### `IE64_ISA.md`

Check IE64 instruction encoding, semantics, register behaviour, traps,
system registers, assembler syntax, and disassembly against:

- `cpu_ie64*.go`
- `debug_disasm_ie64.go`
- `assembler/`
- `sdk/include/*.inc`
- IE64 assembler/disassembler tests
- IE64 CPU and JIT tests

### `IE64_ABI.md`

Check IE64 calling conventions, stack alignment, trap frames, supervisor
access rules, IntuitionOS ABI boundaries, exported constants, and examples
against:

- `cpu_ie64*.go`
- `registers.go`
- `trap*.go`
- `sdk/include/ie64.inc`
- `sdk/include/iexec.inc`
- `sdk/intuitionos/`
- IE64 CPU, trap, memory-protection, and IntuitionOS tests

### `IE64_COOKBOOK.md`

Check examples and calling conventions against:

- `sdk/docs/IE64_ABI.md` for conflict discovery only, not proof
- `sdk/include/*.inc`
- `sdk/examples/asm/*.asm`
- `assembler/`
- relevant CPU, MMIO, file, video, audio, and monitor tests

Use `IE64_ABI.md` only to find conflicts and cross-document drift. It is
not proof. Verify both the cookbook claim and any corresponding ABI claim
against code, includes, tests, generated tables, or runnable probes.
Where practical, assemble examples and run them under IE or IE Mon.

### `IE32_ISA.md`

Check IE32 instruction encoding, operand modes, CPU behaviour, monitor
disassembly, assembler/tooling claims, timing notes, and examples against:

- `cpu_ie32*.go`
- `debug_disasm_ie32.go`
- `assembler/`
- `sdk/include/*.inc`
- IE32 assembler/disassembler tests
- IE32 CPU tests

### `architecture.md`

Check whole-machine architecture claims, diagrams, subsystem boundaries,
memory/bus maps, CPU integration, MMIO routing, audio/video composition,
file/media/terminal integration, timing/concurrency, snapshots, and
build-profile-visible behaviour against:

- `machine_bus*.go`
- `memory_sizing.go`
- `profile_bounds.go`
- `registers.go`
- `cpu_*.go`
- `jit_*.go`
- `video_*.go`
- `video_compositor.go`
- `audio_*.go`
- `*_engine.go`
- `*_player.go`
- `file_io*.go`
- `media_loader*.go`
- `terminal_io.go`
- `debug_*.go`
- `script_*.go`
- `sdk/include/*.inc`
- architecture, MMIO, CPU, audio, video, debug, script, and build-tag
  tests relevant to each claim group

Diagrams in `architecture.md` are technical claims. Audit labels,
arrows, layer ordering, ownership boundaries, data flow, and omitted
components against the same sources used for prose.

### `iemon.md`

Check monitor commands, syntax, side effects, output formats, watchpoints,
breakpoints, reverse execution, assembly mode, and saved-state behaviour
against:

- `debug_*.go`
- `debug_commands.go`
- `debug_cpu_*.go`
- monitor and debugger tests
- focused IE Mon transcript probes

### `iescript.md`

Check script modules, functions, callback names, return values, errors,
file access rules, CPU/debug bindings, video/audio helpers, and examples
against:

- `script_*.go`
- script binding registration code
- script engine tests
- focused `.ies` probes

## Audit Evidence Log

The entries below are chronological evidence for completed audit work.
They do not override the control plan above. If an evidence entry
conflicts with a later source-backed contract, the later contract and the
current executable source route control the next audit pass.

## Entry Format

Use one entry per claim group. Keep entries small enough that a future
reader can repeat the check without guessing.

```text
ID: SDK-DOC-0001
Status: OPEN | CHECKED | FIXED
Document:
Section:
Claim:
Purpose judgement:
Canonical sources checked:
Runnable verification:
Observed result:
Action:
Notes:
Disposition:
```

`OPEN` means the check is in progress or a defect is not fixed yet.
`CHECKED` means the prose already matched the implementation.
`FIXED` means the audit found a defect and the document or
implementation was corrected.

## Audit Entries

ID: SDK-DOC-0036
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`
Section: Initial five-book diagnostic review - IE64.
Claim: The IE64 companion manual must be a formal reference for
experienced programmers, covering programmer-visible registers,
encoding, operand rules, instructions, control registers, traps, memory
protection, assembler syntax, memory map, examples, and the public ABI
material needed by IE64 users.
Purpose judgement: The current structure supports that task with a
programmer model, fixed-width encoding description, complete instruction
reference, pseudo-instructions, addressing rules, interrupt and MMU
chapters, memory map, encoding examples, opcode appendices, and an
IntuitionOS ABI appendix. The missing-risk areas were source coverage of
the opcode/FPU surface, the TLBINVAL operand description, complete MMIO
range coverage, assembler directive coverage, and folded ABI material.
Canonical sources checked: `cpu_ie64.go`, `mmu_ie64.go`,
`registers.go`, `debug_disasm_ie64.go`, `assembler/`, `sdk/include/`,
IE64 CPU, assembler, disassembler, MMU, and ABI-focused tests.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`
and the final PDF render command recorded in `SDK-DOC-0031`.
Observed result: The shipped manual contains the source-enumerated
instruction surface, FPU coverage, memory map rows, assembler directive
surface, TLBINVAL virtual-address wording, and ABI appendix material
required by the review.
Action: Closed by the current `sdk/docs/IE64_ISA.md` content and pinned
by `sdk_doc_audit_test.go` checks for headings, opcode coverage, FPU
coverage, TLBINVAL wording, memory-map rows, ABI appendix text,
assembler directives, encoding bytes, style, and closed-reference rules.
This pass also added `#### 8.1.1 RAM and MMIO Programming Rules`,
expanded the `TLBINVAL` entry to state that Rs holds a virtual address,
and fixed the stale `OP_TLBINVAL` source comment in `cpu_ie64.go`.
Notes: Review display:
- Purpose/readers: formal IE64 reference for experienced programmers.
- Structure: supports the purpose with programmer model, encoding,
  instruction entries, memory map, MMU, examples, opcode appendices, and
  ABI appendix.
- Source-enumerated surface route: `cpu_ie64.go`, `mmu_ie64.go`,
  `registers.go`, `debug_disasm_ie64.go`, `assembler/`,
  `sdk/include/*.inc`, and IE64 tests.
- Findings: opcode appendix and FPU section needed source-coverage
  evidence; TLBINVAL had to be documented as taking a virtual address in
  `Rs`; memory map needed complete MMIO rows; ABI material from support
  docs had to be folded into the shipped manual; assembler directive and
  escape coverage needed pinned checks.
Finding closures:
- `sdk/docs/IE64_ISA.md` sections 3, 4, 8, 11, 12, Appendix A,
  Appendix B, and Appendix C.
- `sdk/docs/IE64_ISA.md` `#### 8.1.1 RAM and MMIO Programming Rules`
  for total/active RAM discovery and strict MMIO window rules.
- `sdk/docs/IE64_ISA.md` `**TLBINVAL** (opcode 0xEA)` for the
  virtual-address operand, page-offset handling, and JIT invalidation
  side effect.
- `cpu_ie64.go` `OP_TLBINVAL` source comment, changed from VPN wording
  to virtual-address wording.
- `sdk_doc_audit_test.go`
  `TestSDKCompanionDocs_IE64OpcodeTableMatchesSource`,
  `TestSDKCompanionDocs_IE64FPUSectionCoversSourceOpcodes`,
  `TestSDKCompanionDocs_IE64TLBINVALDocumentsVAOperand`,
  `TestSDKCompanionDocs_IE64MemoryMapCoversSourceRanges`,
  `TestSDKCompanionDocs_IE64ABIV0AppendixCoversSupportContract`,
  `TestSDKCompanionDocs_IE64AssemblerDirectiveSurface`, and
  `TestSDKCompanionDocs_ISAEncodingExamplesMatchSourceConstants`.
Challenge answer to "Do you disagree with this review?": no.
Disposition: KEEP.

ID: SDK-DOC-0037
Status: FIXED
Document: `sdk/docs/IE32_ISA.md`
Section: Initial five-book diagnostic review - IE32.
Claim: The IE32 companion manual must be a formal reference for
experienced programmers, covering the register file, eight-byte
instruction format, operand modes, complete opcode surface, branch
rules, stack behaviour, timer and interrupt behaviour, monitor-visible
disassembly expectations, assembler syntax, execution caveats, and byte
examples.
Purpose judgement: The current structure supports that task with a
programmer model, encoding chapter, complete instruction reference,
addressing-mode chapters, branch, stack, timer, assembler, caveat, and
opcode/example appendices. The missing-risk areas were opcode-table
coverage, accurate byte examples, caveats for raw encodings and store
operands, register-indirect offset wording, and assembler directive plus
diagnostic coverage.
Canonical sources checked: `cpu_ie32.go`, `debug_disasm_ie32.go`,
`assembler/`, `sdk/include/`, IE32 CPU, assembler, disassembler, and
instruction tests.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`
and the final PDF render command recorded in `SDK-DOC-0031`.
Observed result: The shipped manual covers the source-enumerated IE32
opcode surface, addressing modes, caveats, assembler syntax, diagnostic
switches, and byte examples expected by the review.
Action: Closed by the current `sdk/docs/IE32_ISA.md` content and pinned
by `sdk_doc_audit_test.go` checks for headings, opcode coverage,
assembler directives, equivalent opcode appendix shape, encoding bytes,
style, and closed-reference rules.
This pass also added `### 10.9 Monitor Disassembly Contract`, including
unknown-opcode display, operand-prefix rendering, and branch-target
metadata expectations from `debug_disasm_ie32.go`.
Notes: Review display:
- Purpose/readers: formal IE32 reference for experienced programmers.
- Structure: supports the purpose with register model, encoding,
  addressing modes, full instruction reference, timer/interrupt model,
  caveats, opcode appendix, and byte examples.
- Source-enumerated surface route: `cpu_ie32.go`,
  `debug_disasm_ie32.go`, `assembler/`, `sdk/include/*.inc`, and IE32
  tests.
- Findings: opcode appendix needed source-coverage evidence;
  addressing-mode examples had to show actual eight-byte encodings;
  store/register and register-indirect caveats needed reader-facing
  wording; assembler directives and diagnostics needed pinned coverage.
Finding closures:
- `sdk/docs/IE32_ISA.md` sections 3, 4, 5, 9, 10, 11, Appendix A, and
  Appendix B.
- `sdk/docs/IE32_ISA.md` `### 10.9 Monitor Disassembly Contract` and
  its table-of-contents entry.
- `sdk_doc_audit_test.go`
  `TestSDKCompanionDocs_IE32OpcodeTableMatchesSource`,
  `TestSDKCompanionDocs_IE32AssemblerDirectiveSurface`,
  `TestSDKCompanionDocs_ISAOpcodeAppendicesUseEquivalentTableShape`,
  and `TestSDKCompanionDocs_ISAEncodingExamplesMatchSourceConstants`.
Challenge answer to "Do you disagree with this review?": no.
Disposition: KEEP.

ID: SDK-DOC-0038
Status: FIXED
Document: `sdk/docs/iemon.md`
Section: Initial five-book diagnostic review - IEMon.
Claim: The monitor companion manual must be an end-user command
reference covering syntax, address parsing, output expectations, side
effects, breakpoint and watchpoint behaviour, reverse/timeline
behaviour, scripts, macros, rc files, multi-CPU workflows, and examples.
Purpose judgement: The current structure supports that task with quick
start, address formats, argument parsing, conditional breakpoints,
reverse history, project rc files, command history, watchpoints, page
guards, access history, source-checked command surface, grouped command
reference, CPU notes, workflows, display behaviour, IE64 fault reports,
and pitfalls. The missing-risk areas were full help-registry command and
alias coverage, side-effect coverage for state-changing commands,
reverse/timeline wording, and IE64 fault-field coverage.
Canonical sources checked: `debug_commands.go`, `debug_monitor.go`,
`debug_conditions.go`, `debug_access.go`, `debug_snapshot.go`,
`debug_ioview.go`, `debug_cpu_*.go`, monitor tests, debugger UX tests,
and focused command-registry checks.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`
and the final PDF render command recorded in `SDK-DOC-0031`.
Observed result: The shipped manual documents every command and alias
from the monitor help registry, includes the source-checked command
surface table, and describes the relevant side effects and workflow
boundaries for the reviewed command groups.
Action: Closed by the current `sdk/docs/iemon.md` content and pinned by
`sdk_doc_audit_test.go`
`TestSDKCompanionDocs_IEMonCommandCoverageMatchesHelpRegistry`, the
fit-for-purpose heading gate, style gates, and closed-reference rules.
This pass also added `### Command Effect Matrix` so state-changing,
host-file, execution-control, and inspection commands are separated
explicitly for end users and scripts.
Notes: Review display:
- Purpose/readers: end-user reference for the machine monitor.
- Structure: supports the purpose with address parsing,
  break/watch/page-guard behaviour, reverse/timeline behaviour, command
  reference, CPU notes, workflows, and fault reports.
- Source-enumerated surface route: `debug_commands.go` help registry,
  `debug_*.go`, monitor/debug tests, and transcript-style probes where
  source alone is insufficient.
- Findings: command reference had to cover every help-registry command
  and alias, including reverse/timeline/access-log/page-guard/report
  commands; side effects for CPU focus, freeze/thaw, state save/load,
  scripts, macros, and audio freeze needed explicit coverage; fault
  interception and IE64 fault fields needed source-backed wording.
Finding closures:
- `sdk/docs/iemon.md` sections `Address Formats`, `Conditional
  Breakpoints`, `Command Reference`, `CPU-Specific Notes`,
  `Multi-CPU Debugging Workflows`, `IE64 Fault Reports`, and `Common
  Pitfalls`.
- `sdk/docs/iemon.md` `### Command Effect Matrix`, added in this pass.
- `sdk_doc_audit_test.go`
  `TestSDKCompanionDocs_IEMonCommandCoverageMatchesHelpRegistry` and
  `TestSDKCompanionDocs_FitForPurposeReferenceScaffold`.
Challenge answer to "Do you disagree with this review?": no.
Disposition: KEEP.

ID: SDK-DOC-0039
Status: FIXED
Document: `sdk/docs/iescript.md`
Section: Initial five-book diagnostic review - IEScript.
Claim: The IE Script companion manual must be an end-user Lua automation
reference covering invocation, runtime rules, module/function coverage,
arguments, returns, errors, callbacks, file access rules, debug
integration, cancellation, and runnable examples.
Purpose judgement: The current structure supports that task with launch
modes, runtime and safety rules, module conventions, function groups for
all exported modules, recording/REPL/EhBASIC sections, worked examples,
troubleshooting, cancellation semantics, common pitfalls, and a quick
reference. The missing-risk areas were binding-name coverage, quick
reference counts, sandbox/path/freeze behaviour, monitor-command
restrictions, return-shape wording, and example coverage.
Canonical sources checked: `script_engine.go`, `script_engine_test.go`,
`script_rotozoomer_ies_test.go`, monitor integration code, media/audio
and video helper code, and script path-validation tests.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`
and the final PDF render command recorded in `SDK-DOC-0031`.
Observed result: The shipped manual mentions every exported Lua binding
from `script_engine.go`, count-bearing quick-reference headings match
the binding registrations, and safety/concurrency rules are presented
as reader-facing behaviour rather than internal process notes.
Action: Closed by the current `sdk/docs/iescript.md` content and pinned
by `sdk_doc_audit_test.go`
`TestSDKCompanionDocs_IEScriptFunctionCoverageMatchesBindings`, the
fit-for-purpose heading gate, style gates, and closed-reference rules.
This pass also added `### Error and Result Rules`, including Lua error
path behaviour, result-type conventions, and the monitor command filter
for `dbg.command` / `dbg.command_output`.
Notes: Review display:
- Purpose/readers: end-user Lua automation reference.
- Structure: supports the purpose with runtime/safety rules, module
  reference, examples, troubleshooting, cancellation semantics, and
  quick reference.
- Source-enumerated surface route: `script_engine.go` module
  registration, script tests, monitor integration, file/path validation
  code, and audio/video helper implementations.
- Findings: module/function quick reference had to match exported
  bindings for all modules; sandbox/path/freeze rules needed explicit
  reader-facing behaviour; examples had to use exported functions and
  known return shapes; monitor-command restrictions and auto-release
  behaviour needed source-backed coverage.
Finding closures:
- `sdk/docs/iescript.md` sections `Script Runtime Model`, `Safety and
  Concurrency Rules`, `Module Reference`, `Worked Examples`,
  `Troubleshooting`, `Script Cancellation and Auto-Release`, and `Quick
  Reference`.
- `sdk/docs/iescript.md` `### Error and Result Rules`, added in this
  pass.
- `sdk_doc_audit_test.go`
  `TestSDKCompanionDocs_IEScriptFunctionCoverageMatchesBindings` and
  `TestSDKCompanionDocs_FitForPurposeReferenceScaffold`.
Challenge answer to "Do you disagree with this review?": no.
Disposition: KEEP.

ID: SDK-DOC-0040
Status: FIXED
Document: `sdk/docs/architecture.md`
Section: Initial five-book diagnostic review - architecture.
Claim: The architecture companion manual must describe the stable
whole-machine model for experienced programmers: bus and memory model,
CPU relationships, MMIO principles, video and audio composition,
file/media/terminal integration, timing and concurrency, snapshot
boundaries, build-profile-visible behaviour, and diagrams whose labels
and arrows are treated as technical claims.
Purpose judgement: The current structure supports that task with a
diagram-reading key, complete architecture diagram, whole-system and
layered overviews, subsystem and JIT matrices, CPU, video, audio,
memory, I/O, data-flow, concurrency, timing, and source-file sections.
The missing-risk areas were diagram/table source pointers, JIT matrix
coverage, MMIO range coverage, video/audio composition, scripting and
debug integration, snapshot boundaries, build-profile-visible behaviour,
and removal of obsolete plan-style or unshipped-document references.
Canonical sources checked: `machine_bus*.go`, `memory_sizing.go`,
`profile_bounds.go`, `registers.go`, `cpu_*.go`, `jit_*.go`,
`video_*.go`, `video_compositor.go`, `audio_*.go`, `*_engine.go`,
`*_player.go`, `file_io*.go`, `media_loader*.go`, `terminal_io.go`,
`debug_*.go`, `script_*.go`, `sdk/include/`, subsystem tests, and
build-tag-aware source files.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`
and the final PDF render command recorded in `SDK-DOC-0031`.
Observed result: The shipped manual has the required diagram/table
structure, subsystem matrices, memory map, concurrency model, and
source-file appendix, with audit/process scaffolding and unshipped-doc
references removed from reader-facing text.
Action: Closed by the current `sdk/docs/architecture.md` content and
pinned by `sdk_doc_audit_test.go`
`TestSDKCompanionDocs_FitForPurposeReferenceScaffold`, style gates, and
closed-reference rules.
This pass also added `## Build Profiles and Observable Runtime` to make
the headless, novulkan, and headless-novulkan build-profile effects
reader-facing instead of implicit Makefile knowledge.
Notes: Review display:
- Purpose/readers: systems architecture manual for experienced
  programmers.
- Structure: supports the purpose with whole-system diagrams, subsystem
  matrix, JIT matrix, CPU/video/audio/memory/I/O/data-flow/timing
  sections, and source-file appendix.
- Source-enumerated surface route: `machine_bus*.go`,
  `memory_sizing.go`, `profile_bounds.go`, `registers.go`,
  `cpu_*.go`, `jit_*.go`, `video_*.go`, `audio_*.go`,
  `file_io*.go`, `media_loader*.go`, `terminal_io.go`, `debug_*.go`,
  `script_*.go`, and SDK includes.
- Findings: diagrams and tables had to be treated as technical claims;
  CPU/JIT matrix, bus/MMIO ranges, video/audio composition,
  scripting/debug integration, snapshot boundaries, and
  build-profile-visible behaviour needed source pointers; obsolete
  plan-style prose and links to unshipped docs had to stay out of the
  shipped manual.
Finding closures:
- `sdk/docs/architecture.md` sections `Reading the Architecture Tables
  and Diagrams`, `Single Complete Architecture Diagram`,
  `Whole-System Architecture`, `Subsystem Matrix`, `Platform JIT
  Matrix`, `CPU Subsystem`, `Video Subsystem`, `Audio Subsystem`,
  `Memory Map`, `I/O Peripherals`, `Data Flow`, `Concurrency Model and
  System Timing`, and `Appendix: Key Source Files`.
- `sdk/docs/architecture.md` `## Build Profiles and Observable
  Runtime`, added in this pass.
- `sdk_doc_audit_test.go`
  `TestSDKCompanionDocs_FitForPurposeReferenceScaffold`,
  `TestSDKCompanionDocs_NoAuditProcessLanguage`, and
  `TestSDKCompanionDocs_NoReferencesToUnshippedBooks`.
Challenge answer to "Do you disagree with this review?": no.
Disposition: KEEP.

ID: SDK-DOC-0030
Status: FIXED
Document: All five shipped manuals.
Section: Source-outward claim groups, line classification, style gates,
closed-reference rules, and fit-for-purpose scaffold.
Claim: The five shipped manuals have been read as complete
reader-facing books, classified into repeatable claim groups, checked
against canonical source routes, repaired where necessary, and guarded
by focused regression tests for the public surfaces most likely to drift.
Purpose judgement: The manuals now match the companion-set roles stated
in this ledger: IE64 and IE32 are formal ISA references; IEMon and
IEScript are operational references; architecture is a systems
architecture manual. Accurate but shallow or misplaced material was
expanded, retitled, moved into the right shipped manual, or removed from
the shipped set.
Canonical sources checked: The source routes listed in `SDK-DOC-0036`
through `SDK-DOC-0040`; `sdk_doc_audit_test.go`; the five target
Markdown files; generated PDF outputs.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`;
style and closed-reference checks are implemented in
`sdk_doc_audit_test.go`.
Observed result: The focused SDK audit test suite passes after the
ledger entries and the five manual repairs from this pass are present.
The tests cover page-one last-modified dates, forbidden dash characters,
British-English prose spellings, fit-for-purpose headings,
initial-review ledger entries, audit-process language removal,
unresolved placeholder removal, IE64/IE32 opcode and encoding coverage,
IE64 MMU/memory/ABI details, IEScript binding coverage, IEMon
help-registry command coverage, assembler directive surfaces, and
references to unshipped documents.
Action: Added `sdk_doc_audit_test.go` as the regression gate for the
five-manual companion set and updated the shipped manuals to satisfy the
source-outward coverage, style, closed-reference, and purpose gates.
Notes: The test file deliberately preserves source-owned spellings and
tokens, such as Lua table key `color` and Mermaid `color:` attributes,
while rejecting American-English prose in normal text. No open
claim-group backlog remains for the shipped five-manual set.
Disposition: KEEP.

ID: SDK-DOC-0041
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/iescript.md`, and `sdk_doc_audit_test.go`.
Section: Post-review defect corrections from manual consistency review.
Claim: Four defects found after the initial completion mark are fixed in
the manuals and pinned by focused regression checks: the IE64 table of
contents appendix anchor, IE32 store notation order, IE64 TED MMIO range,
and IEScript raw monitor command filter wording.
Purpose judgement: These were not optional polish items. They were
reader-visible correctness defects in the shipped manuals: one broken
cross-reference, one inconsistent formal notation, one stale MMIO range,
and one incomplete error/filter rule in a newly added convention section.
Canonical sources checked: `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/iescript.md`,
`ted_video_constants.go`, `script_engine.go`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now includes explicit
regression checks for all four defects. The tests fail on the stale
anchor, the old `store(operand, value)` schema wording, the TED
`$F0F00-$F0F5F` row, or an IEScript convention section that omits
`trace file` rejection with the `trace file off` exception.
Action: Updated `sdk/docs/IE64_ISA.md` table of contents to
`#appendix-a-opcode-map`; changed IE32 schema wording to
`store(value, operand)`; changed the IE64 TED aggregate row to
`$F0F00-$F0F6B` / 108 B / TED audio, player, and video registers; and
updated the IEScript error/result convention to include `trace file`
rejection with `trace file off` allowed. Added
`TestSDKCompanionDocs_IE64TOCAppendixAnchor`,
`TestSDKCompanionDocs_IE32StoreNotationIsConsistent`,
`TestSDKCompanionDocs_IE64TEDRangeMatchesSource`, and
`TestSDKCompanionDocs_IEScriptRawMonitorFilterMatchesSource`.
Notes: The execution plan missed these because the earlier regression
set checked broad public-surface coverage but did not include internal
Markdown anchor validation, intra-section notation consistency, TED
range parity with the TED video constants, or detailed parity for
`validateSandboxedMonitorCommand`. Those checks are now present.
Disposition: KEEP.

ID: SDK-DOC-0042
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/iescript.md`, and
`sdk_doc_audit_test.go`.
Section: Second post-review defect corrections from source parity
review.
Claim: Three additional reader-visible defects from the second review
batch are fixed and pinned by regression checks: IE64 timer cadence in
the architecture manual, IE32 `INC`/`DEC` coverage in the complete
instruction reference, and the IE64 privileged-fault table's MFCR CR6
exception. The IEScript video-count finding was challenged and rejected
for this checkout because both `script_engine.go` and the quick-reference
table expose 65 `video.*` bindings.
Purpose judgement: The accepted findings affected programming guidance
and formal reference completeness. IE64 timer programming must not be
described with IE32's `SAMPLE_RATE` cadence, real IE32 opcodes must not
be missing from the complete instruction reference, and the fault table
must not contradict the documented `MFCR Rd, CR6` user-mode exception.
Canonical sources checked: `cpu_ie64.go`, `jit_exec.go`,
`cpu_ie32.go`, `script_engine.go`, `sdk/docs/architecture.md`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/iescript.md`, and `sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now includes explicit
regression checks for IE64 interpreter timer decrement per instruction,
IE64 JIT timer decrement by retired native-block instruction count,
IE32 `SAMPLE_RATE`-gated timer cadence, IE32 arithmetic-section
coverage for `INC` and `DEC`, IE64 fault-cause wording that preserves
the `MFCR Rd, CR6` exception, and IEScript quick-reference table counts
matching source-exported bindings.
Action: Rewrote `sdk/docs/architecture.md` CPU timer cadence to split
IE32 and IE64 behaviour; added `INC` and `DEC` rows and destination-mode
explanation to `sdk/docs/IE32_ISA.md` section 4.3; qualified the
IE64 fault-cause 5 table row as `MFCR except MFCR Rd, CR6`; kept
`sdk/docs/iescript.md` at `### video (65)` because source and table both
contain 65 video bindings; added
`TestSDKCompanionDocs_ArchitectureTimerCadenceMatchesSource`,
`TestSDKCompanionDocs_IE32CompleteReferenceCoversINCDEC`,
`TestSDKCompanionDocs_IE64FaultCauseMFCRCR6Exception`, and a
quick-reference row-count check inside
`TestSDKCompanionDocs_IEScriptFunctionCoverageMatchesBindings`.
Notes: The execution plan missed these because the previous gates did
not distinguish IE32 and IE64 timer implementations, did not assert
section-level completeness for `INC` and `DEC` in the arithmetic table,
did not compare fault-cause summary wording with the nearby MFCR CR6
exception, and counted IEScript source coverage without checking the
quick-reference table row counts against the headings. Those checks are
now present.
Disposition: KEEP.

ID: SDK-DOC-0043
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/iemon.md`, `sdk/docs/iescript.md`,
and `sdk_doc_audit_test.go`.
Section: Contract-level wording and memory-semantics clarification.
Claim: The manuals now distinguish guest-facing contract from current
implementation detail without removing technical detail. The
architecture memory map now distinguishes decoded address mappings from
reservations/usages inside shared physical memory. IE64 appendices and
the IntuitionOS ABI are discoverable from the table of contents and
overview. IE32 documents canonical runtime timer addresses separately
from legacy assembler/include symbols. IEMon documents dispatch-level
aliases and IE64 fault cause 11. IEScript includes x86 in the
script-controlled JIT API documentation with host-platform caveats.
Purpose judgement: These changes make the shipped manuals clearer for
experienced users: they can tell whether a row describes bus decoding,
a subsystem reservation, an ISA/ABI/API command contract, or a current
implementation note. Top-level "Source Compatibility Note" blocks were
not retained because they are process/meta language rather than
reader-facing product documentation.
Canonical sources checked: `sdk/docs/architecture.md`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/iemon.md`, `sdk/docs/iescript.md`, `cpu_ie64.go`,
`cpu_ie32.go`, `registers.go`, `assembler/ie32asm.go`,
`sdk/include/ie32.inc`, `debug_commands.go`, `script_engine.go`,
`jit_x86_dispatch.go`, `jit_x86_exec.go`, and `sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now checks that shipped
manuals do not contain top-level Source Compatibility Note blocks, that
architecture memory-map rows explain shared physical address semantics
and overlapping reservations, that IE64 TOC/overview entries expose
Appendices A/B/C and ABI v0, that IE64 timer implementation fields are
labelled as implementation notes behind the CR9/CR10/CR11 contract, that
IE32 timer addresses distinguish runtime constants from legacy assembler
symbols, that IEMon includes `wr`/`wrw` and IE64 cause 11, and that
IEScript lists x86 in script-controlled JIT support with amd64/arm64
platform caveats.
Action: Rewrote the architecture memory map as a decoded-as/use/visible
to/owner/notes table; updated IE64 appendix TOC and timer-register
contract wording; added IE32 canonical-vs-legacy timer address wording;
changed IEMon command-surface wording to include dispatch aliases and
added `wr`/`wrw` plus IE64 `illegal` cause 11; updated IEScript JIT API
support text to include x86; added regression tests for each change.
Notes: The review findings are accepted except for the earlier proposal
to add a generic source-compatibility convention block near the top of
each shipped manual. That block was removed from all five manuals and
`TestSDKCompanionDocs_NoSourceCompatibilityNotes` prevents it from
returning. Contract-level distinctions remain inside the relevant
technical sections where they are useful to readers.
Disposition: KEEP.

ID: SDK-DOC-0044
Status: FIXED
Document: `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/iescript.md`, `sdk/docs/iemon.md`, and
`sdk_doc_audit_test.go`.
Section: Timer ABI clarity, CPU-local snapshot scope, and monitor save
argument parsing.
Claim: Four additional source-parity defects are fixed and pinned by
regression checks: IE32 timer mirror offsets no longer imply a
guest-visible timer MMIO contract, IE64 legacy timer delivery is marked
as host/test-only rather than a usable guest pattern, IEScript
`dbg.save_state` / `dbg.load_state` describe CPU-local monitor snapshot
scope instead of whole-machine state, and IEMon documents both `save`
address operands as expression-capable start/end addresses.
Purpose judgement: These are user-facing contract defects. The manuals
must not tell IE32 programmers to rely on internal timer mirrors as a
stable MMIO ABI, must not present an IE64 legacy interrupt path that
guests cannot configure, must not overstate scripted snapshot scope, and
must accurately describe monitor command argument parsing.
Canonical sources checked: `cpu_ie32.go`, `cpu_ie64.go`,
`script_engine.go`, `debug_commands.go`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/iescript.md`,
`sdk/docs/iemon.md`, and `sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now checks that IE32 timer
offsets are described as internal/debug-visible backing-memory mirrors,
not guest bus/MMIO controls; that IE64 legacy timer delivery is not
documented as a guest programming pattern and points guests to the
CR7/ERET model; that IEScript state save/load calls are documented as
CPU-local `ss`/snapshot operations and point whole-machine users to
`rg`/`rt`; and that IEMon `save <start> <end> <file>` accepts
expressions for both address operands.
Action: Reworded `sdk/docs/IE32_ISA.md` section 7.1 and timer notes;
replaced the IE64 non-MMU timer example with a legacy host/test model
warning; reworded IEScript `dbg.save_state` and `dbg.load_state`;
corrected IEMon argument parsing matrix for `save`; added
`TestSDKCompanionDocs_IE32TimerMirrorsAreNotDocumentedAsGuestMMIO`,
`TestSDKCompanionDocs_IE64LegacyTimerPatternIsHostTestOnly`,
`TestSDKCompanionDocs_IEScriptStateSaveLoadIsCPULocal`, and
`TestSDKCompanionDocs_IEMonSaveArgumentParsingMatchesSource`.
Notes: The review findings are accepted. The earlier architecture
memory-map overlays, IEScript x86 JIT, IEMon `wr`/`wrw`, IE64 cause 11,
and IE64 Appendix C discoverability fixes remain covered by
`SDK-DOC-0043`.
Disposition: KEEP.

ID: SDK-DOC-0045
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/iescript.md`, and `sdk_doc_audit_test.go`.
Section: Source-backed MMIO map and IEScript memory-helper address-space
corrections.
Claim: Five additional source-parity defects are fixed and pinned by
regression checks: the TED video VRAM aperture is present in both memory
maps, the IE64 memory map includes the AHX MMIO block, the architecture
TED player row uses the source-mapped inclusive end and size, the media
staging buffer row uses an inclusive range, and IEScript memory helpers
distinguish focussed-CPU adapter access from raw 32-bit bus helpers.
Purpose judgement: These are shipped reference defects. Memory maps must
list mapped MMIO apertures and use the same ranges as `main.go` and the
constant files. Scripting docs must not blur CPU-adapter memory access
with raw bus helpers that truncate addresses to 32 bits.
Canonical sources checked: `main.go`, `ted_video_constants.go`,
`ted_constants.go`, `ahx_constants.go`, `media_loader_constants.go`,
`script_engine.go`, `sdk/docs/architecture.md`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/iescript.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now checks IE64 memory-map
coverage for AHX and TED VRAM, architecture memory-map coverage for TED
player, TED VRAM, and media staging ranges, and IEScript debug memory
helper wording that separates focussed CPU `read_mem`/`write_mem` from
raw shared-bus `fill_mem`/`hunt_mem`/`compare_mem`/`transfer_mem` with
`uint32` address truncation.
Action: Added `0xF3000-0xF6FFF` / 16KB TED VRAM aperture rows to
`sdk/docs/architecture.md` and `sdk/docs/IE64_ISA.md`; added
`$F0B80-$F0B91` AHX to the IE64 memory map; changed architecture TED
player to `0xF0F10-0xF0F1F` / 16B; changed media staging to
`0x800000-0x80FFFF`; documented raw 32-bit bus semantics and high-address
truncation for the relevant IEScript debug memory helpers; updated
`TestSDKCompanionDocs_IE64MemoryMapCoversSourceRanges`,
`TestSDKCompanionDocs_ArchitectureMemoryMapExplainsSharedReservations`,
and added
`TestSDKCompanionDocs_ArchitectureMemoryMapCoversTEDAndMediaSourceRanges`
and
`TestSDKCompanionDocs_IEScriptDebugMemoryHelpersDistinguishCPUAndRawBus`.
Notes: The review findings are accepted. No new source-backed issues were
found in `sdk/docs/IE32_ISA.md` or `sdk/docs/iemon.md` in this pass.
Disposition: KEEP.

ID: SDK-DOC-0046
Status: FIXED
Document: `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/architecture.md`, `sdk/docs/iescript.md`, and
`sdk_doc_audit_test.go`.
Section: Runtime memory-size contract, 32-bit script bus helpers, and
SID MMIO source ranges.
Claim: Three additional source-parity defects are fixed and pinned by
regression checks: IE32 backing memory is documented as the production
shared `MachineBus` memory sized at boot rather than the legacy 32 MiB
`NewMachineBus()` default, IEScript `mem.*` helpers are documented as
raw 32-bit shared-bus helpers rather than generic or above-4GiB IE64 RAM
accessors, and the architecture and IE64 memory maps use the SID1,
SID2, and SID3 apertures exposed by source.
Purpose judgement: These are shipped reference defects. The IE32 manual
must not present a test fallback as the runtime memory contract. The
IEScript manual and architecture overview must not imply that Lua
`mem.*` helpers address the entire IE64 physical space. SID rows in the
public memory maps must match the mapped constant ranges instead of
stale or collapsed apertures.
Canonical sources checked: `boot_guest_ram.go`, `machine_bus.go`,
`script_engine.go`, `sid_constants.go`, `main.go`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/architecture.md`, `sdk/docs/iescript.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now checks that IE32
documents production runtime memory as the shared boot-sized bus with
the 32 MiB default limited to `NewMachineBus()` fallback/test use; that
IEScript `mem.*` docs and the architecture scripting overview describe
raw 32-bit bus semantics, `uint32` address conversion, and
high-address truncation; and that SID1, SID2, and SID3 memory-map rows
match source-defined inclusive ranges.
Action: Reworded the IE32 address-space summary; added a `mem.*`
address-space contract paragraph and `uint32` wording to IEScript;
softened the architecture scripting overview from "entire bus" to the
shared 32-bit bus/MMIO surface plus CPU-adapter debugger APIs; changed
architecture SID1 to `0xF0E00-0xF0E1C` / 29B; split architecture SID2
and SID3 into `0xF0E30-0xF0E4C` / 29B and `0xF0E50-0xF0E6C` / 29B; and
added the same SID1/SID2/SID3 rows to the IE64 memory map.
Notes: The review findings are accepted. No new source-backed issues
were found in `sdk/docs/iemon.md` in this pass.
Disposition: KEEP.

ID: SDK-DOC-0047
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/iemon.md`, `sdk/docs/iescript.md`,
`sdk/docs/architecture.md`, and `sdk_doc_audit_test.go`.
Section: OS-owned material excluded from the five shipped manuals.
Claim: The five shipped SDK companion manuals do not mention IntuitionOS
or the OS kernel name. OS-specific ABI, syscall, kernel, loader,
fault-printer, and repository-migration material belongs outside this
five-book set.
Purpose judgement: The five shipped manuals are CPU, monitor,
scripting, and machine-architecture references. They must not become
IntuitionOS references or carry OS-specific names that will move to a
separate repository.
Canonical sources checked: `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/iemon.md`,
`sdk/docs/iescript.md`, `sdk/docs/architecture.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite rejects `IntuitionOS`,
`Intuition OS`, `Intuition-OS`, `IExec`, `` `iexec` ``, `GURU
MEDITATION`, and fault-printer phrasing in the ISA, monitor, and
scripting manuals. The architecture manual may mention IExec or
Intuition OS when that is appropriate whole-machine architecture
material. The suite also
rejects OS-scope wording in `iemon.md`, including the old IE64 fault
report heading, `operating-system`, `kernel`, `guest handler`,
`guest supervisor`, `AROS`, and `EmuTOS`.
Action: Removed the remaining OS-specific monitor fault-printer wording,
removed OS-specific diagnostic-report framing, rewrote the monitor
fault-interception section as CPU fault interception, rewrote the monitor
disassembly note to remove supervisor-software framing, changed the
whole-machine snapshot text from an AROS bridge reference to optional host
bridges, updated the five page-one last-modified dates to `2026-05-26`,
and added `TestSDKCompanionDocs_NoOSManualMaterialOutsideArchitecture` plus
`TestSDKCompanionDocs_IEMonIsHardwareMonitorNotOSManual`.
Notes: This entry supersedes earlier audit entries that folded
IntuitionOS ABI material into the IE64 ISA manual. That material is no
longer in scope for the ISA, monitor, or scripting manuals; the
architecture manual may name IExec or Intuition OS where appropriate for
whole-machine architecture. `SDK-DOC-0082`
supersedes the temporary reserved Program Executor label.
Disposition: KEEP.

ID: SDK-DOC-0048
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/iescript.md`,
`sdk/docs/iemon.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, and `sdk_doc_audit_test.go`.
Section: Source-backed reference corrections after ISA-scope cleanup.
Claim: Five additional source-backed defects are fixed and pinned by
regression checks: x86 compatibility bank/VRAM windows are documented as
source-backed overlays capped by `DEFAULT_MEMORY_SIZE`, IEScript's
monitor-parity `dbg.*` APIs have full reference entries rather than
quick-reference-only rows, IEMon names `ss`/`sl` as CPU-local snapshots
in the heading, the IE64 privilege summary includes the `MFCR Rd, CR6`
user-mode exception, and the IE32 OR row escapes the Markdown table pipe.
Purpose judgement: These are reader-facing reference defects. The
architecture manual must describe the actual x86 bus adapter, IEScript
must document exported APIs where reference readers look for argument and
return contracts, IEMon headings must not imply whole-machine snapshot
scope, the IE64 privilege summary must not contradict the detailed CR6
exception, and Markdown tables must render the intended instruction row.
Canonical sources checked: `cpu_x86_runner.go`, `script_engine.go`,
`debug_commands.go`, `sdk/docs/architecture.md`,
`sdk/docs/iescript.md`, `sdk/docs/iemon.md`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now checks x86 bank-window
source constants and `DEFAULT_MEMORY_SIZE` caps against architecture
wording, verifies detailed IEScript entries for `dbg.backtrace_frames`,
`dbg.tracering_*`, `dbg.source_at`, `dbg.history_*`, `dbg.device_*`,
`dbg.layout`, `dbg.bug_report`, and `dbg.help`, rejects the old IEMon
"Save/Load Machine State" heading, verifies the IE64 privilege summary's
CR6 exception, and rejects an unescaped pipe in the IE32 OR row.
Action: Reworded the x86 extension and bank-window architecture
sections; added full IEScript API reference entries with argument,
return-shape, field, nil, and error behaviour; renamed the IEMon
snapshot section to "CPU-Local Snapshot Save/Load"; qualified the IE64
user privilege summary; escaped the IE32 OR operation pipe; and added
`TestSDKCompanionDocs_ArchitectureX86BankWindowsMatchSource`,
`TestSDKCompanionDocs_IEScriptDbgMonitorParityAPIsAreFullyDocumented`,
`TestSDKCompanionDocs_IE32LogicalORTableEscapesMarkdownPipe`, plus
expanded existing IE64 and IEMon checks.
Notes: The review findings are accepted. The earlier ISA-scope issue was
not present in the current files: the IE64 and IE32 ISA manuals no
longer contain obvious platform-device or OS sections.
Disposition: KEEP.

ID: SDK-DOC-0049
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk_doc_audit_test.go`, and this ledger.
Section: Host RAM sizing, ISA tool-scope cleanup, and shared coprocessor
reservation wording.
Claim: Four additional review findings are fixed and pinned by
regression checks: the architecture intro describes platform-dispatched
usable-RAM detection instead of a global Linux `/proc/meminfo` contract,
the architecture coprocessor section no longer implies private per-core
memory regions, IE64 and IE32 ISA manuals no longer carry assembler-tool
or monitor-display reference sections, and the ISA instruction-reference
schema heading no longer falsely describes compact grouped tables as
Motorola-style per-instruction entries.
Purpose judgement: These are shipped reference defects. Architecture
must describe the source's Linux/Darwin/Windows RAM-sizing dispatch and
must preserve the shared-physical-map contract outside the memory-map
table. ISA manuals must remain CPU ISA references: instruction syntax,
encodings, addressing modes, exceptions, and CPU-visible behaviour stay
in scope, while tool invocation, directives, include policy, manifest
metadata, and monitor display conventions do not.
Canonical sources checked: `memory_sizing_usable_linux.go`,
`memory_sizing_usable_darwin.go`, `memory_sizing_usable_windows.go`,
`sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, and `sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now checks the
platform-specific RAM-sizing source paths and architecture wording,
checks shared coprocessor memory-reservation wording, rejects
assembler-tool and monitor-display material in both ISA manuals, and
expects the current ISA table-schema heading instead of a false
per-instruction entry-schema claim.
Action: Reworded the architecture intro; changed coprocessor worker
memory wording to "dedicated shared-memory reservation in the unified
physical map"; removed the IE64 assembler directive, macro,
conditional-assembly, repeat, expression, string-escape, and comment
quick-reference section from the ISA manual; removed IE32 assembler
tool, directive, expression, string, include/incbin, operand-splitting,
and IEMon disassembly material from the ISA manual; renumbered affected
IE64/IE32 sections; and added
`TestSDKCompanionDocs_ArchitectureRAMSizingNamesPlatformDispatch`,
`TestSDKCompanionDocs_ArchitectureCoprocessorMemoryReservationsAreShared`,
and `TestSDKCompanionDocs_ISADocsExcludeAssemblerToolAndMonitorMaterial`.
Notes: The review finding about full Motorola-style per-instruction
entries is accepted as a real structural issue. This pass removes the
false schema claim and stops the manuals from asserting compliance, but
it does not complete the larger per-instruction rewrite.
Disposition: KEEP.

ID: SDK-DOC-0050
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk_doc_audit_test.go`, and this ledger.
Section: IE32/IE64 ISA manual scope and per-instruction schema.
Claim: The IE32 and IE64 ISA manuals now read as physical CPU reference
manuals rather than emulator, debugger, assembler-tool, or platform
programming manuals.
Purpose judgement: The ISA manuals are shipped technical references for
the IE32 and IE64 CPUs. They must describe programmer-visible CPU state,
instruction encodings, operand forms, instruction behaviour, exception
semantics, timer/control-register semantics, addressing modes, and opcode
maps. They must not explain Go implementation files, interpreter/JIT
paths, host-test hooks, debugger rendering, monitor display conventions,
assembler directives, pseudo-instruction expansion, platform devices, or
future roadmap speculation.
Canonical sources checked: `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk_doc_audit_test.go`, `cpu_ie32.go`,
`cpu_ie64.go`, and `jit_exec.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite now passes and includes
guards that reject non-CPU-manual wording in both ISA manuals, require
section 4 to use per-instruction entries with Operation, Assembler
Syntax, Attributes, Description, Condition Codes, Instruction Format,
Instruction Fields, Exceptions, and Notes fields, and reject compact
opcode-table form inside the complete instruction-reference section.
Action: Converted the IE32 and IE64 complete instruction references from
compact opcode tables to per-instruction entries; changed the schema
heading back to "Instruction Entry Schema"; rewrote reset, timer,
interrupt, MMU, TLB, atomic, branch, and addressing prose to describe
CPU-visible behaviour; removed the IE64 pseudo-instruction section from
the ISA manual surface; removed or replaced Go/interpreter/JIT/debugger/
disassembler/host-test/current-implementation/future-roadmap wording; and
updated legacy timer wording to state that the non-MMU IE64 vector is not
a programmable interrupt-vector ABI.
Notes: Assembly-language syntax remains in scope only where it describes
instruction forms and encodings. The appendix opcode maps remain compact
summary maps; the per-instruction schema requirement applies to the
complete instruction-reference body.
Disposition: KEEP.

ID: SDK-DOC-0051
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk_doc_audit_test.go`, and this ledger.
Section: Physical-CPU ISA wording, IE64 atomic exceptions, and IE32
timer contract.
Claim: Four follow-up ISA-manual findings are fixed and pinned by tests:
IE64 atomic out-of-RAM fault behaviour now matches source, IE64 atomic
opcodes are documented as per-instruction entries, reserved opcode text
uses architectural stopped-state wording instead of console-output
phrasing, and IE32 timer/interrupt prose no longer exposes internal
field names.
Purpose judgement: The ISA manuals must read as physical CPU references.
They can document CPU-visible latches, counters, traps, stopped state,
instruction encodings, and exception causes, but not host memory
capacity, console printing, implementation field names, or grouped
instruction tables where the manual requires per-instruction entries.
Canonical sources checked: `cpu_ie64.go`, `cpu_ie32.go`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite passes and now checks that
IE64 atomics distinguish `FAULT_MISALIGNED` for unaligned/low-I/O-region
addresses from `FAULT_NOT_PRESENT` for translated ordinary-RAM bounds
failure; that CAS, XCHG, FAA, FAND, FOR, and FXOR each have full
per-instruction schema entries; and that both ISA manuals reject
host/emulator/internal wording such as host-memory capacity, error
printing, execution-loop halting, `cycleCounter`, `timerEnabled`,
`timerCount`, `timerPeriod`, `timerState`, `interruptEnabled`, and
`inInterrupt`.
Action: Rewrote IE64 atomic exception semantics to match `execAtomic`;
expanded the IE64 atomic section from a grouped table into six
per-instruction entries; replaced reserved-opcode console-output wording
with stopped-processor-state wording in both ISA appendices; removed the
IE64 host-memory-capacity phrase; and rewrote the IE32 timer section as
architectural timer state, sample-divider cadence, interrupt-enable
latch, and interrupt-active latch behaviour.
Notes: Appendix opcode maps remain compact summaries. The complete
instruction-reference body and the detailed atomic section use the
per-instruction schema.
Disposition: KEEP.

ID: SDK-DOC-0052
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk_doc_audit_test.go`, and this ledger.
Section: ISA placeholder-entry removal, timer divider wording, and
physical-CPU phrasing.
Claim: Five additional review findings are fixed and pinned by tests:
the IE64 generated duplicate placeholder entries are removed; both ISA
manuals reject the previous generated boilerplate phrases; the IE32 timer
divider is documented as the architectural value 44,100 instead of the
implementation symbol `SAMPLE_RATE`; IE64 no longer carries migration
phrasing about historical masks, transpiled programs, or the Intuition
Engine machine architecture; and the IE32 addressing-mode summary uses
architectural resolution wording instead of runtime wording.
Purpose judgement: A CPU ISA manual must not expose generated placeholder
text, implementation symbols, migration history, emulator runtime
phrasing, or user-facing entries that defer their content to surrounding
sections. The complete instruction-reference entries must be self-contained
enough to stand as CPU-reference entries.
Canonical sources checked: `cpu_ie64.go`, `cpu_ie32.go`,
`audio_chip.go`, `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite passes and now rejects
placeholder text such as `See section 3 for field definitions`, `See
instruction format and fields`, `according to the operand rules for this
instruction class`, `See the surrounding subsection`, `MOVE (reg)`,
`MOVE (imm)`, and bare generated operations such as `Operation: BRA`.
It also rejects ISA-manual physical-framing leaks including `historical`,
`transpiled programs`, `Intuition Engine machine architecture`, `Runtime
resolution`, `SAMPLE_RATE`, `normal execution loop`, diagnostic printing,
and running-flag language.
Action: Removed the IE64 duplicate placeholder instruction blocks;
rewrote the IE64 branch entries from bare mnemonic placeholders into
instruction-specific operation and description text; replaced generated
description and notes boilerplate in the ISA reference entries; changed
IE32 timer prose and transition tables to use 44,100 as the architectural
divider value; changed the IE32 addressing summary column to
"Architectural resolution"; removed IE64 historical-mask and transpiled
program wording; and rewrote remaining IE32 execution-loop/diagnostic
phrasing as stopped-state or architectural operand-resolution wording.
Notes: Source still defines the divider as `SAMPLE_RATE = 44100` in
`audio_chip.go`; the ISA manual intentionally documents the CPU contract
value rather than that implementation symbol.
Disposition: KEEP.

ID: SDK-DOC-0053
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk_doc_audit_test.go`, and this ledger.
Section: IE32 exception source truth and ISA manual physical-CPU wording.
Claim: Four follow-up ISA findings are fixed and pinned by tests: IE32
instruction entries no longer invent address, permission, alignment, or
bus fault behaviour; both ISA manuals reject generated `Executes ... using
the operands and fields named in this entry` description boilerplate; IE64
no longer uses historical/emulator-transition phrasing for the PC mask,
R30 reset state, host-integration apertures, or fixed-vector interrupt
path; and the IE32 timer divider is named as the architectural
`IE32_TIMER_DIVIDER` constant with value 44,100.
Purpose judgement: Source shows IE32 memory accesses below
`IO_REGION_START` use direct memory reads/writes with no architectural
bounds, alignment, permission, or bus fault path, while high addresses are
routed to the bus. IE32 stop conditions in source include divide-by-zero,
HALT, invalid opcode, and stack overflow/underflow, not a generalized
memory-fault model. ISA manuals must document those source-backed CPU
contracts and avoid API-doc boilerplate or emulator migration language.
Canonical sources checked: `cpu_ie32.go`, `cpu_ie64.go`, `audio_chip.go`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite passes and now checks the IE32
direct memory/bus paths and stop-condition strings in source before
rejecting invented IE32 memory-fault claims. It also rejects generated
description boilerplate, `legacy 25-bit`, `host-integration`,
`SAMPLE_RATE`, execution-loop/diagnostic/running-flag wording, and other
non-physical-CPU phrasing in the ISA manuals.
Action: Replaced IE32 generic memory-fault exception text with source-true
stopped-state wording for actual stop conditions; converted
generated `Executes ... using the operands and fields named in this entry`
descriptions to direct operation descriptions; rewrote IE64 overview,
reset, address-space, and timer-vector sections in present-tense CPU
terms; removed host-integration wording from ISA boundary statements; and
defined `IE32_TIMER_DIVIDER` as the architectural 44,100-instruction
divider.
Notes: IE64 retains source-backed MMU and memory fault language where the
CPU actually has an architectural trap model. IE32 normal instruction
entries omit non-existent memory fault classes; only source-backed
stopped-state conditions are called out.
Disposition: KEEP.

ID: SDK-DOC-0054
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk_doc_audit_test.go`, and this ledger.
Section: ISA entry quality, IE32 stop conditions, IE64 memory traps, and
IE32 timer prescaler wording.
Claim: The latest review findings are fixed and pinned by tests: both ISA
manuals now reject `Description: Performs ...` generated-description
boilerplate; IE32 entries no longer use awkward negative memory-fault
taxonomy; IE32 documents only source-backed stopped-processor conditions
for divide/modulo by zero, stack overflow/underflow, HALT, and reserved
opcodes; IE64 memory-operation exception text uses source-backed MMU trap
causes instead of a generic catch-all; and IE32 timer text uses
"timer prescaler" rather than "sample divider".
Purpose judgement: The ISA manuals must be CPU references, not generated
API summaries. Exception sections must document real CPU outcomes from the
source, and timer terminology must define the CPU contract without leaking
audio/sample-rate implementation names.
Canonical sources checked: `cpu_ie32.go`, `cpu_ie64.go`, `mmu_ie64.go`,
`audio_chip.go`, `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run 'TestSDK' ./...`.
Observed result: The focused SDK test suite passes and now rejects
generated `Description: Performs ...` prose, generated operand boilerplate,
invented IE32 memory-fault claims, negative IE32 no-fault taxonomy,
`SAMPLE_RATE`, and sample-divider wording in the ISA docs. The IE32
source check still anchors direct memory access, bus access, divide-by-zero,
and invalid-opcode stop conditions.
Action: Replaced generated description boilerplate with direct
architectural-effect descriptions; changed normal IE32 entry exceptions to
`None`; added source-backed stopped-state exceptions to DIV, MOD, stack,
RTI, HALT, and reserved-opcode documentation; changed IE64 memory-operation
exceptions to enumerate MMU cause codes 0/1/2/10 and translated-physical
not-present behaviour; and renamed IE32 timer divider prose to timer
prescaler while keeping the `IE32_TIMER_DIVIDER = 44,100` contract.
Notes: Further manual polish can make individual descriptions more
literary, but the audit now rejects the specific generated and source-false
patterns identified in this pass.
Disposition: KEEP.

ID: SDK-DOC-0055
Status: FIXED
Document: `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`, and
`sdk_doc_audit_test.go`.
Section: ISA instruction-entry exceptions and physical-CPU reference
voice.
Claim: IE32 load-family instructions do not stop on a zero divisor;
only DIV and MOD stop when their resolved divisor is zero. IE64 HALT
enters the stopped processor state without advancing PC. Instruction
entries should not use generated `Architectural effect`, `processor
applies`, or generic `None specific to this entry` placeholder prose.
Purpose judgement: The ISA manuals are CPU reference manuals. Exception
rows must be tied to actual CPU outcomes in source, and entry prose must
read as instruction-specific CPU behaviour rather than generated API
summaries.
Canonical sources checked: `cpu_ie32.go`, `cpu_ie64.go`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`.
Observed result: The focused audit passes and now checks that LOAD, LDA,
LDX, and LDY have `Exceptions: None`; DIV and MOD carry the zero-divisor
stopped-processor condition; IE64 HALT documents stopped state,
non-advanced PC, and no trap; and generated placeholder description and
exception phrases are rejected from the ISA instruction-reference
sections.
Action: Moved the IE32 zero-divisor exception text off the load-family
entries and onto DIV/MOD; replaced generic exception placeholders with
explicit `None` where no instruction-specific exception exists; rewrote
the IE64 HALT entry to document stopped-PC behaviour from source; and
tightened generated-prose audit gates.
Notes: This entry closes the source-truth error where LOAD/LDA inherited
DIV/MOD exception text and records the additional manual-quality gates
added after the previous pass.
Disposition: KEEP.

ID: SDK-DOC-0056
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Section: Processor-manual framing, instruction fields, IE64 memory
exceptions, and HALT semantics.
Claim: The ISA manuals are processor user's manuals, not engine or VM
specifications. Instruction-field rows must describe encoded byte
fields rather than repeating opcode/syntax/memory-class summaries. IE64
LOAD entries list read faults only, STORE entries list write faults
only, and HALT enters the stopped processor state without advancing PC
or generating a trap.
Purpose judgement: Experienced CPU-reference readers treat each entry
as an instruction-level contract. Directionless memory-fault boilerplate,
weak field summaries, and VM-style titles are not acceptable in a
physical-CPU-style manual.
Canonical sources checked: `cpu_ie64.go`, `mmu_ie64.go`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`.
Observed result: The focused audit passes and now checks that IE64/IE32
titles are processor user's manuals; generated `Instruction Fields:
opcode = ...; syntax = ...` rows are rejected; IE64 LOAD and STORE
entries have direction-specific MMU exception text; and IE64 HALT
documents stopped state, non-advanced PC, and no generated trap.
Action: Retitled the ISA books as processor user's manuals; rewrote
instruction-field rows into byte-field layouts; split IE64 read and
write memory exceptions by instruction direction; changed IE64 HALT
exceptions to `None. No trap is generated`; and moved HALT PC behaviour
into notes for both ISA manuals.
Notes: This pass closes the remaining source-truth issue in the IE64
load/store exception rows and adds audit gates for the Motorola-style
schema-quality failures identified in review.
Disposition: KEEP.

ID: SDK-DOC-0057
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Section: IE64 MMU/privilege instruction entries, fixed-form IE64
fields, IE64 DIVS width semantics, and IE32 stopped-condition wording.
Claim: IE64 MMU and privilege opcodes are real ISA instructions and
must be documented with the same per-instruction schema as the rest of
the manual. Fixed-form instructions such as NOP and HALT must state that
unused bytes are reserved/ignored. DIVS must document its full-width
signed division before result masking, contrasting MODS selected-width
sign extension. IE32 must describe stopped-processor conditions, not an
architectural trap model.
Purpose judgement: These are processor user's manuals. Summary tables,
generic field prose for operandless opcodes, and trap terminology that
does not exist in source weaken the instruction-level contract.
Canonical sources checked: `cpu_ie64.go`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, and `sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`.
Observed result: The focused audit passes and now checks full-schema
entries for MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE,
SUAEN, and SUADIS; reserved-byte field text for IE64 NOP and HALT;
full-width signed `DIVS` semantics against source; and the IE32 schema's
stopped-processor wording.
Action: Replaced the IE64 MMU/privilege summary table and encoding
block with nine Motorola-style instruction entries; rewrote NOP and
HALT field descriptions to reserve bytes 1-7; documented the DIVS/MODS
signed-width distinction; changed IE32's schema wording from
"Stop conditions or traps" to stopped-processor conditions; and added
source-anchored audit gates for those contracts.
Notes: This entry closes the missing-schema defect for IE64
MMU/privilege instructions and the remaining fixed-form/generic-field
issues identified in the review.
Disposition: KEEP.

ID: SDK-DOC-0058
Status: FIXED
Document: IE64 ISA and IE32 ISA.
Section: IE64 system instructions; IE32 instruction schema and address
conventions.
Claim: Fixed-form processor instructions must describe only the fields
they actually encode, IE32 memory-indirect stores must document their
source-backed double-indirect behavior, and the IE32 processor manual
must not define a loader/programme-loading contract.
Purpose judgement: The ISA manuals are processor user's manuals written
as if IE64 and IE32 are physical CPUs. Generic operand-field prose on
operandless opcodes, bad cross-references, and loader contracts are
not acceptable instruction-set reference material.
Canonical sources checked: `cpu_ie64.go` cases `OP_SEI64`,
`OP_CLI64`, `OP_RTI64`, and `OP_WAIT64`; `cpu_ie32.go` cases `NOP`
and `HALT`; IE32 `resolveOperand` and `storeRegister`; and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`.
Observed result: The focused audit passes and now checks reserved-byte
field text for IE64 SEI, CLI, RTI, and WAIT; reserved-byte field text
for IE32 NOP and HALT; source-backed IE32 memory-indirect store
semantics; and absence of loader/programme-loading contract language
from the IE32 ISA manual.
Action: Replaced generic IE64 SEI/CLI/RTI/WAIT field descriptions with
fixed-form field descriptions, documenting WAIT's `imm32` field only;
replaced generic IE32 NOP/HALT field descriptions with fixed-form
reserved-byte descriptions; replaced the bad section 10.3
memory-indirect store cross-reference with the actual pointer-read then
write behavior; and rewrote IE32 address-convention wording to describe
reset `PC`, initial `SP`, stack boundaries, and CPU access width without
loader contract language.
Notes: This entry closes the fixed-form system-instruction and IE32
addressing/scope findings from the review.
Disposition: KEEP.

ID: SDK-DOC-0059
Status: FIXED
Document: IE64 ISA and IE32 ISA.
Section: IE64 `MOVT`; IE32 `DIV`/`MOD`; IE32 address-space conventions.
Claim: Instruction-field descriptions must name only fields used by the
specific instruction, and an ISA processor manual must not describe
loader staging conventions as CPU address-space properties.
Purpose judgement: The ISA manuals are processor user's manuals. Field
entries that mention source registers, immediate/register selectors,
displacements, or branch targets for instructions that do not encode
those fields are misleading CPU reference material. Loader staging is
tooling/helper behaviour, not an IE32 CPU property.
Canonical sources checked: `cpu_ie64.go` case `OP_MOVT`;
`cpu_ie32.go` cases `DIV` and `MOD`; `cpu_ie32.go`
`LoadProgramBytes`; and `sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`.
Observed result: The focused audit now checks source-backed IE64 `MOVT`
field text, rejects generic `Rs`/`Rt`/selector/displacement prose from
that entry, checks concrete IE32 `DIV` and `MOD` field text, rejects
branch-target prose in those arithmetic entries, and rejects
hyphenated loader/programme-load contract language from the IE32 ISA
manual.
Action: Rewrote the IE64 `MOVT` instruction fields to document only
opcode, `Rd`, reserved bytes, and `imm32`; rewrote IE32 `DIV` and `MOD`
instruction fields to document the register, addressing mode, reserved
byte, and `operand32` interpretation; and removed the remaining
programme-load wording from the IE32 address-space section.
Notes: This entry closes the specific source-backed field-schema and
ISA/tooling-boundary findings from this pass. The manual-wide generic
field-prose cleanup and hard-gate rules are recorded in `SDK-DOC-0060`.
Disposition: KEEP.

ID: SDK-DOC-0060
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md`, and
`sdk_doc_audit_test.go`.
Section: IE ISA manual hard gate and manual-wide instruction-field
schema.
Claim: The IE32 and IE64 ISA manuals must document CPU ISA material only
and every instruction entry must use exact, source-backed field prose.
Purpose judgement: These manuals are physical processor reference
manuals for experienced software engineers. Generic field templates,
branch-target leakage into non-branch IE32 entries, loader/helper
contracts, platform-device wording, and architecture-manual handoffs make
the manuals read as generated emulator documentation rather than CPU
reference manuals.
Canonical sources checked: `cpu_ie64.go`, `cpu_ie32.go`,
`sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`.
Observed result: The focused audit passes and now requires the ledger to
contain the IE ISA Manual Hard Gate. It rejects generated instruction
field phrases such as `where applicable`, `when the form uses`, `first
encoded register field`, and `operand32 or branch target` from
instruction entries, and checks the new CPU-only atomic wording.
Action: Added the IE ISA Manual Hard Gate to this ledger; tightened the
IE64 and IE32 purpose statements; made the completion criteria require
the ISA hard gate; replaced broad generic IE64 and IE32 instruction-field
templates with concrete per-entry field text; removed IE32
architecture-manual handoff wording; and rephrased IE64 atomic
restrictions as CPU address-space trap behaviour.
Notes: The Markdown and test changes happened after the prior PDF render
entry, so that render was provisional. The final PDF render gate was
repeated in `SDK-DOC-0031` after this entry and `SDK-DOC-0061`.
Disposition: KEEP.

ID: SDK-DOC-0061
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/iemon.md`, `sdk/docs/iescript.md`,
`sdk/docs/architecture.md`, and
`sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md`.
Section: Current-run re-execution after updated ledger hard gates.
Claim: The updated five-manual audit plan was rerun after the latest
manual and ledger edits, including the ISA hard gate, closed-reference
gate, page-date gate, dash-style gate, and focused source-backed
regression tests.
Purpose judgement: A rerun after hard-gate edits must not rely on the
previous PDF render or on stale pass results. The manuals must be checked
again as the current files on disk, and the final PDF render must happen
after the rerun evidence is recorded.
Canonical sources checked: `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/iemon.md`,
`sdk/docs/iescript.md`, `sdk/docs/architecture.md`,
`sdk/docs/verify/SDK_DOC_AUDIT_LEDGER.md`, and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`; focused text gates for
forbidden dash characters, unshipped document references, current
page-one modified dates, forbidden ISA generic field prose, and
physical-processor ISA scope phrases; `git diff --check`; and the final
PDF render command recorded in `SDK-DOC-0031`.
Observed result: The focused audit and hard-gate text checks passed on
the current files. No new source-backed manual defect was found in this
rerun; the only blocking run-state issue was that the prior PDF render
preceded the latest ledger hard-gate changes.
Action: Recorded this rerun evidence, changed the current run state from
open to fixed, and reran the final PDF render gate after the ledger
change.
Notes: If any later manual, source, source-comment, generated-table,
test, or ledger hard-gate change is made, this rerun evidence is stale
and the final PDF render gate must be repeated.
Disposition: KEEP.

ID: SDK-DOC-0062
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Section: IE64 load/store width prose, IE64 FPU instruction entries, and
IE32 store destination semantics.
Claim: The current manuals must not leak platform bus ownership into the
IE64 ISA memory-width description; IE64 FPU instruction entries must not
use placeholder attribute text or hide invalid FP-register encodings in
section-level prose only; IE32 store notation must distinguish read
operand resolution from store destination resolution because stores do
not write architectural registers.
Purpose judgement: These are processor reference manuals. The IE64 text
must describe CPU-visible transfer semantics and CPU memory/MMU faults,
not platform aperture layout. The FPU entries must be concrete enough for
an experienced reader to determine operand size, memory access,
privilege, FP register constraints, FPSR/FPCR effects, and invalid
encoding behaviour from the entry itself. The IE32 store model must match
the CPU's write path instead of suggesting that store operands can select
a destination register.
Canonical sources checked: `cpu_ie64.go` FPU dispatch and invalid-freg
stop path; `fpu_ie64.go` FPSR/FPCR side effects; `cpu_ie32.go`
`resolveOperand` and `storeRegister`; `sdk/docs/IE64_ISA.md`; and
`sdk/docs/IE32_ISA.md`.
Runnable verification: `go test -tags headless -run
'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`, plus focused text
gates for the forbidden phrases `Platform bus routing`, `Operand and
memory attributes are defined`, `destination register or 32-bit memory
location`, shipped-document `IntuitionOS`/GURU references, and forbidden
dash characters.
Observed result: The manuals now describe `.Q` as a 64-bit
little-endian CPU transfer whose validity and fault behaviour come from
CPU memory/MMU rules. The IE64 FPU entries have concrete Attributes
lines and per-entry invalid FP-register or odd double-register stopped
processor conditions where source enforces them. The IE32 schema now
separates `resolve(operand)` for read operands from `store(value,
operand)` for memory destinations; the store entries state that stores
write destination memory and that immediate, direct, and register modes
use `operand32` as the destination address.
Action: Updated the two ISA Markdown manuals and added audit tests to
pin the CPU-scope, FPU-attribute, invalid-FP-encoding, and IE32
store-destination rules.
Notes: Because this entry changes shipped Markdown, audit tests, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0063
Status: FIXED
Document: `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
`cpu_ie32.go`, `cpu_ie32_instruction_test.go`, and
`sdk_doc_audit_test.go`.
Section: IE32 stack/branch/single-step semantics and IE64 CPU-reference
wording.
Claim: IE32 `PUSH` and `POP` entries must document the same stopped
processor conditions as the execution path; IE32 single-step execution
must not contradict the ISA manual for absolute branch targets or
`INC`/`DEC` operand destinations; IE64 memory-width and `BSWAP`
wording must avoid implementation API and branch-only boilerplate.
Purpose judgement: The ISA manuals are physical-CPU-style processor
reference manuals. A debugger single-step path that implements different
architectural behavior makes source-truth verification impossible, and
Go method names or platform-scope phrasing do not belong in a CPU
transfer-width table.
Canonical sources checked: `cpu_ie32.go` `Execute`, `StepOne`,
`Push`, `Pop`, `resolveOperand`, and `storeRegister`;
`sdk/docs/IE32_ISA.md`; `sdk/docs/IE64_ISA.md`; and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestIE32StepOneMatchesExecuteForINCDECMemoryTargets|TestIE32StepOneUsesAbsoluteBranchOperand|TestIE32StepOneStackBoundsMatchExecute|TestSDKCompanionDocs'
.`, plus focused scans for `platform peripheral addresses`, `Bus
Method`, `Read8/Write8`, and related Go memory-method names in the two
ISA manuals.
Observed result: `PUSH` now documents stack overflow as a stopped
processor condition and `POP` documents stack underflow. `StepOne` now
uses the same destination semantics as `Execute` for stores and
`INC`/`DEC`, uses `operand32` as the absolute target for `JMP`, `JNZ`,
`JZ`, and `JSR`, stops on divide/modulo by zero, and uses `Push`/`Pop`
for stack bounds. IE64 memory-width prose now uses architectural
transfer wording instead of `Read8`/`Write8` method names, and `BSWAP`
uses the normal non-branch condition-code wording.
Action: Updated IE32 source, IE32 StepOne regression tests, the IE32 and
IE64 ISA manuals, and audit tests that pin CPU-only IE32 address-space
wording plus the absence of IE64 Go memory-method labels.
Notes: Because this entry changes shipped Markdown, source, tests, and
the ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0064
Status: FIXED
Document: `sdk/docs/IE32_ISA.md`, `sdk/docs/IE64_ISA.md`,
`cpu_ie32.go`, `cpu_ie32_instruction_test.go`, and
`sdk_doc_audit_test.go`.
Section: IE32 single-step source-truth consistency and ISA
condition-code prose.
Claim: IE32 single-step execution must match the architectural behaviour
documented by the ISA manual and implemented by the main execution loop
for signed branches, `NOT`, `RTI`, and shift counts. ISA instruction
entries must state the actual condition-code effect instead of using
branch-decision filler on control-transfer instructions.
Purpose judgement: The ISA manuals are physical processor reference
manuals, so a second CPU execution path cannot expose different
architectural semantics. A Motorola-style Condition Codes field states
which condition-code state changes, not how a branch chooses its target.
Canonical sources checked: `cpu_ie32.go` `Execute`, `StepOne`, `Pop`,
signed branch dispatch, `NOT`, `SHL`, `SHR`, and `RTI`;
`sdk/docs/IE32_ISA.md`; `sdk/docs/IE64_ISA.md`; and
`sdk_doc_audit_test.go`.
Runnable verification: `go test -tags headless -run
'TestIE32StepOneMatchesExecuteForINCDECMemoryTargets|TestIE32StepOneUsesAbsoluteBranchOperand|TestIE32StepOneSignedBranchesCompareAgainstZero|TestIE32StepOneNOTIgnoresOperandFields|TestIE32StepOneShiftCountsAreUnmasked|TestIE32StepOneStackBoundsMatchExecute|TestSDKCompanionDocs_ISADocsStayCPUScope'
.`, plus a focused shipped-document scan for the old branch-decision
condition-code phrase and existing ISA-scope forbidden phrases.
Observed result: `StepOne` now compares `JGT`, `JGE`, `JLT`, and `JLE`
against zero like `Execute`; `NOT` inverts the selected register and
ignores operand fields; `SHL` and `SHR` use the resolved shift count
directly; and `RTI` uses `Pop` so stack underflow enters the stopped
processor state. The IE32 and IE64 ISA entries now use direct
condition-code wording, and the audit test rejects the old
branch-decision filler phrase.
Action: Updated IE32 source, IE32 StepOne regression tests, the IE32 and
IE64 ISA manuals, and the ISA CPU-scope audit gate.
Notes: Because this entry changes shipped Markdown, source, tests, and
the ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0065
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/iemon.md`,
`cpu_ie64.go`, `cpu_ie32.go`, `cpu_ie64_test.go`, and
`sdk_doc_audit_test.go`.
Section: IE64 JSR form wording, IE64 stack access failure semantics,
and monitor single-step `WAIT` behaviour.
Claim: The IE64 `JSR` summary must identify opcode `0x50` as the
PC-relative subroutine-call form. IE64 stack instruction entries must
document both MMU translation traps and the source-backed stopped state
for a physical stack access outside CPU-visible backing. The broken
indirect `JSR` heading must render cleanly. The monitor/debug
single-step path may skip `WAIT` delays only if that contract is stated
outside the ISA manuals.
Purpose judgement: The IE64 ISA manual is a physical processor
reference, so instruction entries must describe CPU-visible semantics
and not leave stack failures as MMU-only text. The monitor manual owns
debugger single-step behaviour; the ISA manuals continue to describe
normal processor execution.
Canonical sources checked: `cpu_ie64.go` `Execute`, `StepOne`,
`mmuStackWrite`, `mmuStackRead`, `OP_JSR64`, `OP_JSR_IND`, `OP_RTS64`,
`OP_PUSH64`, `OP_POP64`, `OP_RTI64`, and `OP_WAIT64`;
`machine_bus_phys.go` `ReadPhys64WithFault` and
`WritePhys64WithFault`; `cpu_ie32.go` `WAIT` and `StepOne`;
`assembler/ie64asm.go`; `sdk/docs/IE64_ISA.md`; and
`sdk/docs/iemon.md`.
Runnable verification: `go test -tags headless -run
'TestIE64StepOneStackOutOfBackingDoesNotAdvancePC|TestSDKCompanionDocs_IE64StackAndJSRReferenceText|TestSDKCompanionDocs_ISADocsStayCPUScope|TestSDKCompanionDocs_IE64FixedFormInstructionsDocumentReservedBytes'
.`, plus focused shipped-document scans for the old absolute-`JSR`
wording, broken indirect-`JSR` heading, `IntuitionOS`, `GURU`, and
forbidden dash characters.
Observed result: The IE64 `JSR` summary now calls opcode `0x50`
PC-relative. The `JSR`, `RTS`, `PUSH`, `POP`, indirect `JSR`, and `RTI`
entries now state MMU trap causes and the non-trap stopped state for an
out-of-backing physical stack access. The indirect `JSR` heading now
uses balanced Markdown code spans. IE64 single-step stack access now
preserves stopped-PC behaviour for out-of-backing `PUSH` and `POP`, and
the monitor manual states that single-stepping `WAIT` advances one
instruction without consuming the requested real-time delay.
Action: Updated IE64 source, IE64 regression tests, IE64 ISA reference
text, IEMon single-step documentation, and audit tests pinning the JSR
and stack exception wording.
Notes: Because this entry changes shipped Markdown, source, tests, and
the ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0066
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk/docs/architecture.md`, `assembler/ie64asm.go`,
`assembler/ie64asm_test.go`, and `sdk_doc_audit_test.go`.
Section: IE64 control-register syntax, timer step timing, TLB wording,
JMP heading formatting, and `.l`-only quick-reference syntax.
Claim: The IE64 manual must not document assembler-facing CR15 syntax
that the assembler rejects; examples must use `mtcr CRn, Rs` order and
parseable CR operands; the illegal-instruction cause table must use the
source operand order; IE32 and IE64 timer wording must describe the
actual timer-step point after fetch/decode or operand resolution and
before instruction execution; the IE64 `JMP` heading must render cleanly;
`.l`-only instructions must show the required suffix in the summary; and
the TLB section must describe the translation and permission data stored
by source, not a full PTE.
Purpose judgement: The ISA manuals are physical processor manuals with
assembler syntax, so every example and quick-reference form must be
usable and source-backed. Timer wording must be precise enough for
interrupt timing. Architecture must agree with the same CPU-visible
timer timing model.
Canonical sources checked: `assembler/ie64asm.go` `parseCR`, `asmMTCR`,
`asmMFCR`, and `.l`-only ALU dispatch; `cpu_ie64.go` timer step,
`OP_MTCR`, `OP_MFCR`, and `CR_RAM_SIZE_BYTES`; `cpu_ie32.go` timer
prescaler step; `jit_exec.go` IE64 block timer decrement; and
`mmu_ie64.go` `TLBEntry`.
Runnable verification: `GOCACHE=/tmp/ie-go-build-cache go test -tags
headless -run
'TestSDKCompanionDocs_ArchitectureTimerCadenceMatchesSource|TestSDKCompanionDocs_IE64TimerContractLabelsImplementationFields|TestSDKCompanionDocs_IE32TimerDividerIsArchitecturalValue|TestSDKCompanionDocs_IE64ControlRegisterSyntaxAndMMUText|TestSDKCompanionDocs_IE64JumpAndLOnlyQuickReferenceSyntax|TestSDKCompanionDocs_ISADocsStayCPUScope'
.`, and `GOCACHE=/tmp/ie-go-build-cache go test -tags 'headless ie64'
-run 'TestIE64Asm_(MFCR|CRNames)$' ./assembler`, plus focused
shipped-document scans for stale CR, timer, TLB, JMP-heading, `.l`
suffix, OS, and dash wording.
Observed result: The assembler now accepts `cr15` and `ram_size_bytes`.
The IE64 examples use `mtcr cr8`, `mtcr cr7`, `mtcr cr9`, and
`mtcr cr11`; the illegal-instruction cause table uses `MTCR
RAM_SIZE_BYTES, Rs`; the IE64 and IE32 timer sections and architecture
timer section describe decoded-instruction timer steps instead of
retirement/executed-instruction wording; the IE64 `JMP` heading renders
with balanced code spans; the opcode summary lists `CLZ.l`, `CTZ.l`,
`POPCNT.l`, and `BSWAP.l`; and the TLB section documents cached VPN,
PPN, leaf-PTE address, and flags.
Action: Updated IE64 assembler source and tests, the IE64 and IE32 ISA
manuals, architecture timer prose, and audit tests pinning these
source-backed contracts.
Notes: Because this entry changes shipped Markdown, source, tests, and
the ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0067
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Section: IE64 complete instruction-reference coverage, timer expiry
order, FPU constants and compare results, TLB wording, and IE32 timer
state-transition wording.
Claim: The IE64 Complete Instruction Reference must include every
source-defined opcode as a first-class Motorola-style entry with
continuous entry numbering; MMU/privilege and atomic opcodes must not
live only in later narrative sections; IE64 timer expiry must document
reload before interrupt dispatch; FMOVECR constant 14 must name the
smallest positive FP32 subnormal value rather than ambiguous `FLT_MIN`;
FCMP/DCMP unordered comparisons must state that the integer result is
zero; TLB prose must describe architectural translation caching rather
than a software implementation structure; and IE32 timer transition
wording must not say an enabled timer "executes" an instruction.
Purpose judgement: The ISA manuals are physical processor manuals. The
instruction reference has to be complete from a source-derived opcode
inventory, not from local manual skim checks. Timer and FPU wording must
describe exact CPU-visible state transitions and results.
Canonical sources checked: `cpu_ie64.go` opcode constants, timer
decrement/reload/interrupt order, MMU/privilege dispatch, and atomic
dispatch; `fpu_ie64.go` `FCMP`, `DCMP`, and `ie64FmovecrROMTable`;
`mmu_ie64.go` TLB entry shape; `assembler/ie64asm.go` opcode forms; and
`cpu_ie32.go` IE32 timer step location.
Runnable verification: `GOCACHE=/tmp/ie-go-build-cache go test -tags
headless -run 'TestSDKCompanionDocs|TestSDKDocAuditLedger' .`, plus
source/document scans for stale timer, TLB, FMOVECR, generic-summary,
OS, and dash wording.
Observed result: IE64 section 4 now contains first-class entries for
MTCR, MFCR, ERET, TLBFLUSH, TLBINVAL, SYSCALL, SMODE, CAS, XCHG, FAA,
FAND, FOR, FXOR, SUAEN, and SUADIS; later MMU and atomic chapters now
define shared state and semantics instead of owning the instruction
entries. IE64 instruction-entry numbering is continuous from 1 through
129. IE64 timer expiry now reloads TIMER_COUNT before interrupt handler
dispatch. FMOVECR #14 names the exact `0x00000001` smallest positive
FP32 subnormal value. FCMP and DCMP unordered cases state that `rd`
receives zero. IE32 timer state transitions describe completion of
operand resolution before instruction-body execution.
Action: Updated the IE64 and IE32 ISA manuals and added audit tests that
diff the IE64 complete reference against source opcodes, reject
instruction-number gaps, pin timer reload-before-dispatch ordering, pin
FPU unordered result and constant wording, and reject the stale IE32
timer transition row.
Notes: Because this entry changes shipped Markdown, tests, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0068
Status: FIXED
Document: `cpu_ie64.go`, `cpu_ie32.go`, and
`sdk_isa_inventory_test.go`.
Section: Source-comment correctness under the empirical ISA inventory
gate.
Claim: Source comments are not canonical, but incorrect source comments
must be fixed when they contradict the empirically verified ISA manual
contract. IE64 timer comments must describe decoded-instruction timer
steps and reload-before-interrupt-dispatch ordering. IE32 WAIT source
comments must describe microseconds, not cycles.
Purpose judgement: The source-only inventory gate should not let stale
comments survive beside executable source facts. Even though comments do
not define the generated inventory, misleading comments weaken future
source-first audits and should be pinned by regression tests.
Canonical sources checked: `cpu_ie64.go` timer decrement/reload/interrupt
sequence and CR9 timer-period storage; `cpu_ie32.go` WAIT execution path;
and the generated source-only ISA inventory test.
Runnable verification: `GOCACHE=/tmp/ie-go-build-cache go test -tags
headless -run
'TestSDKISAInventory|TestSDKISAAuditLedger|TestSDKCompanionDocs_(IE64|IE32).*Opcode|TestSDKCompanionDocs_IE64Control|TestSDKCompanionDocs_ISA'
.`
Observed result: `cpu_ie64.go` now says `CR_TIMER_PERIOD` is in
decoded-instruction timer steps and that TIMER_COUNT reloads from
TIMER_PERIOD before any enabled interrupt is dispatched. `cpu_ie32.go`
now says WAIT uses microseconds. `TestSDKISAInventoryRejectsStaleSourceComments`
rejects the stale comment phrases.
Action: Updated source comments and added an empirical inventory test
for these known stale source-comment contracts.
Notes: Because this entry changes source comments, tests, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0069
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`, and
`sdk_doc_audit_test.go`.
Section: Physical-CPU ISA prose scope for stack faults, target fields,
displacement fields, and double-FP register-pair operands.
Claim: The ISA manuals must describe encoded CPU behaviour, not
assembler lowering, emulator storage, or source-form handling. IE64
stack access failures after non-trapping physical access must be worded
as physical stack accesses outside implemented CPU-visible memory
entering the stopped processor state without creating a trap frame. IE32
branch and subroutine targets must be described as encoded absolute
32-bit operand fields. IE64 PC-relative and displacement control-flow
forms must describe the encoded signed `imm32` field directly. IE64
double-precision FPU operands must be described as even-numbered F
register-pair encodings, not assembler syntax behaviour.
Purpose judgement: These are physical processor manuals. Assembler
accepted forms, emitted modes, and storage/backing implementation terms
belong outside instruction-level ISA prose.
Canonical sources checked: `cpu_ie64.go` stack access paths through
`mmuStackRead`/`mmuStackWrite`, PC-relative `JSR`, register-displacement
`JMP`/`JSR`, and double-FP register-pair checks; `cpu_ie32.go` branch
and `JSR` target handling; and existing ISA audit tests.
Runnable verification: `GOCACHE=/tmp/ie-go-build-cache go test -tags
headless -run
'TestSDKISAInventory|TestSDKISAAuditLedger|TestSDKCompanionDocs_(IE64|IE32).*Opcode|TestSDKCompanionDocs_IE64Control|TestSDKCompanionDocs_ISA|TestSDKCompanionDocs_IE64StackAndJSRReferenceText|TestSDKCompanionDocs_IE64MemoryEntriesUseDirectionSpecificFaults'
.`, plus shipped-manual scans for assembler-lowering and backing-storage
phrases.
Observed result: IE64 stack entries now use implemented
CPU-visible-memory stopped-state wording. IE64 load/store exception text
uses the same implemented-memory wording instead of mapped backing.
IE32 branch/subroutine target prose now describes encoded operand fields.
IE32 bare-expression prose now describes immediate-mode encodings.
IE64 addressing-mode prose now says PC-relative forms consume encoded
signed `imm32`; displacement forms consume `imm32` directly. IE64 FPU
prose now says double-precision encodings name the even F register of
the pair.
Action: Updated both ISA manuals and extended the physical-CPU reference
voice test to reject the leaked phrases.
Notes: Because this entry changes shipped Markdown, tests, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0070
Status: FIXED
Document: `sdk_source_inventory_test.go`,
`sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`, and
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`.
Section: Empirical source inventories for IEMon, IEScript, and
architecture.
Claim: The five-manual control plan must not merely name empirical
inventories. IEMon, IEScript, and architecture inventories must exist,
be generated or mechanically checked from source, golden-compare against
current source facts, and have positive manual-coverage gates.
Purpose judgement: The upgraded ledger made empirical inventories a
stable control for all five manuals. Leaving only the ISA inventory on
disk would let future audit passes certify the plan text while skipping
the monitor, scripting, and architecture source surfaces.
Canonical sources checked: `debug_commands.go` `monitorHelpRegistry`
and `executeCommand` dispatch cases; `script_engine.go`
`registerModules`; and root Go files classified by architecture
subsystem prefix for bus/RAM, CPU, JIT, video, audio, debug monitor,
scripting, file/media, and snapshot surfaces.
Runnable verification: `GOCACHE=/tmp/ie-go-build-cache go test -tags
headless -run
'TestSDKDocAuditLedger|TestSDKISAInventory|TestSDKCompanionDocs|TestSDK(IEMon|IEScript|Architecture)'
.`
Observed result: Added source-derived golden inventories for IEMon
commands and dispatch aliases, IEScript Lua bindings, and architecture
source categories. Added tests that regenerate and compare the
inventories, require the inventory files named by the control plan to
exist, and compare `iemon.md`, `iescript.md`, and `architecture.md`
against those generated facts. The focused SDK document gate passes.
Action: Added `sdk_source_inventory_test.go` and the three missing
empirical inventory files.
Notes: Because this entry changes inventory files, tests, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0071
Status: FIXED
Document: `sdk_source_inventory_test.go`,
`sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`, `sdk/docs/iemon.md`, and
`sdk/docs/iescript.md`.
Section: Empirical source inventory hardening after the five-manual
plan upgrade.
Claim: The empirical inventories must cover source-backed public
surfaces and claims, not only convenient registration paths or broad
category names. IEScript must include public globals registered outside
`registerModules`; architecture must include concrete source-backed
claim rows; IEMon command coverage must require full reference entries
for command surfaces that need argument, error, and side-effect
documentation.
Purpose judgement: The previous source inventory pass improved the plan
but still allowed whackamole failures: `bit32.*` and `keys.*` were
documented but absent from the audit table, architecture coverage could
pass with category names only, and `trace mmio` could pass through an
overview mention without a command-reference subsection.
Canonical sources checked: `script_engine.go` `registerModules`,
`registerBit32`, and the manually built `keys` table; `debug_commands.go`
`trace` help and `cmdTraceMMIO`; source constants and registration sites
for architecture memory map rows, JIT dispatch build tags, coprocessor
mailbox capacity, Lua `mem.*` bus semantics, and monitor snapshot-device
registration.
Runnable verification:
`GOCACHE=/home/zayn/GolandProjects/IntuitionEngine/.codex-gocache go
test -tags headless -run
'TestSDKDocAuditLedger|TestSDKISAInventory|TestSDKCompanionDocs|TestSDK(IEMon|IEScript|Architecture)'
.`
Observed result: Extended the IEScript audit generator to emit
`bit32.*` bindings and individual `keys.*` constants. Replaced the
architecture audit's category-only coverage with additional concrete
memory-map, JIT-matrix, coprocessor, Lua-memory, and snapshot-contract
claim rows. Required `iemon.md` to contain a full
`trace mmio <region> [count]` command-reference subsection. Expanded the
IEScript `keys` table so every source-registered key constant appears
as a manual token. The focused SDK document gate passes.
Action: Updated the inventory generator/tests, regenerated the IEScript
and architecture source-audit tables, and repaired the IEMon and
IEScript manual gaps exposed by the stronger gate.
Notes: Because this entry changes shipped Markdown, inventory files,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0072
Status: FIXED
Document: `sdk_source_inventory_test.go`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`, and
`sdk/docs/iemon.md`.
Section: Empirical source inventory completeness for architecture,
IEMon, and IEScript semantics.
Claim: The architecture audit must cover the full public memory-map
surface published by `architecture.md`, not only selected ranges. The
IEMon audit must record command summaries, syntax variants, examples,
and dispatch aliases from the structured help registry. The IEScript
audit must include source-backed API behaviour claims where the manual
documents user-visible semantics, not only binding names.
Purpose judgement: A category-level or selected-row audit still lets
precise public claims drift. The source audit tables must enumerate the
claim surfaces users rely on so a future change to source constants,
command syntax, or API behaviour creates a golden diff instead of
passing silently.
Canonical sources checked: `registers.go`, device constant files, and
`main.go` `MapIO` registrations for the architecture memory map;
`debug_commands.go` `monitorHelpRegistry` and command dispatch aliases
for IEMon; `script_engine.go` `requireFrozenForRange` for the raw
memory freeze contract.
Runnable verification:
`GOCACHE=/home/zayn/GolandProjects/IntuitionEngine/.codex-gocache go
test -tags headless -run
'TestSDKDocAuditLedger|TestSDKISAInventory|TestSDKCompanionDocs|TestSDK(IEMon|IEScript|Architecture)'
.`
Observed result: Expanded the architecture inventory to cover all
published memory-map ranges, including terminal, VGA, ULA, ULA VRAM,
clipboard, Bootstrap HostFS, Voodoo, worker reservations, mailbox, and
profile memory rows. Expanded the IEMon inventory to include registry
summaries, syntax variants, and examples, and added a compact registry
syntax inventory to `iemon.md` so source syntax is mechanically covered
without duplicating every help example in the prose. Added the IEScript
`raw memory access requires cpu.freeze()` API claim to the source audit.
The focused SDK document gate passes.
Action: Updated the inventory generator/tests, regenerated the
architecture, IEMon, and IEScript source-audit tables, and repaired the
IEMon manual syntax coverage exposed by the stronger gate.
Notes: Because this entry changes shipped Markdown, inventory files,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0073
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/iescript.md`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`, and
`sdk_source_inventory_test.go`.
Section: Architecture source-evidence rows, architecture subrange
coverage, and IEScript semantic API contracts.
Claim: Architecture inventory rows must cite executable source on disk,
not the manual being audited. Detailed public architecture subranges
published outside the top-level memory-map table must appear in the
source audit. The IEScript audit must include source-backed API
contracts for signatures, return behaviour, address truncation, range
errors, and other user-visible semantics, not only binding names.
Purpose judgement: The empirical tables are only useful when they catch
the drift users would see in the shipped manuals. A self-referential
architecture row and binding-only script inventory can pass while
published addresses and API behaviour are wrong.
Canonical sources checked: `machine_bus.go` low-RAM boundary constants;
`video_chip.go` Mode7 register offsets; `audio_chip.go` SoundChip
legacy, FLEX, filter, sync, and ring-mod constants; `script_engine.go`
`luaMemRead*`, `luaMemWrite*`, `luaMemReadBlock`,
`luaMemWriteBlock`, `luaMemFill`, and `registerBit32`.
Runnable verification:
`UPDATE_SDK_ARCH_SOURCE_AUDIT=1 UPDATE_SDK_IESCRIPT_SOURCE_AUDIT=1 go
test -tags headless -run
'TestSDK(Architecture|IEScript)SourceInventoryGoldenMatchesSource|TestSDK(Architecture|IEScript)ManualCoverageMatchesSourceInventory'
.`
Observed result: Replaced the `0x00000-0x9EFFF` architecture audit
evidence with `machine_bus.go` constants. Added architecture memory-map
subrange rows for Mode7 registers, legacy SoundChip waveform windows,
sync/ring-mod source registers, global filter registers, primary FLEX,
SID2 FLEX, and SID3 FLEX ranges. Corrected `architecture.md` so the
global SoundChip filter range is `0xF0820-0xF0830`, and added explicit
modulation/effects subrange text. Added IEScript API-contract rows for
`mem.*` address truncation, returns, length errors, raw-byte behaviour,
and `bit32` shift/rotate/extract/replace error semantics; updated
`iescript.md` to document those source-backed behaviours.
Action: Updated the inventory generator/tests, regenerated the
architecture and IEScript source-audit tables, and repaired the two
manuals exposed by the stronger gate.
Notes: Because this entry changes shipped Markdown, inventory files,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0074
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/iemon.md`,
`sdk/docs/iescript.md`, `sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`,
`sdk_source_inventory_test.go`, `cpu_z80_runner.go`,
`cpu_six5go2.go`, and `registers.go`.
Section: Architecture CPU bridge tables, SoundChip decoded legacy
subranges, IEScript reverse-history API contracts, and stale bridge
comments.
Claim: Architecture-visible CPU bridge maps must be audited as
source-backed public architecture claims. Z80 and 6502 VGA bridge
ranges must include the DAC read-index, DAC mask, and VRAM-bank
registers through `$AD` and `$D70D`; x86 must not publish standard PC
VGA I/O ports because the x86 adapter does not implement them.
SoundChip legacy rows must preserve the triangle and sine sweep
register exceptions inside the numeric square range. IEScript public
API rows must cover return-field and option-field contracts, not only
binding names.
Purpose judgement: These claims are concrete user-facing programming
surfaces. Broad memory rows and binding-existence checks can pass while
bridge tables, decoded register exceptions, and structured API returns
drift from source.
Canonical sources checked: `vga_constants.go` Z80 and 6502 VGA bridge
constants; `cpu_z80_runner.go` `Z80BusAdapter.In`/`Out`;
`cpu_six5go2.go` `Bus6502Adapter.Read`/`Write`; `cpu_x86_runner.go`
`X86BusAdapter.In`/`Out` and VRAM translation; `audio_chip.go`
`TRI_SWEEP`, `SINE_SWEEP`, legacy register constants, and dispatch
ordering; `script_engine.go` `luaDbgHistoryHorizon` and
`luaDbgHistoryConfig`.
Runnable verification:
`UPDATE_SDK_ARCH_SOURCE_AUDIT=1 UPDATE_SDK_IESCRIPT_SOURCE_AUDIT=1 go
test -tags headless -run
'TestSDK(Architecture|IEScript)SourceInventoryGoldenMatchesSource|TestSDK(Architecture|IEScript)ManualCoverageMatchesSourceInventory'
.`
Then:
`go test -tags headless -run
'TestSDKDocAuditLedger|TestSDKISAInventory|TestSDKCompanionDocs|TestSDK(IEMon|IEScript|Architecture)'
.`
Observed result: Added `cpu bridge row` facts to the architecture
inventory and coverage gate. Updated the architecture manual to document
Z80 VGA as `$A0-$AD`, 6502 VGA as `$D700-$D70D`, and x86 as having no
standard PC VGA I/O port bridge. Replaced simplified SoundChip
subrange facts with decoded exception facts for `0xF0914` and
`0xF0918`, and updated architecture prose accordingly. Fixed stale Z80
and 6502 VGA-range source comments and the IEMon Z80 bridge summary.
Added IEScript reverse-history table-field and option-field API
contracts to the generated source audit and manual. The focused SDK
document gate passes.
Action: Updated the inventory generator/tests, regenerated the
architecture and IEScript source-audit tables, repaired architecture,
IEMon, and IEScript prose, and fixed stale source comments.
Notes: Because this entry changes shipped Markdown, inventory files,
tests, source comments, and the ledger after the previous render, the
final PDF render gate in `SDK-DOC-0031` must be repeated after this
entry.
Disposition: KEEP.

ID: SDK-DOC-0075
Status: FIXED
Document: `sdk/docs/iemon.md`, `sdk/docs/architecture.md`,
`sdk/docs/iescript.md`, `sdk/docs/verify/SDK_IEMON_SOURCE_AUDIT.md`,
`sdk/docs/verify/SDK_IESCRIPT_SOURCE_AUDIT.md`,
`sdk_source_inventory_test.go`, `registers.go`, and
`cpu_x86_runner.go`.
Section: IEMon CPU region-divergence rows, x86 bridge wording,
IEScript media type contract, and stale source comments.
Claim: IEMon region-divergence rows are public monitor reference
claims and must be covered by the empirical IEMon source audit. The
6502 VGA range is `0xD700-0xD70D`, not `0xD700-0xD70A`. The x86
runner exposes shared ports and a VGA VRAM window, not standard PC VGA
I/O ports. `media.type()` includes `midi` in its returned string set.
Purpose judgement: These are precise user-facing bridge/API claims. A
passing command-name or binding-name audit is insufficient when tables
publish address ranges or functions publish closed return sets.
Canonical sources checked: `vga_constants.go` `C6502_VGA_BASE` and
`C6502_VGA_END`; `cpu_six5go2.go` 6502 VGA read/write dispatch;
`registers.go` CPU-specific I/O comments; `cpu_x86_runner.go`
`X86BusAdapter.In`/`Out`, `translateVRAM`, and x86 runner field
comments; `script_engine.go` `mediaTypeToString`; `media_loader.go`
MIDI extension detection.
Runnable verification:
`UPDATE_SDK_IEMON_SOURCE_AUDIT=1 UPDATE_SDK_IESCRIPT_SOURCE_AUDIT=1 go
test -tags headless -run
'TestSDK(IEMon|IEScript)SourceInventoryGoldenMatchesSource|TestSDK(IEMon|IEScript)ManualCoverageMatchesSourceInventory'
.`
Observed result: Added IEMon `region divergence row` facts for Z80 and
6502 bridge differences, and made the IEMon manual coverage test enforce
them. Updated `iemon.md` and the stale `registers.go` source comment to
use `0xD700-0xD70D` for 6502 VGA. Replaced the architecture source-file
inventory wording for `cpu_x86_runner.go` with shared port I/O, bank
windows, and VGA VRAM-window wording, and corrected stale x86 runner
comments that described VGA as port I/O. Added the `media.type()`
returned-string contract to the IEScript source audit and documented the
`midi` return value in `iescript.md`. The focused IEMon/IEScript source
inventory gate passes.
Action: Updated the inventory generator/tests, regenerated the IEMon and
IEScript source-audit tables, repaired IEMon, architecture, and
IEScript prose, and fixed stale source comments.
Notes: Because this entry changes shipped Markdown, inventory files,
tests, source comments, and the ledger after the previous render, the
final PDF render gate in `SDK-DOC-0031` must be repeated after this
entry.
Disposition: KEEP.

ID: SDK-DOC-0076
Status: FIXED
Document: `cpu_six5go2.go`.
Section: 6502 banked-visible memory comment.
Claim: The 6502 extended banking source comment must not describe the
banked-visible CPU ceiling as a fixed 16 MiB address space. Executable
source exposes the current banked CPU-visible ceiling through the shared
bus configuration, and the architecture manual already documents the
32 MiB ceiling.
Purpose judgement: Source comments in source-routed architecture areas
are part of the audit surface because stale comments can seed future
manual drift. The executable source remains canonical; the comment must
match the source-backed contract.
Canonical sources checked: `machine_bus.go` `DEFAULT_MEMORY_SIZE`,
`banked8BitVisibleRAMBytes`, and `BankedVisibleCeiling`;
`boot_guest_ram.go` 6502/Z80 boot sizing path; `cpu_six5go2.go`
`Bus6502Adapter` bank-window comment; `sdk/docs/architecture.md`
6502/Z80 banked visibility wording.
Runnable verification:
`go test -tags headless -run
'TestSDKISAInventory|TestSDKIEMonSourceInventory|TestSDKIEScriptSourceInventory|TestSDKArchitectureSourceInventory|TestSDKCompanionDocs'
.`
Observed result: Rewrote the stale `cpu_six5go2.go` comment to describe
the current banked CPU-visible ceiling rather than a fixed 16 MiB address
space. No shipped manual text or generated source-audit table required a
content change for this finding because `architecture.md` already states
the 32 MiB ceiling.
Action: Fixed the stale source comment and reran focused SDK document
and source-inventory gates.
Notes: Because this entry changes a source comment and the ledger after
the previous render, the final PDF render gate in `SDK-DOC-0031` must be
repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0077
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/iemon.md`, `cpu_ie32.go`.
Section: IE64 zero-divisor semantics, IEMon shared-memory wording, and
IE32 constructor comment.
Claim: IE64 integer divide and remainder instructions do not trap or stop
on a zero divisor; executable source writes zero to `Rd` when `Rd` is not
`R0`. IEMon memory-region prose must describe CPU views of the shared
MachineBus memory map, not "host-visible" RAM. The IE32 constructor source
comment must state that it uses the supplied bus's shared memory slice,
not that it allocates main memory.
Purpose judgement: These are user-visible reference facts or source
comments in source-routed documentation areas. Omitting the zero-divisor
result leaves experienced assembly programmers with an incomplete ISA
contract. "Host-visible RAM" conflicts with the five-book shared-memory
rule and can imply a private or host-side address space that executable
source does not provide. The stale constructor comment could seed future
manual drift and therefore must be fixed.
Canonical sources checked: `cpu_ie64.go` cases `OP_DIVU`, `OP_DIVS`,
`OP_MOD64`, and `OP_MODS`; `cpu_ie32.go` `CPU.memory` field and
`NewCPU`; `machine_bus.go` `GetMemory`; `sdk/docs/IE64_ISA.md`
integer arithmetic entries; `sdk/docs/iemon.md` memory-map table.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_IE64IntegerDivisionDocumentsZeroDivisorResult|TestSDKCompanionDocs_IEMonMemoryMapUsesSharedMachineMemoryWording|TestSDKISAInventoryRejectsStaleSourceComments'
.`
Observed result: The first run of the new gates failed on the missing
IE64 zero-divisor wording, `host-visible RAM` in IEMon, and the stale
`Allocates main memory` source comment. After the fixes, the same focused
test command passed.
Action: Added focused regression gates, documented the IE64 zero-divisor
result on `DIVU`, `DIVS`, `MOD`, and `MODS`, rewrote the IEMon memory-map
paragraph and IE64/IE32/M68K rows to shared-machine-memory wording, and
fixed the IE32 source comment.
Notes: Because this entry changes shipped Markdown, a source comment,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0078
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `machine_bus.go`,
`sdk_doc_audit_test.go`, and `sdk_isa_inventory_test.go`.
Section: IE64 integer divide/remainder Operation fields and RAM-size
control-register source comments.
Claim: IE64 integer divide and remainder instruction entries must make
the zero-divisor result self-contained in the `Operation` field, not only
in the `Notes` field. Source comments must not describe
`CR_RAM_SIZE_BYTES` as a future IE64 path because the control register is
implemented and documented.
Purpose judgement: Motorola-style instruction entries rely on the
Operation field as the compact semantic contract. A raw `/` or `%`
formula is misleading when executable source returns zero for a zero
divisor. Stale source comments in source-routed memory-sizing code can
seed future manual drift.
Canonical sources checked: `cpu_ie64.go` cases `OP_DIVU`, `OP_DIVS`,
`OP_MOD64`, and `OP_MODS` in both normal execution and `StepOne`;
`cpu_ie64.go` `CR_RAM_SIZE_BYTES` definition and MFCR/MTCR handling;
`machine_bus.go` `SetSizing` and `ActiveVisiblePages` comments;
`sdk/docs/IE64_ISA.md` integer arithmetic entries.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_IE64IntegerDivisionDocumentsZeroDivisorResult|TestSDKISAInventoryRejectsStaleSourceComments|TestSDKCompanionDocs|TestSDKDocAuditLedger'
.`
Observed result: Rewrote the `DIVU`, `DIVS`, `MOD`, and `MODS`
Operation fields so the zero-divisor result is explicit in the operation
schema. Removed stale "future IE64 CR_RAM_SIZE_BYTES" wording from
`machine_bus.go`. Tightened the focused document and stale-source-comment
gates so these classes of drift fail tests.
Action: Updated the IE64 manual, source comments, focused regression
tests, and ledger evidence.
Notes: Because this entry changes shipped Markdown, source comments,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0079
Status: FIXED
Document: `ahx_constants.go`, `sdk_isa_inventory_test.go`, and this
ledger.
Section: AHX source-comment range correctness.
Claim: Source comments for the AHX register block must match the
executable `MapIO(AHX_BASE, AHX_SUBSONG, ...)` range. The implemented AHX
block is `0xF0B80-0xF0B91`, not `0xF0B80-0xF0B94`; the AHX player
subrange ends at `0xF0B91`, not `0xF0B94`.
Purpose judgement: The shipped architecture manual already publishes the
correct range, but stale source comments beside register constants can
seed future manual and audit drift. The ledger rule requires stale source
comments discovered during source-routed audits to be fixed even when the
manual text is already correct.
Canonical sources checked: `ahx_constants.go` constants `AHX_BASE` and
`AHX_SUBSONG`; `main.go` AHX `MapIO(AHX_BASE, AHX_SUBSONG, ...)`;
`sdk/docs/architecture.md` AHX architecture row.
Runnable verification:
`go test -tags headless -run
'TestSDKISAInventoryRejectsStaleSourceComments|TestSDKArchitectureSourceInventory|TestSDKCompanionDocs|TestSDKDocAuditLedger'
.`
Observed result: Corrected both AHX source comments to end at
`0xF0B91` and extended the stale-source-comment regression to reject the
old `0xF0B94` AHX ranges.
Action: Updated the AHX source comments, focused stale-comment test, and
ledger evidence.
Notes: Because this entry changes a source comment, a test, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0080
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`cpu_ie64.go`, `jit_exec.go`, `jit_exec_test.go`, `cpu_ie32.go`,
`ted_constants.go`, `ted_video_constants.go`, `registers.go`,
`sdk_doc_audit_test.go`, `sdk_isa_inventory_test.go`, and this ledger.
Section: IE64 timer/JIT timing, reserved encodings, and stale
source-comment cleanup.
Claim: The IE64 timer is a CPU instruction-step timer. JIT execution must
not defer timer expiry until after a multi-instruction native block when
the architectural timer is armed. IE64 reserved control-register
encodings, IE32 reserved addressing-mode bytes, TED video ranges in
source comments/central region constants, and IE32 operand-resolution
comments must match executable source behaviour.
Purpose judgement: These are public CPU-manual and audit-source
contracts. A passing text inventory is not sufficient when executable JIT
timing differs from the interpreter or when reserved encodings have
source-defined behaviour that the manuals omit.
Canonical sources checked: `cpu_ie64.go` timer pre-dispatch path,
`StepOne`, MTCR/MFCR switch bodies, and timer control registers;
`jit_exec.go` native-block dispatch; `jit_common.go` multi-instruction
block scanning; `cpu_ie32.go` `resolveOperand` and `storeRegister`;
`ted_video_constants.go` `TED_VIDEO_END`; `registers.go`
`TED_REGION_END`; `main.go` TED video `MapIO`; the IE64 and IE32 shipped
manuals.
Runnable verification:
`go test -tags headless -run
'TestExecuteJIT_TimerInterruptsBeforeMidBlockInstruction|TestSDKCompanionDocs_IE64ReservedControlRegisterEncodings|TestSDKCompanionDocs_IE64MMUPrivilegeInstructionsUseFullSchema|TestSDKCompanionDocs_IE32ReservedAddressingModesMatchSource|TestSDKISAInventoryRejectsStaleSourceComments'
.`
Observed result: Added a JIT regression proving a timer interrupt fires
before the body of the mid-block instruction whose decoded step expires
the timer. Routed armed-timer IE64 JIT execution through `StepOne` and
moved the shared timer step into a CPU helper used by both `Execute` and
`StepOne`, eliminating block-boundary countdown. Documented supervisor
reserved CR behaviour (`MFCR` returns zero, `MTCR` has no effect) and IE32
reserved addressing-mode behaviour (`0x05`-`0xFF`). Corrected stale TED
video source comments and the central TED region end to `0xF0F6B`.
Corrected the IE32 operand-resolution helper comment and extended focused
stale-comment/manual gates for these defects. The focused verification
command passed.
Action: Updated source, manuals, focused regression tests, stale-comment
gates, and ledger evidence.
Notes: Because this entry changes shipped Markdown, source, source
comments, tests, and the ledger after the previous render, the final PDF
render gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0081
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/architecture.md`,
`cpu_ie64.go`, `jit_common.go`, `cpu_x86_runner.go`,
`cpu_z80_runner.go`, `cpu_chip_matrix_test.go`,
`ted_video_constants.go`, `sdk/include/ie65.inc`,
`sdk/include/ie80.inc`, `sdk_source_inventory_test.go`,
`sdk_doc_audit_test.go`, `sdk_isa_inventory_test.go`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`, and this ledger.
Section: IE64 invalid FPU-register execution and CPU bridge coverage for
TED, SN76489, and ULA.
Claim: IE64 invalid FPU register encodings must enter stopped processor
state with PC unchanged on every CPU execution path, including debugger
single-step and armed-timer JIT fallback. Architecture bridge tables and
their source inventory must cover the executable TED video, SN76489, and
ULA port surfaces, and stale TED bridge comments/includes must be fixed.
Purpose judgement: The IE64 manual is a physical CPU reference, so it
cannot describe behaviour implemented only by the normal interpreter
while `StepOne` and JIT fallback differ. The architecture manual publishes
bridge tables as user-facing machine contracts, so the empirical
architecture inventory must include the exact bridge rows instead of
only broad categories or VGA-only rows.
Canonical sources checked: `cpu_ie64.go` normal FPU dispatch,
`StepOne`, and invalid FPU register handling; `jit_exec.go`
armed-timer fallback through `interpretOne`; `jit_common.go` block
fallback decision; `ted_video_constants.go` TED video index and 6502
range constants; `cpu_x86_runner.go` x86 TED port bridge;
`cpu_z80_runner.go` Z80 TED/SN76489/ULA port bridge;
`cpu_six5go2.go` 6502 TED video dispatch; `sn76489_constants.go`;
`ula_constants.go`; `sdk/docs/architecture.md`; and
`SDK_ARCH_SOURCE_AUDIT.md`.
Runnable verification:
`go test -tags headless -run
'TestIE64StepOneInvalidFPURegisterStopsWithoutAdvancingPC|TestExecuteJIT_TimerArmedInvalidFPURegisterStopsWithoutAdvancingPC|TestZ80_TED_PortIO|TestZ80_ULA_PortIO|TestZ80_SN76489_PortIO|TestSDKArchitectureSourceInventory|TestSDKCompanionDocs_ArchitectureCPUBridgeTablesCoverSourceRoutes|TestSDKISAInventoryRejectsStaleSourceComments'
.`
Observed result: Added direct `StepOne` and armed-timer JIT regressions
for invalid IE64 FPU register encodings. `StepOne` now rejects missing
FPU or invalid FPU register fields before executing or advancing PC, and
JIT block fallback rejects invalid FPU encodings before native emission.
Added x86-owned TED video index names while keeping Z80-owned aliases,
corrected x86 TED video bridge handling through index `0x32`, corrected
stale Z80/x86/TED/include comments, expanded 6502 include constants for
TED raster compare registers, added source-backed Z80 SN76489 and ULA
bridge tests, added architecture bridge rows, and extended
`SDK_ARCH_SOURCE_AUDIT.md` plus coverage gates to reject the previous
bridge omissions. The focused verification command passed.
Action: Updated source, tests, source comments/includes, shipped
architecture manual, generated architecture source inventory, focused
audit gates, and ledger evidence.
Notes: Because this entry changes shipped Markdown, source, source
comments/includes, tests, a generated inventory, and the ledger after the
previous render, the final PDF render gate in `SDK-DOC-0031` must be
repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0082
Status: FIXED
Document: `sdk/docs/architecture.md`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk_doc_audit_test.go`, `sdk_source_inventory_test.go`, and this
ledger.
Section: Program Executor operation values and IntuitionOS architecture
scope exception.
Claim: `EXEC_CTRL` value `4` is not reserved. It is the source-backed
IntuitionOS IExec operation value, with executable dispatch through
`EXEC_OP_IEXEC` and `startIExec()`. The architecture manual may name
IExec or Intuition OS wherever that is appropriate whole-machine
architecture material, while the ISA manuals, IEMon manual, and IEScript
manual must still exclude OS-owned material.
Purpose judgement: `architecture.md` is the whole-machine architecture
manual. It must publish stable MMIO control values exposed by source, and
the gate must not force a false reserved value to avoid an OS-owned name.
At the same time, the CPU, monitor, and scripting manuals must not become
IntuitionOS references.
Canonical sources checked: `program_executor_constants.go`
`EXEC_OP_EXECUTE`, `EXEC_OP_EMUTOS`, `EXEC_OP_AROS`, `EXEC_OP_IEXEC`,
and `EXEC_OP_HARD_RESET`; `program_executor.go` `HandleWrite` dispatch
to `startExecute`, `startEmuTOS`, `startAROS`, `startIExec`, and
`startHardReset`; and `program_executor_test.go` operation-value pins.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_NoOSManualMaterialOutsideArchitecture|TestSDKCompanionDocs_ArchitectureProgramExecutorOpsMatchSource|TestSDKArchitectureSourceInventory|TestSDKArchitectureManualCoverageMatchesSourceInventory'
.`
Observed result: Added a failing regression gate first. It failed while
`architecture.md` still said `4=Reserved` and
`SDK_ARCH_SOURCE_AUDIT.md` lacked an `EXEC_CTRL` operation-values row.
After the fix, `architecture.md` states
`EXEC_CTRL operation values: 1=Execute, 2=EmuTOS, 3=AROS,
4=IntuitionOS IExec, 5=Hard reset`; `SDK_ARCH_SOURCE_AUDIT.md` carries
the same source-backed architecture-claim row; and the focused
verification command passed.
Action: Corrected the Program Executor diagram, narrowed the OS-material
exclusion gate to the ISA, monitor, and scripting manuals plus
architecture-only GURU/fault-printer wording, added a source-backed
Program Executor gate, and
added the operation-values fact to the generated architecture inventory.
Notes: This entry supersedes the temporary `4=Reserved` wording recorded
under `SDK-DOC-0047`. Because this entry changes shipped Markdown, a
generated inventory, tests, and the ledger after the previous render, the
final PDF render gate in `SDK-DOC-0031` must be repeated after this
entry.
Disposition: KEEP.

ID: SDK-DOC-0083
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/IE32_ISA.md`,
`sdk_doc_audit_test.go`, and this ledger.
Section: IE64 atomic aperture, low-32-bit bit-operation fields, PC
overview, and IE32 stack-bound predicates.
Claim: IE64 atomic RMW instructions are limited to the processor's
atomic RAM aperture after optional MMU translation, not every
CPU-visible writable memory range. `CLZ`, `CTZ`, `POPCNT`, and `BSWAP`
always operate on the low 32 bits of `Rs`; their encoded size bits are
ignored by the processor even though the assembler spelling is `.l`.
The IE64 PC overview must allow instruction-specific PC behaviour for
control-transfer, trap, return, fault, interrupt, and stopped-state
instructions. IE32 stack documentation must distinguish the inlined
`PUSH`/`JSR` overflow check from interrupt entry through the CPU push
helper while stating the word-alignment invariant that makes the aligned
boundary equivalent.
Purpose judgement: The ISA manuals are physical CPU reference manuals.
They must describe the CPU-visible contract precisely enough for
low-level code, including reserved/ignored fields and exact stopped or
faulting boundaries, without overgeneralizing from a broad memory or PC
summary.
Canonical sources checked: `cpu_ie64.go` `execAtomic` alignment,
low-MMIO aperture, optional MMU translation, `len(cpu.memory)` bounds,
and `cpu.memBase` atomic pointer formation; `cpu_ie64.go` `OP_CLZ`,
`OP_CTZ`, `OP_POPCNT`, and `OP_BSWAP` dispatch using
`uint32(cpu.regs[rs])`; `cpu_ie64.go` `OP_HALT64`, `OP_RTI64`, and
`OP_ERET` PC paths; and `cpu_ie32.go` `Push`, inlined `PUSH`/`JSR`,
and interrupt-entry paths.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_IE64AtomicFaultsMatchSource|TestSDKCompanionDocs_IE64PCOverviewAllowsInstructionSpecificPCBehavior|TestSDKCompanionDocs_IE64LOnlyBitInstructionsDocumentIgnoredSizeField|TestSDKCompanionDocs_IE32StackLimitPredicatesMatchSource'
.`
Observed result: Added failing gates first. The initial run failed on
the IE64 PC overview, low-32-bit field prose, IE32 stack table, and IE64
atomic aperture wording. After the manual fixes, the same focused
command passed. IE64 now documents atomic RAM aperture limits, ignored
size bits for the four low-32-bit operations, and instruction-specific
PC behaviour. IE32 now splits the stack overflow predicates and records
the word-alignment invariant.
Action: Updated the IE64 and IE32 manuals plus focused audit gates.
Notes: Because this entry changes shipped Markdown, tests, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0084
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/iemon.md`, `cpu_ie64.go`, `sdk_doc_audit_test.go`, and this
ledger.
Section: Architecture compositor alpha rule, IE64 fault cause 0,
IE64 signed-immediate operand wording, IE64 trap-stack reset wording,
and IEMon access-instrumentation pitfall.
Claim: The architecture compositor contract is all-zero transparency:
an all-zero frame pixel is transparent, and any nonzero alpha or RGB
component is opaque, with zero-alpha nonzero-RGB pixels promoted to
opaque `0xFFRRGGBB`. IE64 fault cause 0 covers absent page mappings and
unavailable physical or atomic backing, not only `P=0` PTEs. The IE64
`MULS` immediate entry, and the neighbouring `DIVS` immediate
description using the same source operand path, must state that the
immediate operand is zero-extended `imm32` converted to `int64`, with
size applied only when masking the result. IE64 reset wording must
describe processor reset state instead of Go object reuse. IEMon
access-backed commands fail closed whenever access instrumentation is
not enabled.
Purpose judgement: The architecture and monitor manuals publish
current machine behaviour, not speculative future conditions. The IE64
ISA manual is a physical CPU reference and must use architectural
wording for fault causes, operand interpretation, and reset state.
Canonical sources checked: `video_compositor.go`
`compositorOpaquePixel`, `video_compositor_test.go`
`TestCompositorOpaquePixelTreatsRGBWithZeroAlphaAsOpaque`,
`cpu_ie64.go` `FAULT_NOT_PRESENT`, physical load/store and atomic
fault sites, `cpu_ie64.go` `OP_MULS` immediate operand decoding in both
execution paths, `cpu_ie64.go` `Reset`, and `debug_commands.go`
`Instrumented()` fail-closed checks for page guards, access log, and
`bfirst`.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_IE64MULSImmediateDocumentsZeroExtendedOperand|TestSDKCompanionDocs_ArchitectureCompositorAlphaMatchesSource|TestSDKCompanionDocs_IE64FaultCauseZeroCoversPhysicalAndAtomicBacking|TestSDKCompanionDocs_IE64TrapStackResetUsesProcessorManualVoice|TestSDKCompanionDocs_IEMonAccessInstrumentationFailsClosed'
.`
Observed result: Added failing gates first. The initial run failed on
the stale `MULS` immediate description, compositor alpha sentence,
narrow `FAULT_NOT_PRESENT` source comment and table row, implementation
voice in the trap-stack reset paragraph, and speculative IEMon
instrumentation wording. After fixing the manuals and stale source
comment, the same focused command passed. A final ambiguity scan also
found the same signed-immediate phrase in the `DIVS` immediate
description; that neighbouring description now uses the same
zero-extended-`imm32` wording.
Action: Updated the three manuals, the stale IE64 source comment, and
focused audit gates.
Notes: Because this entry changes shipped Markdown, a source comment,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0085
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk_doc_audit_test.go`, `sdk_source_inventory_test.go`, and this
ledger.
Section: Architecture video-compositor scale-mode contract,
CPU-selection RAM sizing for bare `.ie68`, IE64 `MULHS` immediate
operand wording, and architecture source inventory coverage.
Claim: The video compositor starts in stretch-fill mode, and `F11`
toggles non-16:9 sources to aspect-fit. Bare `.ie68` uses the
active-visible RAM ceiling path shared with IE32/x86; EmuTOS and AROS
M68K loader modes use explicit profile bounds. IE64 `MULHS #imm` uses
zero-extended `imm32` converted to `int64`, not a sign-extended
immediate field. The architecture source audit must include empirical
claim rows for compositor scale mode and bare/profile-bound M68K RAM
sizing so broad subsystem coverage cannot hide these contracts.
Purpose judgement: These are user-facing architecture and ISA contracts
derived from executable source, and the audit inventory must track
claim-level behaviour rather than only broad subsystem existence.
Canonical sources checked: `video_compositor.go` `NewVideoCompositor`
and `ToggleScaleModeIfNonNative`, `video_compositor_test.go`
default-scale regression, `boot_guest_ram.go` `resolveModeCaps` and
`resolveActiveVisibleCeiling` cases for `modeM68KBare`, `modeEmuTOS`,
and `modeAros`, and `cpu_ie64.go` `OP_MULHS` operand decoding and
`mulHighSigned` dispatch.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_ArchitectureCompositorScaleModeMatchesSource|TestSDKCompanionDocs_ArchitectureM68KBareUsesActiveVisibleRAMCap|TestSDKCompanionDocs_IE64MULHSImmediateDocumentsZeroExtendedOperand|TestSDKArchitectureSourceInventoryGoldenMatchesSource|TestSDKArchitectureManualCoverageMatchesSourceInventory'
.`
Observed result: Added failing gates and source-inventory rows first.
The initial run failed on the reversed compositor scale sentence, the
bare `.ie68` profile-bound wording, the incomplete `MULHS` immediate
description, and the missing architecture audit rows. After fixing the
manual text and regenerating `SDK_ARCH_SOURCE_AUDIT.md`, the same
focused command passed.
Action: Updated the architecture and IE64 manuals, added focused gates,
and expanded the generated architecture source inventory.
Notes: Because this entry changes shipped Markdown, tests, a generated
audit table, and the ledger after the previous render, the final PDF
render gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0086
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`,
`sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md`,
`sdk_isa_inventory_test.go`, and this ledger.
Section: IE64 FPU instruction side effects and ISA source inventory
coverage.
Claim: IE64 FPU instructions that only update FPSR condition-code bits
must not claim possible sticky exception flag writes. `FABS`, `FNEG`,
`FSIN`, `FCOS`, `FTAN`, `FATAN`, `DABS`, and `DNEG` write FPSR
condition-code bits and do not set FPSR sticky exception flags. `FINT`
and `DINT` read FPCR rounding bits, write FPSR condition-code bits, and
do not set FPSR sticky exception flags. The ISA source audit must track
FPU side-effect rows for condition-code writes, FPCR reads, and sticky
exception flag write/no-write contracts instead of only opcode
existence.
Purpose judgement: The IE64 manual is a processor reference manual; an
Attributes line is an instruction-level contract and must distinguish
condition-code state from sticky exception flags exactly.
Canonical sources checked: `cpu_ie64.go` execute and step switch cases
for the affected FPU opcodes, and `fpu_ie64.go` `FABS`, `FNEG`,
`FSIN`, `FCOS`, `FTAN`, `FATAN`, `FINT`, `DABS`, `DNEG`, and `DINT`
implementations. The broader generated inventory also checks
source-backed sticky-writing FPU entries such as arithmetic, sqrt, log,
exp, and pow.
Runnable verification:
`go test -tags headless -run
'TestSDKISAInventoryGoldenMatchesExecutableSource|TestSDKISAInventoryManualCoverageMatchesSourceFacts'
.`
Observed result: Added failing source-inventory and manual-coverage
gates first. The initial run failed because the generated ISA audit was
not granular enough and the manual still claimed possible sticky
exception flags for `FABS`. After correcting the affected IE64 FPU
Attributes lines, adding the FPCR/no-sticky wording for `FINT` and
`DINT`, and regenerating `SDK_ISA_SOURCE_AUDIT.md`, the same focused
command passed.
Action: Updated the IE64 manual, expanded the generated ISA source
inventory with FPU side-effect rows, and added manual coverage gates.
Notes: Because this entry changes shipped Markdown, tests, a generated
audit table, and the ledger after the previous render, the final PDF
render gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0087
Status: FIXED
Document: `sdk/docs/iemon.md`, `sdk/docs/iescript.md`,
`cpu_z80_runner.go`, `registers.go`, `ahx_constants.go`,
`sdk_doc_audit_test.go`, `sdk_isa_inventory_test.go`, and this ledger.
Section: User-facing process/source wording and stale source comments.
Claim: The shipped IEMon and IEScript manuals must not expose audit
process labels or implementation-file names where a user-facing
reference phrase is sufficient. IEMon must present the command table as
a command surface, not a source-checked audit artifact, and must treat
the IE64 ISA manual cause-code note as a cross-reference rather than as
the canonical source. IEScript must describe the IE Script Lua API
exposed to scripts, not the implementation file that registers it.
Stale source comments must match executable behaviour: Z80 JIT is
amd64-only in dispatch, IE32 timer state is CPU-integrated rather than a
stable bus register ABI, and the AHX range is an engine/player register
block with only the first byte being the AHX+ engine/control register.
Purpose judgement: Source files and generated audits remain valid
evidence, but shipped manuals should read as user-facing references and
source comments should not publish stale architecture claims.
Canonical sources checked: `jit_z80_dispatch.go` amd64 dispatch gate,
`cpu_z80_runner.go` `CPUZ80Config`, IE32 timer comments and CPU timer
behaviour, `registers.go`, `ahx_constants.go`, and the IEMon/IEScript
manual text on disk.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_UserManualsDoNotExposeAuditOrSourceFileInternals|TestSDKCompanionDocs_NoAuditProcessLanguage|TestSDKCompanionDocs_IEMonDispatchAliasesAndIE64Cause11|TestSDKISAInventoryRejectsStaleSourceComments'
.`
Observed result: Added failing gates for the user-facing wording and
stale comments, then fixed IEMon, IEScript, and the three source
comments. The focused command passed.
Action: Updated the two manuals, three source comments, and focused
audit gates.
Notes: Because this entry changes shipped Markdown, source comments,
tests, and the ledger after the previous render, the final PDF render
gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0088
Status: FIXED
Document: `sdk/docs/IE64_ISA.md`, `sdk/docs/architecture.md`,
`sdk/docs/iescript.md`, `sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk_doc_audit_test.go`, `sdk_source_inventory_test.go`, and this
ledger.
Section: IE64 indirect JSR encoding fields, IE64 load/store physical
backing faults, AROS IRQ diagnostics lifecycle, and IEScript PSG API
wording.
Claim: IE64 indirect `JSR` byte 1 is reserved and ignored; it does not
encode a quadword size. IE64 load/store exception text must state that
physical backing is checked after optional MMU translation, so non-MMU
physical accesses outside implemented CPU-visible memory also raise
`FAULT_NOT_PRESENT`. The `0xF23C0-0xF23DF` IRQ diagnostic block is
mapped by the AROS loader for the AROS M68K profile and unmapped during
AROS DMA teardown; it is not an always-present all-CPU MMIO block.
IEScript `audio.psg_load` must present its supported extension set
self-contained instead of referring users to an implementation file as
authoritative.
Purpose judgement: These are user-facing processor, architecture, and
script API contracts. Source evidence belongs in tests and generated
inventories, while shipped prose should describe the contract directly.
Canonical sources checked: `cpu_ie64.go` `OP_JSR_IND`, `loadMem`, and
`storeMem`; `assembler/ie64asm.go` `encodeInstruction` and `jsr`
encoding; `aros_loader.go` `MapIRQDiagnostics`; `main.go` AROS loader
call sites; `aros_audio_dma.go` IRQ diagnostic unmap; `registers.go`
IRQ diagnostic constants; `psg_player.go` PSG player extension cases;
and `media_loader.go` PSG media detection.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_UserManualsDoNotExposeAuditOrSourceFileInternals|TestSDKCompanionDocs_ArchitectureIRQDiagnosticsLifecycleMatchesSource|TestSDKCompanionDocs_IE64MemoryEntryExceptionsAreDirectionSpecific|TestSDKCompanionDocs_IE64StackAndJSRReferenceText|TestSDKArchitectureSourceInventoryGoldenMatchesSource'
.`
Observed result: Added failing gates and source-inventory evidence
first. The initial run failed on the stale IEScript implementation-file
reference and the architecture source-audit row that cited constants
without the AROS mapping lifecycle. After fixing the three manuals and
regenerating `SDK_ARCH_SOURCE_AUDIT.md`, the same focused command
passed.
Action: Updated IE64, architecture, and IEScript manuals; expanded the
architecture source inventory evidence; and added focused gates.
Notes: Because this entry changes shipped Markdown, tests, a generated
audit table, and the ledger after the previous render, the final PDF
render gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0089
Status: FIXED
Document: `sdk/docs/architecture.md`, `sdk/docs/IE64_ISA.md`,
`sdk/docs/verify/SDK_ARCH_SOURCE_AUDIT.md`,
`sdk_doc_audit_test.go`, `sdk_source_inventory_test.go`, and this
ledger.
Section: Darwin RAM-sizing contract and IE64 FPCR reserved-bit
semantics.
Claim: The architecture intro must state that Darwin RAM sizing uses a
page-aligned conservative half of `hw.memsize` as the detected base
before applying the per-platform reserve, not raw `hw.memsize` minus
reserve. IE64 FPCR prose must be normative processor-manual text:
FPU arithmetic interprets only bits 1:0 as the rounding mode, while
bits 31:2 are preserved and have no defined effect.
Purpose judgement: Both claims are user-facing architecture/processor
contracts. The manuals should describe the current contract directly;
source file names and implementation evidence belong in the tests and
generated audit table.
Canonical sources checked: `memory_sizing_usable_darwin.go`
`detectUsableRAM`, `memory_sizing.go` reserve application,
`fpu_ie64.go` `GetRoundingMode`, and `fpu_ie64.go` `FMOVCC`.
Runnable verification:
`go test -tags headless -run
'TestSDKCompanionDocs_ArchitectureRAMSizingNamesPlatformDispatch|TestSDKCompanionDocs_IE64FPCRReservedBitsAreNormative|TestSDKArchitectureSourceInventoryGoldenMatchesSource'
.`
Observed result: Added failing gates for the Darwin half-of-physical
detected base and normative FPCR reserved-bit wording, updated the two
manuals, regenerated `SDK_ARCH_SOURCE_AUDIT.md`, and the focused
verification passed.
Action: Updated the architecture intro, IE64 FPCR prose, architecture
source inventory, and companion-doc gates.
Notes: Because this entry changes shipped Markdown, tests, a generated
audit table, and the ledger after the previous render, the final PDF
render gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0090
Status: FIXED
Document: `sid_constants.go`, `sdk_isa_inventory_test.go`, and this
ledger.
Section: SID offset `0x19` source-comment and alias correctness.
Claim: `SID_PLUS_CTRL` at `0xF0E19` is the SID+ control state in
Intuition Engine. The source constants must not also publish
`SID_POT_X` at `0xF0E19` as a potentiometer-X register, because the
executable read path returns the backing register state at offset
`0x19` and the write path treats the same offset as SID+ control.
Purpose judgement: The five shipped manuals are already correct for this
finding, but source comments and source aliases are part of the
source-routed audit surface. Leaving a stale `SID_POT_X` alias beside
the live SID+ control constant can seed future manual and audit drift.
Canonical sources checked: `sid_constants.go` SID register constants;
`sid_engine.go` `HandleRead`, `WriteRegister`, and
`writeRegisterStateLocked`; `sdk/docs/architecture.md` SID+ row.
Runnable verification:
`go test -tags headless -run
'TestSDKISAInventoryRejectsStaleSourceComments|TestSDKDocAuditLedger|TestSDKCompanionDocs'
.`
Observed result: Removed the conflicting `SID_POT_X` alias and replaced
the old real-SID read-only-register comment with the Intuition Engine
readback contract for offset `0x19`, while preserving `SID_PLUS_CTRL` at
`0xF0E19`. Added a stale-source-comment regression that rejects the old
alias and wording.
Action: Updated the SID constants source comments/aliases, focused
stale-comment gate, and ledger evidence.
Notes: Because this entry changes a source comment, a source constant
alias, a test, and the ledger after the previous render, the final PDF
render gate in `SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0091
Status: FIXED
Document: `cpu_ie64.go`, `sdk_isa_inventory_test.go`, and this ledger.
Section: IE64 nested-trap SUA source-comment correctness.
Claim: IE64 trap entry pushes the previous active trap frame before
updating `CR_SAVED_SUA`, so nested-trap preservation is architectural.
Kernel handlers do not need to save and restore `CR_SAVED_SUA` or
`CR_FAULT_PC` manually to survive a nested synchronous trap.
Purpose judgement: The IE64 ISA manual already documents the current
processor contract, but source comments are part of the source-routed
audit surface. Leaving the old single-slot handler discipline in
`cpu_ie64.go` would contradict both executable trap-frame preservation
and the shipped processor manual.
Canonical sources checked: `cpu_ie64.go` `CR_SAVED_SUA` constants,
`trapEntry`, `pushTrapFrame`, `popTrapFrame`, `ERET` handling, and
`sdk/docs/IE64_ISA.md` trap-frame section.
Runnable verification:
`go test -tags headless -run
'TestSDKISAInventoryRejectsStaleSourceComments|TestSDKDocAuditLedger|TestSDKCompanionDocs'
.`
Observed result: Replaced the stale nested-trap comment with the
current architectural trap-frame preservation rule and added a
stale-source-comment regression that rejects the old manual
save/restore wording.
Action: Updated the IE64 source comment, focused stale-comment gate, and
ledger evidence.
Notes: Because this entry changes a source comment, a test, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0092
Status: FIXED
Document: `cpu_ie64.go`, `sdk_isa_inventory_test.go`, and this ledger.
Section: IE64 `CR_SAVED_SUA` inline source-comment correctness.
Claim: `CR_SAVED_SUA` is the saved SUA latch for the active trap frame
and is restored by `ERET` on supervisor return. It does not define a
manual save/restore discipline that mirrors `CR_FAULT_PC`; nested trap
preservation is handled by the architectural trap-frame stack.
Purpose judgement: The IE64 ISA manual already documents the current
processor contract, but stale inline source comments are part of the
source-routed audit surface and must not contradict the trap-frame
implementation.
Canonical sources checked: `cpu_ie64.go` control-register constants,
`trapEntry`, `pushTrapFrame`, `popTrapFrame`, `ERET` handling, and
`sdk/docs/IE64_ISA.md` trap-frame section.
Runnable verification:
`go test -tags headless -run
'TestSDKISAInventoryRejectsStaleSourceComments|TestSDKDocAuditLedger|TestSDKCompanionDocs'
.`
Observed result: Replaced the short stale `CR_SAVED_SUA` inline comment
with the active-trap-frame latch semantics and added a stale-source
comment regression rejecting the old `CR_FAULT_PC` mirror/save-restore
wording.
Action: Updated the IE64 source comment, focused stale-comment gate, and
ledger evidence.
Notes: Because this entry changes a source comment, a test, and the
ledger after the previous render, the final PDF render gate in
`SDK-DOC-0031` must be repeated after this entry.
Disposition: KEEP.

ID: SDK-DOC-0031
Status: FIXED
Document: All five shipped PDFs.
Section: Final PDF render gate.
Claim: The five shipped manuals have been rendered to PDFs after the
manual, test, ledger, style, and hard-gate edits stopped.
Purpose judgement: The PDFs are the public deliverable for the companion
set and must be regenerated from the final Markdown instead of reused
from a provisional render.
Canonical sources checked: `sdk/docs/IE64_ISA.md`,
`sdk/docs/IE32_ISA.md`, `sdk/docs/iemon.md`,
`sdk/docs/iescript.md`, `sdk/docs/architecture.md`,
`scripts/refman-pdf.sh`, the generated PDFs in `sdk/docs/`, and
`sdk/docs/verify/SDK_DOC_PDF_RENDER_MANIFEST.sha256`.
Runnable verification: Copy the five Markdown files plus
`sdk/docs/refman.publish/00-Preface.md` into a temporary source
directory; run `scripts/refman-pdf.sh --src <tmp-src> --out <tmp-out>`;
copy `IE64_ISA.pdf`, `IE32_ISA.pdf`, `iemon.pdf`, `iescript.pdf`, and
`architecture.pdf` from `<tmp-out>` back to `sdk/docs/`.
Observed result: The final render command wrote non-empty PDFs for all
five manuals after the five manual repairs from the initial pass, after
the `SDK-DOC-0041`, `SDK-DOC-0042`, `SDK-DOC-0043`, and
`SDK-DOC-0044`, `SDK-DOC-0045`, `SDK-DOC-0046`, `SDK-DOC-0047`, and
`SDK-DOC-0048`, `SDK-DOC-0049`, `SDK-DOC-0050`, `SDK-DOC-0051`,
`SDK-DOC-0052`, `SDK-DOC-0053`, `SDK-DOC-0054`, `SDK-DOC-0055`,
`SDK-DOC-0056`, `SDK-DOC-0057`, `SDK-DOC-0058`, `SDK-DOC-0059`,
`SDK-DOC-0060`, `SDK-DOC-0061`, `SDK-DOC-0062`,
`SDK-DOC-0063`, `SDK-DOC-0064`, `SDK-DOC-0065`,
`SDK-DOC-0066`, `SDK-DOC-0067`, `SDK-DOC-0068`, and
`SDK-DOC-0069`, `SDK-DOC-0070`, `SDK-DOC-0071`,
`SDK-DOC-0072`, `SDK-DOC-0073`, `SDK-DOC-0074`, and
`SDK-DOC-0075`, `SDK-DOC-0076`, `SDK-DOC-0077`, `SDK-DOC-0078`,
`SDK-DOC-0079`, `SDK-DOC-0080`, `SDK-DOC-0081`, `SDK-DOC-0082`,
`SDK-DOC-0083`, `SDK-DOC-0084`, `SDK-DOC-0085`,
`SDK-DOC-0086`, `SDK-DOC-0087`, `SDK-DOC-0088`,
`SDK-DOC-0089`, `SDK-DOC-0090`, `SDK-DOC-0091`, and
`SDK-DOC-0092` corrections and
rerun evidence, and after the focused SDK companion verification command
passed:
`go test -tags headless -run
'TestSDKISAInventory|TestSDKIEMonSourceInventory|TestSDKIEScriptSourceInventory|TestSDKArchitectureSourceInventory|TestSDKCompanionDocs'
.`
`IE64_ISA.pdf` (2222822 bytes), `IE32_ISA.pdf` (1011068 bytes),
`iemon.pdf` (758696 bytes), `iescript.pdf` (1134410 bytes), and
`architecture.pdf` (1016514 bytes).
The render command was:
`scripts/refman-pdf.sh --src "$tmp_src" --out "$tmp_out"` after copying
`00-Preface.md` and the five shipped Markdown files into `$tmp_src`,
then copying the five generated PDFs from `$tmp_out` to `sdk/docs/`.
The render pipeline used Google Chrome headless through
`scripts/refman-pdf.sh`. `SDK_DOC_PDF_RENDER_MANIFEST.sha256` records
950 SHA-256 rows covering root source files, audit tests, empirical
inventories, shipped manuals, the render script, and generated PDFs.
Action: Regenerated `sdk/docs/IE64_ISA.pdf`,
`sdk/docs/IE32_ISA.pdf`, `sdk/docs/iemon.pdf`,
`sdk/docs/iescript.pdf`, and `sdk/docs/architecture.pdf`.
Notes: The render command uses the repository PDF pipeline and
Google Chrome headless. If a later shipped manual, audit test, source
comment, generated table, or ledger hard gate changes, this entry must
be repeated and the PDFs regenerated again.
Disposition: KEEP.

## Current Run State

Status: FIXED.

The entries above record the scoped checks and fixes discovered during
the current run. The five shipped manuals have been audited claim group
by claim group against canonical source, focused tests, generated
tables, and runnable probes using the same discipline as the Programmer's
Reference Manual plan:

1. record the starting source state before non-blocking fixes
2. create one `OPEN` initial-review entry for each shipped manual before
   acting on review findings
3. perform an initial five-book diagnostic review and record the
   problems/gaps inventory before treating rewrites as complete
4. display the findings from each of the five book reviews in the run
   output before acting on them
5. adversarially challenge each book-level review by asking "Do you
   disagree with this review?", then correct weak findings before acting
6. record closure pointers for every agreed review finding; ledger-only
   closure is not enough
7. classify the manual into claim groups small enough to repeat
8. enumerate public surfaces from canonical source before trusting prose
9. read every line and classify it as technical claim, example,
   table/diagram data, cross-reference, structural prose, or style-only
   prose
10. verify each prose paragraph, table row, diagram fact, example,
   command, API, field, side effect, error case, and stated limitation
   against source, tests, generated tables, or runnable probes
11. compare the source-enumerated public surface against the manual and
   treat missing required coverage as an audit defect
12. judge each claim group against the manual's stated reader purpose;
   rewrite, add, split, retitle, replace, or remove material that is
   accurate but not fit for that purpose
13. remove or rewrite agent-facing audit/process language from shipped
   manuals so they read as user-facing references for experienced
   software engineers
14. remove, move, or archive material that is irrelevant, obsolete,
   duplicate, misleading, audience-mismatched, or owned by another
   maintained manual after source-checking any useful claims in it
15. remove references from the five shipped manuals to non-shipped books,
   support docs, plans, READMEs, external manuals, or unshipped
   documentation after source-checking and folding in any useful claims
16. fix drift in docs, implementation, tests, source comments, source
   notes, generated tables, and examples before closing a claim group
17. record one ledger entry per verified claim group
18. keep entries `OPEN` until all defects in that group are fixed and the
   relevant narrow verification has passed
19. stamp the current audit run's last-modified date on page 1 of each
   shipped manual
20. render the five PDFs only after the full source pass, ledger hard
   gates, and focused verification are complete; rerender after any later
   Markdown, source, source-comment, generated-table, test, or ledger
   hard-gate change

Open claim-group backlog: none for this run after `SDK-DOC-0092` and
the final PDF render gate in `SDK-DOC-0031`.

## Empirical ISA Source Inventory Gate

`sdk/docs/verify/SDK_ISA_SOURCE_AUDIT.md` is the mandatory source-fact
inventory for `sdk/docs/IE64_ISA.md` and `sdk/docs/IE32_ISA.md`.
Every non-heading line in that inventory must be empirically provable
from executable source code on disk. Source comments are not canonical.
Incorrect source comments must be fixed.

The ISA source inventory must not contain statuses, workflow labels,
TODO markers, inferred hardware claims, or review-result prose; the
assembler, disassembler, monitor, device, MMIO, and tooling claims are
out of scope for the ISA source inventory. Manual gaps and manual
mismatches are test failures, not table content.

Any future ISA-manual edit must preserve the generated inventory
comparison in `TestSDKISAInventoryGoldenMatchesExecutableSource` and the
manual coverage comparison in
`TestSDKISAInventoryManualCoverageMatchesSourceFacts`.
