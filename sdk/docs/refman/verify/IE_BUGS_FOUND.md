# IE bugs found by PRM doc-as-test harness

Author-only ledger. Stripped at publish time. **Temporary scratch
record** of defects in the embedded EhBASIC IE64 interpreter (or the
IE core) that the `tools/prm-extract` harness exposed while sweeping
the Programmer's Reference Guide.

**This file is not a parking lot for permanent workarounds.** The
governing rule (plan §7) is that when the harness exposes a real
IE/EhBASIC bug the implementation must be fixed before the chapter
progresses. Every entry written here must be closed before the sweep
is considered complete:

- An `OPEN` entry means the implementation fix is in flight; the
  reader-facing example in the chapter is still the original failing
  form and the chapter is still red.
- A `FIXED` entry records that the implementation now passes the
  original example unchanged; entry is kept as an audit trail of the
  defect the harness caught.
- A `WONTFIX` entry **requires explicit user approval**. The
  running agent must not promote an entry from `OPEN` to `WONTFIX`
  on its own initiative. `WONTFIX` is reserved for genuinely
  irreducible defects (e.g. inherited from upstream EhBASIC where
  the upstream-conformance contract is itself the load-bearing
  reason to preserve the behaviour). Each `WONTFIX` must cite the
  upstream reference and record the user sign-off in the entry's
  notes. Cost or expedience is never sufficient cause; if a fix
  feels too expensive, the entry stays `OPEN` and the sweep stays
  unfinished.

A leftover `OPEN` at the end of a sweep means the sweep itself is not
finished.

Format:

```
- id: IE-PRM-NNNN
  short: <one-line summary>
  status: OPEN|FIXED|WONTFIX
  found-by: PRM harness, sweep YYYY-MM-DD, source <path>:<line>
  component: <ehbasic-rom | ie-core | iemon | script-engine | ...>
  reproducer: |
    <minimum command sequence>
  expected: |
    <what the docs / standard EhBASIC says should happen>
  observed: |
    <what the harness saw>
  doc-side workaround: <pointer to chapter/line where the example was
    rewritten, or "none" if the doc still tests the failing form>
  fix-location-hint: <best guess at where the fix belongs, e.g.
    "ehbasic_ie64.ie64 program-mode FOR/NEXT execution path">
  notes: |
    <free-text context, links, suspected cause>
```

---

- id: IE-PRM-0001
  short: one-line `FOR ... :A(I)=expr:NEXT` in program mode produces zero-filled array
  status: FIXED
  found-by: PRM harness, sweep 2026-05-23, sdk/docs/refman/01-basic-rules.md (chapter 1 array intro)
  fixed-by: sdk/include/ehbasic_exec.inc `.next_loop_back` rewrite, sweep 2026-05-23
  component: ehbasic-rom (embedded image `sdk/examples/prebuilt/ehbasic_ie64.ie64`, built from sdk/include/ehbasic_*.inc via `make basic`)
  reproducer: |
    NEW
    CLEAR
    10 DIM A(10)
    20 FOR I=0 TO 10:A(I)=I*I:NEXT
    30 PRINT A(5)
    RUN
  expected: |
    25
    (FOR loop populates A(0..10) with I*I; PRINT A(5) yields 25.)
  observed-pre-fix: |
    0
    (Array remains zero-filled. Direct-mode variant
      `FOR I=0 TO 10:A(I)=I*I:NEXT`
    additionally reports `?NEXT WITHOUT FOR ERROR IN 0`, confirming
    the same root cause: NEXT was not re-entering the on-same-line
    loop body.)
  observed-post-fix: |
    25
    (Original example passes unchanged in both program-mode and
    direct-mode forms; chapter 1 reverted to the canonical one-line
    FOR/NEXT body. Multi-line FOR regression-tested with `RUN` on
      10 FOR I=1 TO 3
      20 PRINT I
      30 NEXT
    → 1\n2\n3, unchanged.)
  fix-location: |
    sdk/include/ehbasic_exec.inc, label `.next_loop_back`.

    Old behaviour: NEXT always set R14 to `(R25)` (the BASIC line
    following the FOR line) and returned R28=1 to the outer
    exec_next_line walker. That worked for multi-line FOR (body on
    later lines) but unconditionally skipped any body that sat on the
    FOR statement's own line behind a colon — and also bypassed the
    statement after NEXT on a one-liner, hence the
    `?NEXT WITHOUT FOR` from the second iteration when run in direct
    mode.

    New behaviour: NEXT restores R14 to the FOR statement's BASIC
    line pointer (R25) and R17 to the saved text ptr (R15, the byte
    immediately after the FOR/TO/STEP clause), then returns R28=0 so
    exec_line continues on the same line.
    - One-liner FOR: R17 lands on the ':' before the body, so
      exec_line walks the body and reaches the loop's NEXT again.
    - Multi-line FOR: R17 lands on the end-of-line null byte, so
      exec_line exits and the outer exec_next_line walker advances
      R14 to the first body line via `load.l r14, (r14)` exactly as
      before.
    Same code path drives both shapes; no special-case for the
    one-liner.
  rebuild: `make basic` (canonical target — `ie64asm` + ROM rebuild + Go binary with `-tags embed_basic`)
  notes: |
    Confirmed via chapter 01 PRM sweep: the case
      `RUN` against `10 DIM A(10) / 20 FOR I=0 TO 10:A(I)=I*I:NEXT /
       30 PRINT A(5)`
    transitions from FAIL to PASS while every other chapter 01 case
    that was already green stays green. Multi-line, nested, and
    STEP-bearing FOR loops verified by direct probe.

- id: IE-PRM-0002
  short: spaces around binary operators in expressions break operator recognition
  status: FIXED
  found-by: PRM harness, sweep 2026-05-23, sdk/docs/refman/01-basic-rules.md (chapter 1 comparisons-as-numbers example)
  fixed-by: sdk/include/ehbasic_expr.inc — added space-skip pre-checks to expr_compare, expr_add, expr_mul, expr_power and a space-skip at the head of expr_unary, sweep 2026-05-23
  component: ehbasic-rom (embedded image `sdk/examples/prebuilt/ehbasic_ie64.ie64`, built from sdk/include/ehbasic_*.inc via `make basic`)
  reproducer: |
    NEW
    CLEAR
    A=5:B=3
    PRINT 10 * (A>B)
  expected: |
    -10
    (`A>B` is TRUE → -1, multiplied by 10 → -10.)
  observed-pre-fix: |
    100-1
    (PRINT walked the items as three separate emissions because the
    arithmetic level (`expr_mul`) did not skip the space before
    checking for `*`. After `10` printed, PRINT saw the bare `*`
    token, re-entered expr_eval which could not parse it, and the
    `(A>B)` sub-expression then printed independently as `-1`.)
  observed-post-fix: |
    -10
    (Original spaced form passes unchanged; chapter 1 keeps the
    readable `PRINT 10 * (A>B)` shape. Same fix also rescues the
    general case — `PRINT 1 + 2`, `PRINT 2 ^ 10`, `PRINT A > B`
    with arbitrary whitespace around the operator all work now.)
  fix-location: |
    sdk/include/ehbasic_expr.inc.

    Root cause: the higher-precedence logical operators
    (expr_or, expr_and, expr_not_inline) already skipped spaces
    before checking for their operator token, but the lower-
    precedence arithmetic/comparison loops (expr_compare,
    expr_add, expr_mul, expr_power) did not. With tokenised input
    `10 [SP] TK_MULT [SP] (`, each arithmetic loop loaded the byte
    at R17, saw a space (0x20), failed the operator-equality
    check, and returned the left operand alone.

    Additionally, even after the arithmetic loops absorbed the
    leading space and consumed the operator, expr_unary did not
    skip whitespace before classifying the operand, so a right-
    hand side like `* (A>B)` (where the parenthesised expression
    has a leading space) re-introduced the same problem one level
    down.

    Fix: added a tight `load.b r1, (r17); cmp 0x20; advance; loop`
    space-skip block at the top of each affected loop
    (.cmp_loop / .add_loop / .mul_loop / .pow_loop) and at the
    head of expr_unary, matching the pattern already used by
    expr_or/expr_and/expr_not_inline.
  rebuild: `make basic` (canonical target — `ie64asm` + ROM rebuild + Go binary with `-tags embed_basic`)
  notes: |
    Confirmed via chapter 01 PRM sweep: the case
      `PRINT 10 * (A>B)`
    transitions from FAIL to PASS while every other chapter 01 case
    stays green (25/25). Manual probes on chapters 02 (`PRINT 12 AND
    10`, `PRINT 12 EOR 10`) still pass — the AND/OR/EOR space-skip
    that was already present is unchanged, and the new arithmetic
    space-skips do not introduce ambiguity with those operators.

- id: IE-PRM-0003
  short: IE32 monitor `m` bypasses bus — MMIO chip state never visible
  status: FIXED
  fixed-by: |
    debug_cpu_ie32.go DebugIE32.ReadMemory/WriteMemory rewritten
    to route through d.cpu.bus.Read8/Write8; debug_cpu_ie64.go
    ReadMemory/WriteMemory fast path now skips raw access when the
    target range overlaps an MMIO region (new helper
    DebugIE64.memoryRangeHasIO mirrors DebugM68K.memoryRangeHasIO).
    Confirmed against chapter 25 fixture: `m F0C31 1` now returns
    a row whose first byte is `01` (SN_PORT_READY ready bit, sourced
    from sn76489_chip.go HandleRead) instead of the pre-fix `00`
    RAM shadow. Remaining chapter 25 `m F0C31 1` diff is pure
    doc-format (count semantic + address-width digits) and is
    handled by the standard doc-bug correction pass, not by IE.
  found-by: PRM harness, sweep 2026-05-23, sdk/docs/refman/25-ie32.md (chapter 25 SN_PORT_READY readback at $F0C31)
  component: ie-core debug layer (`debug_cpu_ie32.go`, possibly also `debug_cpu_ie64.go` low-address fast path)
  reproducer: |
    Run the iemon child for chapter 25's chord example, halt at the
    self-loop, then:
      (ie32)> m F0C31 1
    Expected (per chapter 25 prose and per `sn76489_chip.go`
    HandleRead which always returns 1 for SN_PORT_READY):
      F0C31: 01
    Actual:
      000F0C31: 00 ...
  expected: |
    The monitor `m` command should reveal the live state of
    memory-mapped I/O registers, matching what an IE32 program would
    see if it executed a load from the same address.
  observed: |
    The monitor reads return zero (or any RAM-shadow value) instead
    of the chip's HandleRead response. Confirmed by tracing the read
    path: `debug_cpu_ie32.go:384` (`DebugIE32.ReadMemory`) returns
    `cpu.memory[start:start+size]` directly — a raw RAM slice. The
    chip's HandleRead is never consulted. Compare with
    `debug_cpu_x86.go:384` and `debug_cpu_z80.go:385`, both of which
    correctly route through `cpu.bus.Read`. The IE64 fast path
    (`debug_cpu_ie64.go`) has the same bypass for low addresses that
    fit inside the legacy memory window, so the same defect almost
    certainly affects IE64 chapters too once any IE64 example needs
    to inspect a chip status register through `m`.
  fix-location: |
    `debug_cpu_ie32.go` — `DebugIE32.ReadMemory` (and matching
    `WriteMemory`) must route through `d.cpu.bus.Read8` /
    `bus.Write8` so MMIO chips get their HandleRead / HandleWrite
    callbacks. Verify against the chapter 25 example and any
    additional ie32 chapter cases that read status registers via
    `m`. Repeat the same audit on `debug_cpu_ie64.go` and add the
    bus route for low addresses that overlap MMIO regions.
  rebuild: standard Go rebuild — `go build -tags embed_basic .`
    (the debug layer is part of the host binary, not the embedded
    EhBASIC ROM, so `make basic`'s asm step is not required).
  notes: |
    Not a CPU exec bug — the IE32 / IE64 CPU exec paths already use
    bus access for loads, and the issue is confined to the
    monitor's diagnostic ReadMemory shortcut. The CPU-core "fix in
    every interpreter and JIT" rule (plan §7) therefore does not
    apply here; this is a single debug-layer fix. Re-run chapters
    24, 25 (and any other IE32/IE64 iemon chapters) after the fix
    and confirm the chip-status reads now return the documented
    values.
