; iexec.inc - IExec Microkernel Constants
;
; For use with ie64asm assembler
; Include with: include "iexec.inc"

; ===========================================================================
; Syscall Numbers (IExec contract)
; ===========================================================================

SYS_ALLOC_MEM     equ 1
SYS_FREE_MEM      equ 2
SYS_ALLOC_SHARED  equ 3
SYS_MAP_SHARED    equ 4
SYS_CREATE_TASK   equ 5
SYS_DELETE_TASK   equ 6
SYS_FIND_TASK     equ 7
SYS_SET_TASK_PRI  equ 8
SYS_SET_TP        equ 9
SYS_GET_TASK_INFO equ 10
SYS_ALLOC_SIGNAL  equ 11
SYS_FREE_SIGNAL   equ 12
SYS_SIGNAL        equ 13
SYS_WAIT          equ 14
SYS_CREATE_PORT   equ 15
SYS_FIND_PORT     equ 16
SYS_PUT_MSG       equ 17
SYS_GET_MSG       equ 18
SYS_WAIT_PORT     equ 19
SYS_REPLY_MSG     equ 20
SYS_PEEK_PORT     equ 21
SYS_ADD_TIMER     equ 22
SYS_REM_TIMER     equ 23
SYS_CLOSE_HANDLE  equ 24
SYS_DUP_HANDLE    equ 25
SYS_YIELD         equ 26
SYS_GET_SYS_INFO  equ 27
SYS_MAP_IO        equ 28
SYS_MAP_VRAM      equ 29
SYS_DEBUG         equ 30
SYS_SEND_MSG_BULK equ 31
SYS_RECV_MSG_BULK equ 32
SYS_DEBUG_PUTCHAR equ 33          ; Debug: print character (R1=char)
SYS_EXIT_TASK     equ 34          ; Exit current task (R1=exit_code, ignored)
SYS_EXEC_PROGRAM  equ 35          ; Launch program (R1=image_ptr, R2=image_size, R3=args_ptr, R4=args_len) [legacy: R1<0x600000=index]
SYS_OPEN_LIBRARY  equ 36          ; OpenLibrary (R1=name_ptr, R2=version) → R1=handle, R2=err
SYS_READ_INPUT    equ 37          ; Read terminal line (R1=buf_ptr, R2=max_len) → R1=bytes_read, R2=err

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

KERN_PAGE_TABLE   equ 0x010000    ; Kernel page table (64 KiB)
KERN_DATA_BASE    equ 0x020000    ; Kernel data structures
KERN_STACK_TOP    equ 0x09F000    ; Kernel stack top

; ---------------------------------------------------------------------------
; Kernel Data Offsets (at KERN_DATA_BASE)
; ---------------------------------------------------------------------------
; Layout (M7, 8 tasks):
;   +0:    current_task (8)
;   +8:    tick_count (8)
;   +16:   num_tasks (8)
;   +24:   nonce_counter (8)
;   +64:   TCB[0..7] (8 x 32 = 256 bytes)              → ends at 320
;   +320:  PTBR[0..7] (8 x 8 = 64 bytes)               → ends at 384
;   +384:  Port[0..7] (8 x 160 = 1280 bytes)            → ends at 1664
;   +1664: Page bitmap (800 bytes)                       → ends at 2464
;   +2464: Region table (8 tasks x 8 x 16 = 1024 bytes) → ends at 3488
;   +3488: Shared object table (8 x 16 = 128 bytes)     → ends at 3616

KD_CURRENT_TASK   equ 0           ; uint64: current task index
KD_TICK_COUNT     equ 8           ; uint64: tick counter
KD_NUM_TASKS      equ 16          ; uint64: number of active tasks
KD_NONCE_COUNTER  equ 24          ; uint64: monotonic counter for share handle nonces
KD_NAME_SCRATCH   equ 32          ; 16-byte scratch buffer for port name operations

; ---------------------------------------------------------------------------
; Task Control Block (TCB) — 32 bytes per task
; ---------------------------------------------------------------------------
; Address: KERN_DATA_BASE + KD_TASK_BASE + (task_id * KD_TASK_STRIDE)

MAX_TASKS         equ 8           ; maximum concurrent tasks

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

KD_PTBR_BASE      equ 320         ; offset of PTBR array (after 8 TCBs)

; ===========================================================================
; Message Port Layout (M7)
; ===========================================================================
; 8 ports max, 160 bytes each, starting at offset 384 in kernel data
; Each port: 32-byte header (with name) + 4 × 32-byte messages (FIFO)

KD_PORT_BASE      equ 384         ; start of port array (after PTBR array)
KD_PORT_STRIDE    equ 160         ; bytes per port
KD_PORT_MAX       equ 8           ; max number of ports
KD_PORT_FIFO_SIZE equ 4           ; messages per port FIFO

; Port header field offsets (relative to port base, 32-byte header)
KD_PORT_VALID     equ 0           ; 1=active, 0=unused (1 byte)
KD_PORT_OWNER     equ 1           ; owning task ID (1 byte)
KD_PORT_COUNT     equ 2           ; messages in FIFO, 0-4 (1 byte)
KD_PORT_HEAD      equ 3           ; read index, 0-3 (1 byte)
KD_PORT_TAIL      equ 4           ; write index, 0-3 (1 byte)
KD_PORT_FLAGS     equ 5           ; port flags (PF_PUBLIC etc.) (1 byte)
KD_PORT_NAME      equ 8           ; port name, 16 bytes, zero-padded ASCII
KD_PORT_MSGS      equ 32          ; start of message array (4 × 32 bytes)

; Port flags
PF_PUBLIC         equ 1           ; port is publicly discoverable via FindPort
PORT_NAME_LEN     equ 16          ; max port name length (bytes)

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
; between the user code/stack/data slot block ($600000-$67FFFF) and the page
; allocator pool ($700000+). MAX_TASKS * USER_SLOT_STRIDE = 8 * 0x10000 =
; 0x80000, so the region exactly fills $680000-$6FFFFF.
USER_PT_BASE      equ 0x680000    ; Task page tables (each 64 KiB)
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
USER_PT_PAGE_BASE   equ 0x680    ; USER_PT_BASE >> MMU_PAGE_SHIFT
USER_PT_PAGE_END    equ 0x700    ; (USER_PT_BASE + MAX_TASKS * USER_SLOT_STRIDE) >> MMU_PAGE_SHIFT

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
ALLOC_POOL_BASE   equ 0x700       ; first allocable page number
ALLOC_POOL_PAGES  equ 6400        ; pages 0x700-0x1FFF

; Dynamic VA layout (per-task windows for AllocMem)
USER_DYN_BASE     equ 0x800000    ; dynamic allocation VA base
USER_DYN_STRIDE   equ 0x100000    ; 1 MB per task window (256 pages)
USER_DYN_PAGES    equ 256         ; max pages per task dynamic window

; ---------------------------------------------------------------------------
; Page Bitmap — 800 bytes at kernel data offset 1664
; ---------------------------------------------------------------------------
; Bit N = 1 means page (ALLOC_POOL_BASE + N) is in use.

KD_PAGE_BITMAP    equ 1664        ; page allocation bitmap
KD_PAGE_BITMAP_SZ equ 800         ; 6400 bits = 800 bytes

; ---------------------------------------------------------------------------
; Region Table — 8 tasks x 8 regions x 16 bytes = 1024 bytes
; ---------------------------------------------------------------------------
; Address: KERN_DATA_BASE + KD_REGION_TABLE + (task_id * KD_REGION_TASK_SZ) + (i * KD_REGION_STRIDE)

KD_REGION_TABLE   equ 2464        ; region table base
KD_REGION_STRIDE  equ 16          ; bytes per region entry
KD_REGION_MAX     equ 8           ; max regions per task
KD_REGION_TASK_SZ equ 128         ; 8 regions x 16 bytes per task

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
; Shared Object Table — 8 entries x 16 bytes = 128 bytes
; ---------------------------------------------------------------------------
; Address: KERN_DATA_BASE + KD_SHMEM_TABLE + (slot * KD_SHMEM_STRIDE)

KD_SHMEM_TABLE    equ 3488        ; shared object table base
KD_SHMEM_STRIDE   equ 16          ; bytes per shared object entry
KD_SHMEM_MAX      equ 8           ; max shared objects system-wide

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
DOS_RUN           equ 6           ; Run program (data0=index, share=args)

; DOS open modes
DOS_MODE_READ     equ 0
DOS_MODE_WRITE    equ 1

; DOS error codes (returned in reply msg type field)
DOS_OK            equ 0
DOS_ERR_NOTFOUND  equ 1
DOS_ERR_FULL      equ 3
DOS_ERR_BADHANDLE equ 4
DOS_ERR_NOMEM     equ 6

; DOS filesystem limits
DOS_MAX_FILES     equ 16          ; Max files in RAM:
DOS_NAME_LEN      equ 32          ; Max filename length (supports "S/Startup-Sequence" etc.)
DOS_FILE_SIZE     equ 4096        ; Max bytes per file (1 page)
DOS_MAX_HANDLES   equ 8           ; Max simultaneous open handles
