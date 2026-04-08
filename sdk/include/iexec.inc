; iexec.inc - IExec Microkernel Constants
;
; For use with ie64asm assembler
; Include with: include "iexec.inc"

; ===========================================================================
; Syscall Numbers (IExec contract)
; ===========================================================================
;
; Category tags (M11.5 — see sdk/docs/IntuitionOS/IExec.md "Exec Boundary"):
;
;   nucleus   — privileged primitive, will exist forever
;   bootstrap — kernel bring-up / panic only, not part of normal app model
;   legacy    — kept for compatibility, scheduled for removal in a future milestone
;
; A new syscall is justified only if (1) it requires the kernel's privileged
; state and (2) it cannot be expressed as a message to a user-space service
; without a bootstrap deadlock. Otherwise the feature belongs in a library,
; device, handler, or resource — not in the syscall surface.
;
; Reserved holes are former syscalls removed in M11.5. Their numbers are
; preserved (never reused) so existing IE64 binaries that called these
; numbers continue to fail in the same predictable way (ERR_BADARG via the
; dispatcher's fall-through path).

SYS_ALLOC_MEM     equ 1            ; nucleus
SYS_FREE_MEM      equ 2            ; nucleus
; (slot 3 reserved — was SYS_ALLOC_SHARED, removed M11.5: never had a handler)
SYS_MAP_SHARED    equ 4            ; nucleus
SYS_CREATE_TASK   equ 5            ; nucleus
; (slot 6 reserved — was SYS_DELETE_TASK, removed M11.5: never had a handler)
; (slot 7 reserved — was SYS_FIND_TASK, removed M11.5: never had a handler)
; (slot 8 reserved — was SYS_SET_TASK_PRI, removed M11.5: never had a handler)
; (slot 9 reserved — was SYS_SET_TP, removed M11.5: never had a handler)
; (slot 10 reserved — was SYS_GET_TASK_INFO, removed M11.5: never had a handler)
SYS_ALLOC_SIGNAL  equ 11           ; nucleus
SYS_FREE_SIGNAL   equ 12           ; nucleus
SYS_SIGNAL        equ 13           ; nucleus
SYS_WAIT          equ 14           ; nucleus
SYS_CREATE_PORT   equ 15           ; nucleus
SYS_FIND_PORT     equ 16           ; nucleus
SYS_PUT_MSG       equ 17           ; nucleus
SYS_GET_MSG       equ 18           ; nucleus
SYS_WAIT_PORT     equ 19           ; nucleus
SYS_REPLY_MSG     equ 20           ; nucleus
; (slot 21 reserved — was SYS_PEEK_PORT, removed M11.5: never had a handler)
; (slot 22 reserved — was SYS_ADD_TIMER, removed M11.5: never had a handler)
; (slot 23 reserved — was SYS_REM_TIMER, removed M11.5: never had a handler)
; (slot 24 reserved — was SYS_CLOSE_HANDLE, removed M11.5: never had a handler)
; (slot 25 reserved — was SYS_DUP_HANDLE, removed M11.5: never had a handler)
SYS_YIELD         equ 26           ; nucleus
SYS_GET_SYS_INFO  equ 27           ; nucleus
SYS_MAP_IO        equ 28           ; bootstrap, trusted-internal — gated by KD_GRANT_TABLE (M12.5)
                                   ; M12.5 de-publicized: caller must hold a grant entry in
                                   ; the kernel grant chain for the requested PPN range. The
                                   ; only producer of grants is hardware.resource (or the
                                   ; immutable bootstrap-grant table for console.handler).
; (slot 29 reserved — was SYS_MAP_VRAM, removed M11.5: superseded by SYS_MAP_IO range mapping)
; (slot 30 reserved — was SYS_DEBUG, removed M11.5: never had a handler)
; (slot 31 reserved — was SYS_SEND_MSG_BULK, removed M11.5: never had a handler)
; (slot 32 reserved — was SYS_RECV_MSG_BULK, removed M11.5: never had a handler)
SYS_DEBUG_PUTCHAR equ 33           ; bootstrap (kernel bring-up / panic only, not for app code)
SYS_EXIT_TASK     equ 34           ; nucleus
SYS_EXEC_PROGRAM  equ 35           ; nucleus  (M11.6: legacy R1<0x600000 index branch removed)
SYS_OPEN_LIBRARY  equ 36           ; nucleus  (binary-compat redirect to SYS_FIND_PORT — see iexec.s .do_open_library)
; (slot 37 reserved — was SYS_READ_INPUT, removed M11.5: terminal MMIO inlined into console.handler via SYS_MAP_IO)
; M12.5 contract: slot 37 stays a reserved hole forever. The M11.5 admission rule
; explicitly forbids reusing removed-syscall slots so old IE64 binaries continue
; to fail predictably with ERR_BADARG. New syscalls use fresh slots above the
; current ABI ceiling. TestIExec_HWRes_Slot37StillReserved is the executable
; contract.
SYS_HWRES_OP      equ 38           ; trusted-internal — verb-multiplexed hardware.resource broker primitive (M12.5)
                                   ; R0 selects: HWRES_BECOME / HWRES_CREATE / HWRES_REVOKE
                                   ; Net public ABI shrinks: SYS_MAP_IO leaves the public nucleus set
                                   ; in M12.5, one trusted-internal slot enters. Apps see one fewer
                                   ; public syscall after M12.5, not one more.

; ===========================================================================
; Error Codes
; ===========================================================================

ERR_OK            equ 0
ERR_NOMEM         equ 1
ERR_BADHANDLE     equ 2
ERR_BADARG        equ 3
ERR_NOTFOUND      equ 4
ERR_PERM          equ 5
ERR_AGAIN         equ 6
ERR_TOOLARGE      equ 7
ERR_EXISTS        equ 8
ERR_FULL          equ 9

; ===========================================================================
; Fault Causes
; ===========================================================================

FAULT_NOT_PRESENT equ 0
FAULT_READ_DENIED equ 1
FAULT_WRITE_DENIED equ 2
FAULT_EXEC_DENIED equ 3
FAULT_USER_SUPER  equ 4
FAULT_PRIV        equ 5
FAULT_SYSCALL     equ 6
FAULT_MISALIGNED  equ 7
FAULT_TIMER       equ 8

; ===========================================================================
; GetSysInfo query IDs
; ===========================================================================

SYSINFO_TOTAL_PAGES equ 0
SYSINFO_FREE_PAGES  equ 1
SYSINFO_TICK_COUNT  equ 2
SYSINFO_CURRENT_TASK equ 3

; ===========================================================================
; Kernel Memory Layout
; ===========================================================================

; M12 redesign: KERN_PAGE_TABLE and KERN_DATA_BASE were originally at
; 0x010000 / 0x020000 respectively, leaving the kernel binary only
; ~60 KiB of code/data space between PROG_START (0x1000) and the page
; table. As more user-space services accumulated in the kernel binary
; (M11 graphics.library + M11 GfxDemo + M12 intuition.library + M12
; About app + M12 embedded Topaz fonts), the binary grew past 60 KiB
; and silently overwrote the page table during phase 3 PTE init,
; corrupting kernel mappings before the boot banner could even print.
; The page table now lives at 0x040000, giving the kernel binary a
; ~252 KiB code/data window — comfortable headroom for future
; embedded services and fonts.
KERN_PAGE_TABLE   equ 0x040000    ; Kernel page table (64 KiB) — M12: was 0x010000
KERN_DATA_BASE    equ 0x050000    ; Kernel data structures      — M12: was 0x020000
KERN_STACK_TOP    equ 0x09F000    ; Kernel stack top

; ---------------------------------------------------------------------------
; Kernel Data Offsets (at KERN_DATA_BASE)
; ---------------------------------------------------------------------------
; Layout (M12.6 Phase D: MAX_TASKS=32, KD_PORT_MAX=32, KD_SHMEM_MAX=16, PORT_NAME_LEN=32):
;   +0:     current_task (8)
;   +8:     tick_count (8)
;   +16:    num_tasks (8)
;   +24:    nonce_counter (8)
;   +32:    name_scratch (32 bytes — KD_NAME_SCRATCH)        → ends at 64
;   +64:    TCB[0..31] (32 x 32 = 1024 bytes)                → ends at 1088
;   +1088:  PTBR[0..31] (32 x 8 = 256 bytes)                 → ends at 1344
;   +1344:  Port[0..31] (32 x 168 = 5376 bytes)              → ends at 6720
;   +6720:  Page bitmap (800 bytes)                          → ends at 7520
;   +7520:  Region table (32 tasks x 8 x 16 = 4096 bytes)    → ends at 11616
;   +11616: Shared object table (16 x 16 = 256 bytes)        → ends at 11872
;   +11872: KD_HWRES_TASK (1 byte)                           → ends at 11873
;   +11880: KD_GRANT_TABLE_HDR (8 bytes)                     → ends at 11888
;   +11888: KD_REGION_OVERFLOW_HEAD (32 x 8 = 256 bytes)     → ends at 12144
;   +12144: KD_SHMEM_OFLOW_HDR (8 bytes)                     → ends at 12152
;   +12152: KD_PORT_OFLOW_HDR (8 bytes)                      → ends at 12160
;
; M12.6 Phase D doubled MAX_TASKS from 16 to 32 (with matching layout
; bumps). Per-task structures (TCB, PTBR, region table, region overflow
; header array) all doubled in size, pushing every kernel data field
; after them down the page. The kernel data area (~512 KiB available
; between KERN_DATA_BASE and KERN_STACK_TOP) easily absorbs the new
; ~12 KiB layout. The user-space slot region (USER_CODE_BASE..USER_PT_BASE)
; was widened from 1 MiB to 2 MiB to fit 32 task code/stack/data slots,
; and USER_PT_BASE/USER_PT_PAGE_BASE/END/USER_DYN_BASE all shifted up by
; 1 MiB (PT region) + 1 MiB (extra code region) = 2 MiB.
;
; A future milestone may bump MAX_TASKS further by widening the slot
; region again (or by switching to a non-slot user-space layout). The
; current cap is bound by the slot layout, not by an arbitrary kernel
; constant — see IExec.md §5.13.1 for the audit row.

KD_CURRENT_TASK   equ 0           ; uint64: current task index
KD_TICK_COUNT     equ 8           ; uint64: tick counter
KD_NUM_TASKS      equ 16          ; uint64: number of active tasks
KD_NONCE_COUNTER  equ 24          ; uint64: monotonic counter for share handle nonces
KD_NAME_SCRATCH   equ 32          ; 32-byte scratch buffer for port name operations (M12: was 16)

; ---------------------------------------------------------------------------
; Task Control Block (TCB) — 32 bytes per task
; ---------------------------------------------------------------------------
; Address: KERN_DATA_BASE + KD_TASK_BASE + (task_id * KD_TASK_STRIDE)

; MAX_TASKS is bounded by the user-space slot region size, NOT by an
; arbitrary kernel constant. Each task gets one 64 KiB slot in the
; USER_CODE_BASE..USER_PT_BASE region (= 2 MiB after M12.6 Phase D, fits
; 32 slots) and one 64 KiB slot in the USER_PT_BASE..USER_DYN_BASE region
; (= 2 MiB after M12.6 Phase D, fits 32 PT slots). To raise the cap above
; 32 in a future milestone, both regions need to grow (which would shift
; USER_DYN_BASE further into the 32 MiB user address space).
;
; History: M11 capped this at 8 because the dynamic-VA layout used a per-
; task USER_DYN_STRIDE that saturated the 32 MiB VA space at 8 tasks; M12
; removed the per-task DYN cap by making the dynamic range global and
; per-task-PT-isolated (see USER_DYN_BASE/USER_DYN_END below) and bumped
; MAX_TASKS to 16. M12.6 Phase D bumps to 32 by widening the slot region
; from 1 MiB to 2 MiB.
MAX_TASKS         equ 32          ; M12.6 Phase D: bumped from 16 (M12 bumped from 8)

KD_TASK_BASE      equ 64          ; offset of task state array
KD_TASK_STRIDE    equ 32          ; bytes per task entry

; Per-task field offsets (relative to TCB base)
KD_TASK_PC        equ 0           ; saved PC (8 bytes)
KD_TASK_USP       equ 8           ; saved user SP (8 bytes)
KD_TASK_SIG_ALLOC equ 16          ; allocated signal bits (4 bytes)
KD_TASK_SIG_WAIT  equ 20          ; signals being waited for (4 bytes, 0 if not waiting)
KD_TASK_SIG_RECV  equ 24          ; pending received signals (4 bytes)
KD_TASK_STATE     equ 28          ; task state byte
KD_TASK_WAITPORT  equ 29          ; WaitPort port ID (0xFF = not in WaitPort)
KD_TASK_GPR_SAVED equ 30          ; 1 = GPRs saved on user stack by timer (M9)

; Task state constants
TASK_READY        equ 0
TASK_RUNNING      equ 1
TASK_WAITING      equ 2
TASK_FREE         equ 3           ; slot is unused and reusable

; Signal bit constants
SIGF_PORT         equ 1           ; Bit 0: message arrived at a port
; WaitPort sentinel
WAITPORT_NONE     equ 0xFF        ; not blocked in WaitPort

; System signal allocation mask (bits 0-15 pre-allocated)
SIG_SYSTEM_MASK   equ 0xFFFF

; ---------------------------------------------------------------------------
; PTBR Array — 8 bytes per task
; ---------------------------------------------------------------------------
; Address: KERN_DATA_BASE + KD_PTBR_BASE + (task_id * 8)

KD_PTBR_BASE      equ 1088        ; offset of PTBR array (after 32 TCBs) — M12.6 Phase D: was 576

; ===========================================================================
; Message Port Layout (M7, M12: PORT_NAME_LEN 16→32, KD_PORT_MAX 8→16)
; ===========================================================================
; 16 ports max, 168 bytes each, starting at offset 384 in kernel data.
; Each port: 40-byte header (with 32-byte name) + 4 × 32-byte messages (FIFO).
;
; M12 changes:
;   - PORT_NAME_LEN bumped from 16 to 32 so user-space service names can use
;     the full Amiga-style "intuition.library" (17 chars) and longer rather
;     than being silently truncated.
;   - KD_PORT_MAX bumped from 8 to 16 because the M11 cap was already tight
;     (5 public service ports + each running task's anonymous reply ports)
;     and intuition.library adds another public port plus two anonymous ones
;     of its own. 16 gives comfortable headroom; further bumps may follow
;     when the table becomes a heap-allocated linked list.
;
; Layout shifts that follow from these bumps: every kernel-data field after
; the port table moves down by (16*168 - 8*160) = 1408 bytes vs the M11
; layout. KD_PAGE_BITMAP moves from 1664 to 3072, KD_REGION_TABLE from
; 2464 to 3872, KD_SHMEM_TABLE from 3488 to 4896.

; M12.6 Phase C: KD_PORT_MAX is no longer a hard cap.
; The first KD_PORT_INLINE_MAX rows still live inline at KD_PORT_BASE
; (preserving the existing fast path and the row layout). Rows 32+
; live in allocator-backed chain pages reached through a single global
; overflow header at KD_PORT_OFLOW_HDR. Walkers iterate inline first,
; then walk the overflow chain. The "find free slot" allocator falls
; through to a new chain page when every existing row is occupied.
;
; Port IDs are stored as 1 byte in KD_PORT_OWNER and KD_TASK_WAITPORT
; (with WAITPORT_NONE = 0xFF as the sentinel), so the M12.6 ABI ceiling
; for total ports is 255 (0xFF reserved). 255 - 32 inline = 223 overflow
; slots, fitting in ceil(223/24) = 10 chain pages of 24 ports each.
KD_PORT_BASE          equ 1344    ; start of inline port array (M12.6 Phase D: was 704)
KD_PORT_STRIDE        equ 168     ; bytes per port (40 header + 4×32 messages)
KD_PORT_INLINE_MAX    equ 32      ; rows stored inline (M12.6: was the cap KD_PORT_MAX)
; Legacy alias for any assembly code that still references the old name.
; Walkers no longer treat it as a hard cap.
KD_PORT_MAX           equ 32
KD_PORT_FIFO_SIZE     equ 4       ; messages per port FIFO

; M12.6 Phase C: port overflow chain — single global header (not per-task).
KD_PORT_OFLOW_HDR        equ 12152 ; chain header (8 bytes) — M12.6 Phase D: was 9336
KD_PORT_OFLOW_FIRST_PPN  equ 0     ; field offset within header (2 bytes)
KD_PORT_OFLOW_TOTAL      equ 2     ; total overflow rows allocated so far (2 bytes)
KD_PORT_OFLOW_PAGES      equ 4     ; chain page count (2 bytes)
KD_PORT_OFLOW_RESV       equ 6     ; reserved (2 bytes)

; Port chain page layout (4 KiB allocator-backed page).
; Same chain-page format as M12.5 region overflow / M12.6 shmem overflow:
;   [0..1]   = next chain page PPN (0 = end of chain)
;   [2..15]  = reserved
;   [16..]   = 24 ports × 168 bytes = 4032 bytes; 48 bytes trailing slack
KD_PORT_PAGE_HDR_SZ   equ 16
KD_PORT_PAGE_NEXT     equ 0
KD_PORT_ROWS_PER_PG   equ 24      ; (4096 - 16) / 168 = 24

; Port header field offsets (relative to port base, 40-byte header)
KD_PORT_VALID     equ 0           ; 1=active, 0=unused (1 byte)
KD_PORT_OWNER     equ 1           ; owning task ID (1 byte)
KD_PORT_COUNT     equ 2           ; messages in FIFO, 0-4 (1 byte)
KD_PORT_HEAD      equ 3           ; read index, 0-3 (1 byte)
KD_PORT_TAIL      equ 4           ; write index, 0-3 (1 byte)
KD_PORT_FLAGS     equ 5           ; port flags (PF_PUBLIC etc.) (1 byte)
KD_PORT_NAME      equ 8           ; port name, 32 bytes, zero-padded ASCII
KD_PORT_MSGS      equ 40          ; start of message array (4 × 32 bytes)

; Port flags
PF_PUBLIC         equ 1           ; port is publicly discoverable via FindPort
PORT_NAME_LEN     equ 32          ; max port name length (bytes) — M12: bumped from 16

; Message field offsets (relative to message base, 32 bytes per message)
KD_MSG_TYPE       equ 0           ; application-defined type (4 bytes)
KD_MSG_SRC        equ 4           ; sender task ID (4 bytes)
KD_MSG_DATA0      equ 8           ; payload data word 0 (8 bytes)
KD_MSG_DATA1      equ 16          ; payload data word 1 (8 bytes)
KD_MSG_REPLY_PORT equ 24          ; reply port ID, 0xFFFF=none (2 bytes)
KD_MSG_SHARE_HDL  equ 26          ; share handle, 0=none (4 bytes)
KD_MSG_SIZE       equ 32          ; bytes per message

; Reply port sentinel
REPLY_PORT_NONE   equ 0xFFFF      ; no reply port

; ===========================================================================
; User Space Layout (M5: slot-based, 8 tasks)
; ===========================================================================
; Task i page table: USER_PT_BASE + i * USER_SLOT_STRIDE
; Task i code page:  USER_CODE_BASE + i * USER_SLOT_STRIDE
; Task i stack page: USER_STACK_BASE + i * USER_SLOT_STRIDE
; Task i data page:  USER_DATA_BASE + i * USER_SLOT_STRIDE
; Task i code VPN:   USER_CODE_VPN_BASE + i * USER_VPN_STRIDE

; USER_PT_BASE was originally 0x100000 but that physical range is mapped by
; the host VideoChip MMIO (VRAM at $100000-$5FFFFF). Kernel PTE writes to the
; old base were dispatched to the framebuffer instead of RAM, leaving zero-
; alpha "pixels" that the compositor skipped, producing a black trailing line
; on every kernel-printed boot banner row. The PTs are now placed in the gap
; between the user code/stack/data slot block ($600000-$6FFFFF) and the
; page allocator pool ($800000+). MAX_TASKS * USER_SLOT_STRIDE =
; 16 * 0x10000 = 0x100000, so the region exactly fills $700000-$7FFFFF.
;
; M12: USER_PT_BASE shifted from 0x680000 to 0x700000 because MAX_TASKS
; doubled from 8 to 16 (slot region grew from 0x80000 to 0x100000).
USER_PT_BASE      equ 0x800000    ; Task page tables (each 64 KiB) — M12.6 Phase D: was 0x700000.
                                  ; Moved up by 1 MiB (one slot region width) to make
                                  ; room for the doubled USER_CODE_BASE region. The PT
                                  ; region now occupies pages 0x800..0xA00 (32 slots ×
                                  ; 16 pages). The allocator pool was shifted from
                                  ; page 0x800 to page 0xA00 to make room — see
                                  ; ALLOC_POOL_BASE / ALLOC_POOL_PAGES below.
USER_CODE_BASE    equ 0x600000    ; Task code pages (physical)
USER_STACK_BASE   equ 0x601000    ; Task stack pages
USER_DATA_BASE    equ 0x602000    ; Task data pages
USER_SLOT_STRIDE  equ 0x10000     ; 64 KiB between task slots
USER_CODE_SIZE    equ 192         ; DEPRECATED: M7 boot template size (removed in M8)

; VPN bases (addr >> 12)
USER_CODE_VPN_BASE  equ 0x600    ; 0x600000 >> 12
USER_VPN_STRIDE     equ 0x10     ; 0x10000 >> 12

; Page-number bounds for the user-PT region. Kept in sync with USER_PT_BASE
; and MAX_TASKS * USER_SLOT_STRIDE so the kernel boot can extend the kernel
; PT to cover this region (see iexec.s phase 3b' kern_userpt_map).
USER_PT_PAGE_BASE   equ 0x800    ; USER_PT_BASE >> MMU_PAGE_SHIFT — M12.6 Phase D: was 0x700
USER_PT_PAGE_END    equ 0xA00    ; (USER_PT_BASE + MAX_TASKS * USER_SLOT_STRIDE) >> MMU_PAGE_SHIFT — M12.6 Phase D: was 0x800

; ===========================================================================
; PTE Flags
; ===========================================================================

PTE_P             equ 0x01        ; Present
PTE_R             equ 0x02        ; Read
PTE_W             equ 0x04        ; Write
PTE_X             equ 0x08        ; Execute
PTE_U             equ 0x10        ; User-accessible
PTE_A             equ 0x20        ; Accessed
PTE_D             equ 0x40        ; Dirty

; ===========================================================================
; MMU Constants
; ===========================================================================

MMU_PAGE_SIZE     equ 0x1000      ; 4 KiB
MMU_PAGE_SHIFT    equ 12
MMU_NUM_PAGES     equ 8192
KERN_PAGES        equ 384         ; Pages 0-383 are kernel-mapped

; ===========================================================================
; Memory Allocation (M6)
; ===========================================================================

; MEMF_ flags (Amiga-style)
MEMF_ANY          equ 0x00000     ; any memory (private, the default)
MEMF_PUBLIC       equ 0x00001     ; bit 0: shareable (classic Amiga)
MEMF_CLEAR        equ 0x10000     ; bit 16: zero-fill (classic Amiga)

; Physical page allocator
; M12.6 Phase E security fix: the allocator pool VPN range MUST be disjoint
; from the user-dynamic VA range (USER_DYN_BASE..USER_DYN_END below).
; Previously they overlapped (both 0xA00..0x1FFF), which let an unprivileged
; task overwrite the supervisor-only pool PTEs that build_user_pt copies into
; every user PT — by SYS_ALLOC_MEM picking a user-dyn VA whose VPN aliased a
; pool PPN. After overwriting, kernel chain walkers running on the user PT
; would dereference attacker-controlled memory, yielding arbitrary kernel
; read/write. The fix is to give the user-dyn window the BOTTOM half of the
; old combined window (VPN 0xA00..0x11FF, 8 MiB) and move the allocator pool
; UP to the TOP half (VPN 0x1200..0x1FFF, 14 MiB). User AllocMem calls now
; map fresh pool PPNs at user-dyn VPNs that are NOT in the pool VPN range,
; so the supervisor copies are never overwritten.
ALLOC_POOL_BASE   equ 0x1200      ; first allocable page number — M12.6 Phase E security fix: was 0xA00 (split user-dyn and pool into disjoint VPN ranges)
ALLOC_POOL_PAGES  equ 3584        ; pages 0x1200-0x1FFF — M12.6 Phase E: was 5632 (lost 2048 pages = 8 MiB to the user-dyn VA window so the two ranges are disjoint)

; Dynamic VA layout (M12: GLOBAL shared range, no per-task windowing).
;
; All tasks share USER_DYN_BASE..USER_DYN_END as their dynamic VA range.
; Each task has its own page table, so the same VA in two tasks maps to
; independent physical pages with no conflict. The per-task region table
; (KD_REGION_TABLE in kernel data) tracks which gaps are used by each task.
;
; The M11 design used a per-task slice (USER_DYN_STRIDE = 3 MiB) which
; capped MAX_TASKS at 8 because 8 * 3 MiB saturated the 32 MiB VA space.
; M12 removes that artificial cap — MAX_TASKS is now bounded only by the
; kernel data structure sizes and the user-PT region (USER_PT_BASE).
;
; USER_DYN_PAGES is now a per-allocation cap (max pages per AllocMem call),
; not a per-task budget — a single task can hold many regions summing to
; far more pages than USER_DYN_PAGES.
USER_DYN_BASE     equ 0xA00000    ; dynamic allocation VA base (shared) — M12.6 Phase D: was 0x800000
USER_DYN_END      equ 0x1200000   ; dynamic allocation VA end (exclusive, shared) — M12.6 Phase E security fix: was 0x2000000. The user-dyn window is now [0xA00000..0x1200000) = 8 MiB, disjoint from the allocator pool VPN range [0x1200..0x1FFF].
USER_DYN_PAGES    equ 768         ; max pages per single AllocMem call

; ---------------------------------------------------------------------------
; Page Bitmap — 800 bytes at kernel data offset 1664
; ---------------------------------------------------------------------------
; Bit N = 1 means page (ALLOC_POOL_BASE + N) is in use.

KD_PAGE_BITMAP    equ 6720        ; page allocation bitmap (M12.6 Phase D: was 6080)
KD_PAGE_BITMAP_SZ equ 800         ; 6400 bits = 800 bytes

; ---------------------------------------------------------------------------
; Region Table — 8 tasks x 8 regions x 16 bytes = 1024 bytes
; ---------------------------------------------------------------------------
; Address: KERN_DATA_BASE + KD_REGION_TABLE + (task_id * KD_REGION_TASK_SZ) + (i * KD_REGION_STRIDE)

; M12.5: KD_REGION_MAX (the 8-row hard cap) is removed by adding a per-task
; OVERFLOW chain. The first 8 rows still live inline at the original
; KD_REGION_TABLE location (preserving the existing fast path and layout
; offsets). Rows 9+ live in allocator-backed chain pages reached through a
; per-task overflow header in a new kernel data field. Walkers iterate
; inline rows first, then walk the overflow chain. The "find free row"
; allocators try inline first, then look for a free row in the overflow
; chain, and finally allocate a new chain page if every existing row is
; occupied. This is a true cap removal: the only failure mode is real
; ERR_NOMEM from the page allocator, not a fixed-row-count limit.
KD_REGION_TABLE       equ 7520    ; inline region table base (32 tasks × 128 bytes — M12.6 Phase D)
KD_REGION_STRIDE      equ 16      ; bytes per region row
KD_REGION_INLINE_MAX  equ 8       ; rows stored inline per task (unchanged from M12)
KD_REGION_TASK_SZ     equ 128     ; bytes per task slot inline (8 × 16)
; Legacy alias for source compatibility — walkers no longer read it as a hard
; cap (they iterate inline THEN overflow), but tests still reference it.
KD_REGION_MAX         equ 8

; Overflow chain (M12.5)
; Per-task overflow chain header (8 bytes), array of 16. New kernel data field.
KD_REGION_OVERFLOW_HEAD equ 11888 ; per-task overflow chain header array base (M12.6 Phase D: was 9200)
KD_REGION_OFLOW_STRIDE  equ 8     ; bytes per task overflow header
KD_REGION_OFLOW_FIRST_PPN equ 0   ; first chain page PPN (2 bytes, 0=empty)
KD_REGION_OFLOW_TOTAL   equ 2     ; total rows in overflow across the chain (2 bytes)
KD_REGION_OFLOW_PAGES   equ 4     ; chain page count (2 bytes)
KD_REGION_OFLOW_RESV    equ 6     ; reserved (2 bytes)

; Overflow chain page layout (4 KiB allocator-backed page):
;   [0..1]   = next chain page PPN (0 = end of chain)
;   [2..15]  = reserved
;   [16..]   = 255 region rows × 16 bytes
KD_REGION_PAGE_HDR_SZ equ 16
KD_REGION_PAGE_NEXT   equ 0       ; chain page header: next page PPN (2 bytes)
KD_REGION_ROWS_PER_PG equ 255     ; (4096 - 16) / 16

; Region entry fields (relative to entry base)
KD_REG_VA         equ 0           ; base VA (4 bytes)
KD_REG_PPN        equ 4           ; base PPN (2 bytes)
KD_REG_PAGES      equ 6           ; page count (2 bytes)
KD_REG_TYPE       equ 8           ; 0=free, 1=private, 2=shared
KD_REG_SHMID      equ 9           ; shared object slot index (1 byte)
KD_REG_FLAGS      equ 10          ; MEMF_ flags low byte (1 byte)

; Region types
REGION_FREE       equ 0
REGION_PRIVATE    equ 1
REGION_SHARED     equ 2
REGION_IO         equ 3            ; I/O mapping (M9): unmap on cleanup, don't free pages

; ---------------------------------------------------------------------------
; Shared Object Table — M12.6 Phase B: inline + overflow chain (no cap)
; ---------------------------------------------------------------------------
; Address (inline rows 0..KD_SHMEM_INLINE_MAX-1):
;   KERN_DATA_BASE + KD_SHMEM_TABLE + (slot * KD_SHMEM_STRIDE)
;
; M12.6 Phase B removes the per-system 16-slot cap (was KD_SHMEM_MAX).
; The first KD_SHMEM_INLINE_MAX rows still live inline at the original
; KD_SHMEM_TABLE offset (preserving the existing fast path and the row
; layout). Rows 16+ live in allocator-backed chain pages reached through
; a single global overflow header at KD_SHMEM_OFLOW_HDR. Walkers iterate
; inline first, then walk the overflow chain. The "find free slot"
; allocator calls the inline scan first, then the overflow scan, and
; finally allocates a new chain page if every existing row is occupied.
; Failure mode is real ERR_NOMEM, not a fixed-row-count cap.
;
; The shmem slot ID is 1 byte (KD_REG_SHMID is 1 byte; the public share
; handle low byte is the same value), so the M12.6 ABI ceiling for total
; shmem slots is 255 (0xFF reserved as sentinel). 255 - INLINE_MAX = 239
; overflow slots, well within one chain page.

KD_SHMEM_TABLE        equ 11616   ; inline shmem table base (M12.6 Phase D: was 8928)
KD_SHMEM_STRIDE       equ 16      ; bytes per shared object entry
KD_SHMEM_INLINE_MAX   equ 16      ; rows stored inline (M12.6: was the cap KD_SHMEM_MAX)
; Legacy alias for any assembly code that still references the old name.
; Walkers no longer treat it as a hard cap.
KD_SHMEM_MAX          equ 16

; M12.6 Phase B: shmem overflow chain — single global header (not per-task).
KD_SHMEM_OFLOW_HDR        equ 12144 ; chain header (8 bytes) — M12.6 Phase D: was 9328
KD_SHMEM_OFLOW_FIRST_PPN  equ 0     ; field offset within header (2 bytes)
KD_SHMEM_OFLOW_TOTAL      equ 2     ; total overflow rows allocated so far (2 bytes)
KD_SHMEM_OFLOW_PAGES      equ 4     ; chain page count (2 bytes)
KD_SHMEM_OFLOW_RESV       equ 6     ; reserved (2 bytes)

; Shmem chain page layout (4 KiB allocator-backed page, identical format
; to the M12.5 region overflow chain):
;   [0..1]   = next chain page PPN (0 = end of chain)
;   [2..15]  = reserved
;   [16..]   = 255 shmem rows × 16 bytes
KD_SHMEM_PAGE_HDR_SZ  equ 16
KD_SHMEM_PAGE_NEXT    equ 0       ; chain page header: next page PPN (2 bytes)
KD_SHMEM_ROWS_PER_PG  equ 255     ; (4096 - 16) / 16

; Shared object entry fields (relative to entry base)
KD_SHM_VALID      equ 0           ; 1=active, 0=unused (1 byte)
KD_SHM_REFCOUNT   equ 1           ; active mappings (1 byte)
KD_SHM_CREATOR    equ 2           ; creating task ID (1 byte)
KD_SHM_PPN        equ 4           ; base PPN (2 bytes)
KD_SHM_PAGES      equ 6           ; page count (2 bytes)
KD_SHM_NONCE      equ 8           ; 24-bit nonce for handle validation (4 bytes)

; ===========================================================================
; Scheduler / Debug
; ===========================================================================

IEXEC_HEARTBEAT_INTERVAL equ 64   ; Print '.' every N timer ticks

; ===========================================================================
; IE64 Program Image Format (M8)
; ===========================================================================
; 32-byte fixed header, then code bytes, then data bytes.
; Entry point is always code offset 0. No relocations.

IMG_MAGIC_LO      equ 0x34364549  ; "IE64" as LE uint32 (bytes: 49 45 36 34)
IMG_MAGIC_HI      equ 0x474F5250  ; "PROG" as LE uint32 (bytes: 50 52 4F 47)
IMG_HEADER_SIZE   equ 32
IMG_OFF_MAGIC     equ 0           ; 8 bytes: magic "IE64PROG"
IMG_OFF_CODE_SIZE equ 8           ; 4 bytes: code section size (max 4096)
IMG_OFF_DATA_SIZE equ 12          ; 4 bytes: data section size (max 4096, 0=none)
IMG_OFF_FLAGS     equ 16          ; 4 bytes: reserved flags (0 for M8)

; Program table entry (24 bytes per entry, sentinel has image_ptr=0)
PROGTAB_ENTRY_SIZE equ 24
PROGTAB_OFF_PTR    equ 0          ; 8 bytes: pointer to image in kernel memory
PROGTAB_OFF_SIZE   equ 8          ; 8 bytes: total image size (header + code + data)

; Clock interval for polling CLOCK service (M8, removed in M9)
CLOCK_INTERVAL    equ 32          ; ticks between '.' output

; ===========================================================================
; M9: exec.library Extensions
; ===========================================================================

; Program table boot count
PROGTAB_BOOT_COUNT equ 3          ; First N entries auto-load at boot

; Terminal I/O page (for SYS_MAP_IO)
TERM_IO_PAGE      equ 0xF0        ; Physical page 0xF0 = address 0xF0000

; Argument passing (SYS_EXEC_PROGRAM)
DATA_ARGS_OFFSET  equ 3072        ; Offset in data page for command args
DATA_ARGS_MAX     equ 256         ; Max arg string length (bytes)

; console.handler message types
CON_MSG_CHAR      equ 0           ; Print single char (data0 low byte)
CON_MSG_READLINE  equ 2           ; Request line input (reply_port, share_handle)

; dos.library protocol opcodes
DOS_OPEN          equ 1           ; Open file (data0=mode, share=name)
DOS_READ          equ 2           ; Read file (data0=handle, data1=max, share=buf)
DOS_WRITE         equ 3           ; Write file (data0=handle, data1=len, share=buf)
DOS_CLOSE         equ 4           ; Close file (data0=handle)
DOS_DIR           equ 5           ; List directory (share=result buf)
DOS_RUN           equ 6           ; Run program (share="cmd\0args\0")
                                  ; Shared buffer holds the command name
                                  ; (resolved through the C: assign) followed
                                  ; by a null-terminated argument string. The
                                  ; legacy data0=index form was retired in M10.

; DOS open modes
DOS_MODE_READ     equ 0
DOS_MODE_WRITE    equ 1

; DOS error codes (returned in reply msg type field)
DOS_OK            equ 0
DOS_ERR_NOTFOUND  equ 1
DOS_ERR_BADARG    equ 2
DOS_ERR_FULL      equ 3
DOS_ERR_BADHANDLE equ 4
DOS_ERR_NOMEM     equ 6

; DOS filesystem limits
;
; M12 note: the per-file slot size was bumped from 4 KiB to 16 KiB so both
; the intuition.library service (which now embeds the Topaz 8x16 bitmap
; font for title-bar text rendering, ~9 KiB total) and the About app
; (which embeds the same font, ~6 KiB total) fit comfortably in a single
; slot. The arbitrary cap remains a known wart — a future milestone
; (M12.8) replaces the fixed-stride slot table with a packed-heap or
; slab/extent allocator inside dos.library. M12.6 Phase A keeps the
; fixed per-file slot size but removes the per-instance count caps.
;
; M12.6 Phase A: DOS_MAX_FILES and DOS_MAX_HANDLES were removed.
; The dos.library file table and handle table are now linked lists of
; AllocMem'd 4 KiB chain pages — see DOS_META_*/DOS_HND_* constants below
; and the .dos_meta_*/.dos_hnd_* helpers in iexec.s. Per-file storage is
; one AllocMem(DOS_FILE_SIZE) per file, no longer packed into a single
; storage_va region.
DOS_NAME_LEN      equ 32          ; Max filename length (supports "S/Startup-Sequence" etc.)
DOS_FILE_SIZE     equ 16384       ; Bytes per file (M12: 4096 → 8192 → 16384). M12.8 will remove this.

; dos.library metadata chain (M12.6 Phase A) — 4 KiB chain pages, 85 entries each.
; Page layout:  [0..7] next_va  [8..15] reserved  [16..4095] entries (85 × 48 bytes)
DOS_META_HDR_SZ   equ 16          ; chain page header bytes
DOS_META_ENTRY_SZ equ 48          ; per-file metadata entry size
DOS_META_PER_PAGE equ 85          ; entries per chain page (16 + 85*48 = 4096)
DOS_META_OFF_NAME equ 0           ; entry: name[32] (NUL-term, 0x00 = unused)
DOS_META_OFF_VA   equ 32          ; entry: file_va (8 bytes — VA of AllocMem'd file body)
DOS_META_OFF_SIZE equ 40          ; entry: size (4 bytes)
DOS_META_OFF_RESV equ 44          ; entry: reserved (4 bytes)

; dos.library handle chain (M12.6 Phase A) — 4 KiB chain pages, 510 entries each.
; Page layout:  [0..7] next_va  [8..15] reserved  [16..4095] entries (510 × 8 bytes)
; Each handle entry stores the VA of the metadata entry it references; 0 = unused.
; handle_id returned to clients = page_index * DOS_HND_PER_PAGE + slot_in_page.
DOS_HND_HDR_SZ    equ 16          ; chain page header bytes
DOS_HND_ENTRY_SZ  equ 8           ; per-handle entry size (metadata entry VA)
DOS_HND_PER_PAGE  equ 510         ; handles per chain page (16 + 510*8 = 4096)

; ===========================================================================
; M11: input.device protocol
; ===========================================================================
; mn_Type opcodes
INPUT_OPEN        equ 0x100        ; Register subscriber port (data0 = port_id)
INPUT_CLOSE       equ 0x101        ; Drop subscriber registration
INPUT_EVENT       equ 0x102        ; Push: data0 = encoded event, data1 = (mx<<48)|(my<<32)|seq32

; Event types (high byte of mn_Data0 in INPUT_EVENT messages)
IE_KEY_DOWN       equ 1
IE_KEY_UP         equ 2            ; Reserved; M11 chip only emits make codes
IE_MOUSE_MOVE     equ 3
IE_MOUSE_BTN      equ 4

; input.device error codes
INPUT_ERR_OK      equ 0
INPUT_ERR_BUSY    equ 1            ; Subscriber already registered
INPUT_ERR_BADARG  equ 2

; ===========================================================================
; M11: graphics.library protocol
; ===========================================================================
; mn_Type opcodes
GFX_ENUMERATE_ADAPTERS equ 0x200    ; → data0 = adapter count
GFX_GET_ADAPTER_INFO   equ 0x201    ; data0 = adapter_id → data0=version<<16|0, data1=caps
GFX_ENUMERATE_MODES    equ 0x202    ; data0 = adapter_id → data0 = mode count
GFX_GET_MODE_INFO      equ 0x203    ; data0=adapter, data1=mode → data0=(w<<16)|h, data1=(fmt<<32)|stride
GFX_OPEN_DISPLAY       equ 0x204    ; data0=adapter, data1=mode → data0 = display_handle
GFX_CLOSE_DISPLAY      equ 0x205    ; data0 = display_handle
GFX_REGISTER_SURFACE   equ 0x206    ; data0=display, data1=(w<<48)|(h<<32)|(fmt<<16)|stride_words, share=surface_share → data0=surface_handle
GFX_UNREGISTER_SURFACE equ 0x207    ; data0 = surface_handle
GFX_PRESENT            equ 0x208    ; data0 = surface_handle, data1 = dirty rect (0=full) → data0 = present_seq

; Pixel formats
GFX_FMT_RGBA32    equ 1

; Adapter capability flags
GFX_CAP_RGBA32    equ 0x0001

; graphics.library error codes
GFX_ERR_OK        equ 0
GFX_ERR_BUSY      equ 1            ; Display already owned, or surface table full
GFX_ERR_BAD_MODE  equ 2
GFX_ERR_BAD_HANDLE equ 3
GFX_ERR_BAD_FORMAT equ 4

; Reserved for M12 — defined now so the opcode space doesn't get reused
; GFX_WAIT_VBLANK    equ 0x209
; GFX_PRESENT_ASYNC  equ 0x20A

; ===========================================================================
; M12: intuition.library protocol
; ===========================================================================
; intuition.library is a user-space windowing service. It is the SOLE
; graphics.library client (registers exactly one screen surface), subscribes
; to input.device for raw events, and delivers IDCMP-style events to per-window
; ports. M12 is a single-window milestone.
;
; mn_Type opcodes
INTUITION_OPEN_WINDOW   equ 0x300    ; data0 = (w<<48)|(h<<32)|(x<<16)|y, data1 = idcmp_port_id, share = win_buf_share → data0 = window_handle
INTUITION_CLOSE_WINDOW  equ 0x301    ; data0 = window_handle
INTUITION_DAMAGE        equ 0x302    ; data0 = window_handle, data1 = (x<<48)|(y<<32)|(w<<16)|h (0 = full window)

; IDCMP message classes (delivered to the window's idcmp_port)
; mn_Type = IDCMP class. mn_Data0 is class-specific:
;   IDCMP_MOUSEMOVE  : data0 = (mx<<32)|my (window-local), data1 = seq32
;   IDCMP_MOUSEBUTTONS: data0 = (buttons<<32)|state, data1 = (mx<<32)|my (window-local)
;   IDCMP_RAWKEY     : data0 = (scancode<<8)|modifiers, data1 = seq32
;   IDCMP_CLOSEWINDOW: data0 = window_handle
;   IDCMP_ACTIVEWINDOW / IDCMP_INACTIVEWINDOW: data0 = window_handle
IDCMP_MOUSEMOVE       equ 0x310
IDCMP_MOUSEBUTTONS    equ 0x311
IDCMP_RAWKEY          equ 0x312
IDCMP_CLOSEWINDOW     equ 0x313
IDCMP_ACTIVEWINDOW    equ 0x314
IDCMP_INACTIVEWINDOW  equ 0x315

; intuition.library error codes
INTUI_ERR_OK          equ 0
INTUI_ERR_BUSY        equ 1            ; Window already open (M12 single-window)
INTUI_ERR_BADARG      equ 2
INTUI_ERR_BADHANDLE   equ 3
INTUI_ERR_NOMEM       equ 4

; Window geometry constants for M12
INTUI_WIN_TITLE_H     equ 16          ; title bar height in pixels
INTUI_WIN_BORDER      equ 2           ; border thickness
INTUI_CLOSE_GADGET_W  equ 16          ; close gadget is the leftmost 16x16 of the title bar

; ===========================================================================
; M12.5: hardware.resource + minimal trust model + grant table
; ===========================================================================
; SYS_MAP_IO is now gated by a kernel-side grant table. Only one task at a
; time may claim the broker identity (hw_resource_task_id) and only that
; broker may create grants. The legacy allowlist-only path is replaced by
; a chain-walking authorization check; the bootstrap-grant table inserted
; during the boot-load loop covers console.handler's serial-port MMIO so
; the boot path can come up before hardware.resource is alive.
;
; New kernel data fields (added at the end of the kernel data area, after
; KD_SHMEM_TABLE which ends at 9184). These fields are kernel-private and
; never exposed to user space.
KD_HWRES_TASK         equ 11872      ; broker task ID (1 byte, 0xFF = unclaimed) — M12.6 Phase D: was 9184
                                     ; Set exactly once by SYS_HWRES_OP/HWRES_BECOME.
                                     ; Subsequent BECOME calls return ERR_EXISTS.
KD_GRANT_TABLE_HDR    equ 11880      ; chain header for the grant table (8 bytes) — M12.6 Phase D: was 9192
                                     ; Field offsets relative to KD_GRANT_TABLE_HDR:
KD_GRANT_HDR_FIRST_PPN equ 0         ;   first chain page PPN (2 bytes, 0=unallocated)
KD_GRANT_HDR_TOTAL    equ 2          ;   total grant rows in use across the chain (2 bytes)
KD_GRANT_HDR_PAGES    equ 4          ;   number of chain pages currently linked (2 bytes)
KD_GRANT_HDR_RESV     equ 6          ;   reserved (2 bytes)
KD_GRANT_HDR_SIZE     equ 8

; Each chain page is a 4 KiB allocator-backed page laid out as:
;   [0..1]   = next chain page PPN (0 = end of chain)
;   [2..15]  = reserved
;   [16..]   = grant rows (each KD_GRANT_ROW_SIZE bytes)
; Existing chain pages are NEVER copied or moved on grow — appending a new
; page only links its PPN into the previous tail's [0..1] field, so any
; kernel-internal pointer to a row stays valid for the row's lifetime.
KD_GRANT_PAGE_NEXT    equ 0          ; next chain page PPN (2 bytes)
KD_GRANT_PAGE_HDR_SZ  equ 16         ; bytes reserved at the start of each chain page
KD_GRANT_ROW_SIZE     equ 16         ; bytes per grant row
KD_GRANT_ROWS_PER_PG  equ 255        ; (4096 - 16) / 16 = 255

; Grant row field offsets (relative to row base):
KD_GRANT_TASK_ID      equ 0          ; granted task ID (1 byte, 0xFF = unused)
KD_GRANT_RESV1        equ 1          ; reserved (3 bytes)
KD_GRANT_REGION       equ 4          ; 4-byte region tag (e.g. 'CHIP', 'VRAM')
KD_GRANT_PPN_LO       equ 8          ; first PPN of granted range (2 bytes)
KD_GRANT_PPN_HI       equ 10         ; last PPN of granted range, inclusive (2 bytes)
KD_GRANT_FLAGS        equ 12         ; reserved flags (4 bytes)

; Grant row sentinel
GRANT_TASK_FREE       equ 0xFF       ; row is unused

; SYS_HWRES_OP verbs (R6 selector — R0 is the IE64 zero register)
HWRES_BECOME          equ 0          ; claim broker identity
HWRES_CREATE          equ 1          ; create a grant row (broker-only)
HWRES_REVOKE          equ 2          ; reserved for M13; returns ERR_BADARG in M12.5
HWRES_TASK_ALIVE      equ 3          ; query task liveness (broker-only)
                                     ; R1 = task_id; returns R1 = 1 if task slot
                                     ; is in use (READY/RUNNING/WAITING), 0 if FREE.
                                     ; Used by the broker to reclaim stale owner
                                     ; slots when a grantee has exited.

; Region tag literals (4-byte ASCII tags as little-endian uint32 constants).
; The kernel doesn't interpret these — only hardware.resource looks them up
; in its policy table. They appear in the kernel only as opaque bytes copied
; into KD_GRANT_REGION.
HWRES_TAG_CHIP        equ 0x50494843  ; 'CHIP' little-endian: 'C'=43 'H'=48 'I'=49 'P'=50
HWRES_TAG_VRAM        equ 0x4D415256  ; 'VRAM' little-endian: 'V'=56 'R'=52 'A'=41 'M'=4D

; hardware.resource public message protocol (delivered to its public port)
HWRES_MSG_REQUEST     equ 0x400      ; data0 = 4-byte region tag (in low bytes), reply expected
HWRES_MSG_GRANTED     equ 0x401      ; reply: data0 = (ppn_base<<32)|page_count
HWRES_MSG_DENIED      equ 0x402      ; reply: data0 = error code

; Bootstrap grant table — populated at boot-load time, NOT at kern_init time
; (task IDs do not exist at kern_init). Each row is keyed by program_table
; index, not task ID; the boot-load loop resolves to a real task ID after
; load_program returns. The bootstrap table is part of the kernel image, not
; the kernel data page.
BOOTSTRAP_GRANT_ROW_SZ equ 16        ; bytes per bootstrap row (laid out in iexec.s)
BOOTSTRAP_GRANT_OFF_PROG_IDX equ 0   ; 1 byte: program_table boot index
BOOTSTRAP_GRANT_OFF_TAG      equ 4   ; 4 bytes: region tag
BOOTSTRAP_GRANT_OFF_PPN_LO   equ 8   ; 2 bytes: PPN low
BOOTSTRAP_GRANT_OFF_PPN_HI   equ 10  ; 2 bytes: PPN high (inclusive)
