; ============================================================================
; IntuitionOS runtime-builder image
; ============================================================================
;
; This source is build-only. It assembles every standalone user-space runtime
; module into one flat image so the ELF rebuilder can export canonical hostfs
; artifacts without embedding those payloads into exec.library itself.
;
; Build: sdk/bin/ie64asm -I sdk/include -I sdk/intuitionos/iexec runtime_builder.s
;

include "iexec.inc"
include "ie64.inc"

include "handler/console_handler.s"
include "lib/dos_library.s"
