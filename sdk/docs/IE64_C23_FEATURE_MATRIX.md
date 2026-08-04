# IE64 C23 feature matrix

This matrix is the public support contract for `ie64-cproc`. It classifies the
freestanding IE64 compiler rather than implying hosted GCC or Clang
compatibility. The permanent tests live in the sibling cproc `test/` suite and
the emulator acceptance tests under `sdk/tests/ie64-cproc/`.

| Area | State | Constraint or diagnostic |
| --- | --- | --- |
| Declarations, scopes, tags and redeclarations | Implemented | Includes tag identity, qualifier preservation and per-declarator attributes. |
| Standard and GNU declaration attributes | Implemented with restrictions | The supported attributes are reported by `__has_attribute` and `__has_c_attribute`; unknown attributes are diagnosed. Custom allocatable function and data sections retain their executable, read-only or writable placement class. |
| Integer, pointer, aggregate and scalar floating expressions | Implemented | The ABI is LP64, little-endian and specified in `IE64_ABI_V3.md`. |
| Preprocessing, macro expansion, `#if` and token paste | Implemented | Invalid pasted tokens, unterminated conditionals and arithmetic errors are diagnosed. |
| `#include` and `#embed` | Implemented with restrictions | Include nesting and `#embed` parameters are checked; filesystem access is confined to the selected include paths. |
| Date and time predefined macros | Implemented | Local time is used normally; `SOURCE_DATE_EPOCH` makes output reproducible. |
| IE64 private builtins and atomics | Implemented | Available only for the IE64 target and type-checked before lowering. |
| `_Atomic` pointer fetch and compare-exchange | Implemented | Object alignment and expected-pointer operands are validated; runtime memory orders are conservatively sequentially consistent. |
| Optimisation options `-O0` to `-O3` | Implemented with restrictions | `-O3` is the documented full `-O2` pipeline alias until another target pass exists. |
| TLS, hosted POSIX, shared libraries, PIE and C++ | Diagnosed as unsupported | These are outside the freestanding IE64 compiler suite. |
| Unsupported compiler switches | Diagnosed as unsupported | The driver rejects meaning-changing options instead of ignoring them. |
| Non-C23 or implementation-extension syntax | Unrecognised | It is rejected by normal lexical or parser diagnostics. |
