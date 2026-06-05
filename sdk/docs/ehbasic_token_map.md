# IE64 BASIC Token Map

This ledger records the current one-byte token space used by the IE64 EhBASIC
port and the migration choice for planned parser fixes.

## Findings

The token space now uses `TK_EXT` (`0x92`) followed by a subtoken for IE64 BASIC
extensions that no longer fit cleanly in the historical one-byte map.

`0x80` through `0xE1` are the original EhBASIC-compatible tokens. `0xE2`
through `0xFF` are hardware extension tokens retained for compatibility where
they already exist.

Current aliases and problematic bindings:

| Source | Token | Notes |
| --- | --- | --- |
| `ELSE` | `TK_ELSE` (`0xAB`) | Repurposes the previously unimplemented `SPC` token slot. |
| `TRON` | `TK_EXT`, `EXT_TRON` | Extended trace-on token. |
| `TROFF` | `TK_EXT`, `EXT_TROFF` | Extended trace-off token. |
| `BLOAD` | `TK_WIDTH` (`0xA3`) | Reuses `WIDTH`; no distinct BLOAD statement token. |
| `DIR` | none | Immediate REPL command only; avoids consuming or renumbering the full one-byte token space. |
| `WEND` | `TK_UNTIL` (`0xAF`) | Existing implementation treats this as a loop terminator alias. |
| `HOST` | none | Not tokenized. Recognized as a raw statement in `exec_line` using the same word-boundary technique as `COSTART` / `COSTOP` / `COWAIT`. A previous draft assigned `HOST` the same byte (`0xDE`) as `TK_VPTR`, which collided with `VARPTR` in expression context; that draft has been retired. |
| `VARPTR` | `TK_VPTR` (`0xDE`) | Function token. Sole owner of `0xDE`. If `VARPTR` is used in statement position the dispatcher routes it to `exec_do_unknown` (it has no statement semantics). |

## Chosen Scheme

Use extended tokens for IE64 BASIC extensions:

| Source | Tokenized bytes |
| --- | --- |
| `TRON` | `TK_EXT`, `EXT_TRON` |
| `TROFF` | `TK_EXT`, `EXT_TROFF` |
| `MEMALLOC` | `TK_EXT`, `EXT_MEMALLOC` |
| `POKE8`, `POKE16`, `POKE32`, `POKE64` | `TK_EXT`, width subtoken |
| `PEEK8`, `PEEK16`, `PEEK32`, `PEEK64` | `TK_EXT`, width subtoken |

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

IE64 BASIC uses `R28` as the internal statement-control channel and returns
the final status from `exec_line` in `R8`. Runtime errors now use `R28=3`.

GOSUB frames grow upward from `ST_CTRL_LOW`; FOR, WHILE, and DO frames grow
downward from `ST_CTRL_HIGH`. Resetting the published control-flow cursors clears
GOSUB, FOR, WHILE, and DO recovery state together.

Statement sites that can raise errors unwind through their local epilogues
before returning to `exec_line`. Expression and helper failures propagate by
leaving `R28=3`; callers that have saved registers branch through their normal
cleanup labels before `rts`.

## String-Heap GC Audit

String variable records are pinned dynamic owner records. Each record holds the
internal string pointer plus reserved metadata fields. `SADD` returns the live
string-data pointer; direct byte writes through that pointer mutate the string
variable.
