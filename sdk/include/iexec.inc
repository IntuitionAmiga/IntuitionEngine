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
; Layout (M5, 8 tasks):
;   +0:   current_task (8)
;   +8:   tick_count (8)
;   +16:  num_tasks (8)
;   +64:  TCB[0..7] (8 x 32 = 256 bytes)   → ends at 320
;   +320: PTBR[0..7] (8 x 8 = 64 bytes)    → ends at 384
;   +384: Port[0..3] (4 x 72 = 288 bytes)  → ends at 672

KD_CURRENT_TASK   equ 0           ; uint64: current task index
KD_TICK_COUNT     equ 8           ; uint64: tick counter
KD_NUM_TASKS      equ 16          ; uint64: number of active tasks

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
; Message Port Layout (M4)
; ===========================================================================
; 4 ports max, 72 bytes each, starting at offset 384 in kernel data
; Each port: 8-byte header + 4 × 16-byte messages (FIFO)

KD_PORT_BASE      equ 384         ; start of port array (after PTBR array)
KD_PORT_STRIDE    equ 72          ; bytes per port
KD_PORT_MAX       equ 4           ; max number of ports
KD_PORT_FIFO_SIZE equ 4           ; messages per port FIFO

; Port header field offsets (relative to port base)
KD_PORT_VALID     equ 0           ; 1=active, 0=unused (1 byte)
KD_PORT_OWNER     equ 1           ; owning task ID (1 byte)
KD_PORT_COUNT     equ 2           ; messages in FIFO, 0-4 (1 byte)
KD_PORT_HEAD      equ 3           ; read index, 0-3 (1 byte)
KD_PORT_TAIL      equ 4           ; write index, 0-3 (1 byte)
KD_PORT_MSGS      equ 8           ; start of message array (4 × 16 bytes)

; Message field offsets (relative to message base, 16 bytes per message)
KD_MSG_TYPE       equ 0           ; application-defined type (4 bytes)
KD_MSG_SRC        equ 4           ; sender task ID (4 bytes)
KD_MSG_DATA       equ 8           ; payload data (8 bytes)
KD_MSG_SIZE       equ 16          ; bytes per message

; ===========================================================================
; User Space Layout (M5: slot-based, 8 tasks)
; ===========================================================================
; Task i page table: USER_PT_BASE + i * USER_SLOT_STRIDE
; Task i code page:  USER_CODE_BASE + i * USER_SLOT_STRIDE
; Task i stack page: USER_STACK_BASE + i * USER_SLOT_STRIDE
; Task i data page:  USER_DATA_BASE + i * USER_SLOT_STRIDE
; Task i code VPN:   USER_CODE_VPN_BASE + i * USER_VPN_STRIDE

USER_PT_BASE      equ 0x100000    ; Task page tables (each 64 KiB)
USER_CODE_BASE    equ 0x600000    ; Task code pages (physical)
USER_STACK_BASE   equ 0x601000    ; Task stack pages
USER_DATA_BASE    equ 0x602000    ; Task data pages
USER_SLOT_STRIDE  equ 0x10000     ; 64 KiB between task slots
USER_CODE_SIZE    equ 96          ; 12 instructions x 8 bytes (boot template)

; VPN bases (addr >> 12)
USER_CODE_VPN_BASE  equ 0x600    ; 0x600000 >> 12
USER_VPN_STRIDE     equ 0x10     ; 0x10000 >> 12

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
; Scheduler / Debug
; ===========================================================================

IEXEC_HEARTBEAT_INTERVAL equ 64   ; Print '.' every N timer ticks
