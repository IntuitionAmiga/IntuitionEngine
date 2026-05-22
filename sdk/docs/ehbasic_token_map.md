# EhBASIC IE64 Token Map

This ledger records the current one-byte token space used by the IE64 EhBASIC
port and the migration choice for planned parser fixes.

## Findings

The token space from `0x80` through `0xFF` is fully assigned. There are no free
direct-token slots for `<=`, `>=`, or `<>`. `ELSE` now uses a repurposed
dead slot.

`0x80` through `0xE1` are the original EhBASIC-compatible tokens. `0xE2`
through `0xFF` are hardware extension tokens. `0xFF` is already assigned to
`TK_TEXTURE`, so a single-byte prefix escape cannot be introduced without
changing the token stream format.

Current aliases and problematic bindings:

| Source | Token | Notes |
| --- | --- | --- |
| `ELSE` | `TK_ELSE` (`0xAB`) | Repurposes the previously unimplemented `SPC` token slot. |
| `TROFF` | `TK_DEF` (`0x97`) | Kept as a legacy tokenizer alias; the `DEF` handler treats bare `TK_DEF` without `FN` as `TROFF`. |
| `BLOAD` | `TK_WIDTH` (`0xA3`) | Reuses `WIDTH`; no distinct BLOAD statement token. |
| `DIR` | none | Immediate REPL command only; avoids consuming or renumbering the full one-byte token space. |
| `WEND` | `TK_UNTIL` (`0xAF`) | Existing implementation treats this as a loop terminator alias. |
| `TRON` | `TK_NULL` (`0x92`) | Existing implementation treats this as trace-on. |
| `HOST` | none | Not tokenized. Recognized as a raw statement in `exec_line` using the same word-boundary technique as `COSTART` / `COSTOP` / `COWAIT`. A previous draft assigned `HOST` the same byte (`0xDE`) as `TK_VPTR`, which collided with `VARPTR` in expression context; that draft has been retired. |
| `VARPTR` | `TK_VPTR` (`0xDE`) | Function token. Sole owner of `0xDE`. If `VARPTR` is used in statement position the dispatcher routes it to `exec_do_unknown` (it has no statement semantics). |

## Chosen Scheme

Use the raw-lex composite operator scheme for numeric comparison operators:

| Source | Tokenized bytes |
| --- | --- |
| `<` | `TK_LT` |
| `>` | `TK_GT` |
| `<=` | `TK_LT`, raw `=` |
| `>=` | `TK_GT`, raw `=` |
| `<>` | `TK_LT`, raw `>` |

This preserves the existing `TK_LT` and `TK_GT` token IDs and avoids spending
new token slots for comparison operators.

For `ELSE`, the token audit found `SPC` was tokenized but not implemented by the
executor or expression evaluator. The port repurposes `0xAB` as `TK_ELSE`, so
`THEN` and `ELSE` are no longer ambiguous in the statement stream.

## Migration Notes

The `.ie64` BASIC image is a raw byte image, not a versioned tokenized-program
container. If a future phase renumbers tokens, the migration must be procedural:

1. Update token constants and keyword tables together.
2. Rebaseline tokenizer/detokenizer tests.
3. Rebuild `sdk/examples/asm/ehbasic_ie64.ie64`.
4. Regenerate `sdk/examples/prebuilt/ehbasic_ie64.ie64`.
5. Bump the EhBASIC banner so stale interpreter images are visible to users.

Source `.bas` files are unaffected because they are tokenized on load.

## HOST / VARPTR Compatibility

`0xDE` is the token byte for `TK_VPTR` only. `VARPTR(...)` in expression
position evaluates the variable-address function. `VARPTR` used as a
statement (it has no statement semantics) routes through the statement
dispatcher to `exec_do_unknown`.

`HOST` is not tokenized. It is recognized as a raw statement in
`exec_line` using the same word-boundary technique as `COSTART`,
`COSTOP`, and `COWAIT`. An earlier draft of this port assigned `HOST`
the same byte (`0xDE`) as `TK_VPTR` and relied on context-sensitive
dispatch to disambiguate them. That draft has been retired: it caused
`HOST` in expression context to be parsed as `VARPTR` and `VARPTR` in
statement position to invoke the `HOST` bridge. Untokenized raw
recognition removes the collision without renumbering the one-byte
token space.

## R28 Runtime-Error Audit

EhBASIC IE64 uses `R28` as the internal statement-control channel and returns
the final status from `exec_line` in `R8`. Runtime errors now use `R28=3`.

The GOSUB stack is also the structured-control stack. Plain GOSUB frames store a
return line pointer, and FOR frames store the `"FOR "` marker, variable slot,
limit, step, and loop line pointers. These frames do not own string heap roots.
Resetting `ST_GOSUB_SP` to `BASIC_GOSUB_STACK` after `R8=3` therefore clears
GOSUB, FOR, WHILE, and DO recovery state together.

Statement sites that can raise errors unwind through their local epilogues
before returning to `exec_line`. Expression and helper failures propagate by
leaving `R28=3`; callers that have saved registers branch through their normal
cleanup labels before `rts`.

## String-Heap GC Audit

String variables are durable roots in this port: each string variable table
entry holds the live heap pointer. GOSUB/FOR/WHILE/DO frames do not contain
string descriptors. String expression temporaries that are live across an
allocation are pushed onto the temporary GC root stack and are copied before
durable variable roots, so compaction cannot overwrite register-held strings.

When `str_alloc` cannot fit a new allocation, `gc_strings` compacts all live
temporary and string variable values back to `BASIC_STR_TEMP`, updates their
root slots, and then retries the allocation. If the compacted heap still cannot
fit the request, `raise_error(ERR_OOM)` reports `?OUT OF MEMORY ERROR`.
