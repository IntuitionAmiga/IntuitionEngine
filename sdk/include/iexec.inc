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

; Kernel data offsets
KD_CURRENT_TASK   equ 0           ; uint64: current task index (0 or 1)
KD_TICK_COUNT     equ 8           ; uint64: tick counter
KD_NUM_TASKS      equ 16          ; uint64: number of tasks

; Per-task state (M3: signals)
; Two tasks stored at: KERN_DATA_BASE + 64 + (task * 32)
; Each task: 32 bytes { PC(8), USP(8), sig_alloc(4), sig_wait(4), sig_recv(4), state(1), pad(3) }
KD_TASK_BASE      equ 64          ; offset of task state array
KD_TASK_STRIDE    equ 32          ; bytes per task entry
; Per-task field offsets (relative to task base)
KD_TASK_PC        equ 0           ; saved PC (8 bytes)
KD_TASK_USP       equ 8           ; saved user SP (8 bytes)
KD_TASK_SIG_ALLOC equ 16          ; allocated signal bits (4 bytes)
KD_TASK_SIG_WAIT  equ 20          ; signals being waited for (4 bytes, 0 if not waiting)
KD_TASK_SIG_RECV  equ 24          ; pending received signals (4 bytes)
KD_TASK_STATE     equ 28          ; task state byte: 0=READY, 1=RUNNING, 2=WAITING
KD_TASK_WAITPORT  equ 29          ; WaitPort port ID (0xFF = not in WaitPort)
; Pre-computed offsets for task 0 and task 1
KD_TASK0_PC       equ 64          ; KD_TASK_BASE + 0*32 + 0
KD_TASK0_USP      equ 72          ; KD_TASK_BASE + 0*32 + 8
KD_TASK1_PC       equ 96          ; KD_TASK_BASE + 1*32 + 0
KD_TASK1_USP      equ 104         ; KD_TASK_BASE + 1*32 + 8
; PTBR per task at: KERN_DATA_BASE + 128 + (task * 8)
KD_PTBR_BASE      equ 128         ; offset of PTBR array
KD_PTBR0          equ 128         ; KD_PTBR_BASE + 0
KD_PTBR1          equ 136         ; KD_PTBR_BASE + 8
; Task state constants
TASK_READY        equ 0
TASK_RUNNING      equ 1
TASK_WAITING      equ 2
; Signal bit constants
SIGF_PORT         equ 1           ; Bit 0: message arrived at a port
; WaitPort sentinel
WAITPORT_NONE     equ 0xFF        ; not blocked in WaitPort

; ===========================================================================
; Message Port Layout (M4)
; ===========================================================================
; 4 ports max, 72 bytes each, starting at offset 144 in kernel data
; Each port: 8-byte header + 4 × 16-byte messages (FIFO)

KD_PORT_BASE      equ 144         ; start of port array (after PTBR array)
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

; System signal allocation mask (bits 0-15 pre-allocated)
SIG_SYSTEM_MASK   equ 0xFFFF

; ===========================================================================
; User Space Layout
; ===========================================================================

USER_PT0_BASE     equ 0x030000    ; Task 0 page table (64 KiB)
USER_PT1_BASE     equ 0x040000    ; Task 1 page table (64 KiB)
USER_TASK0_CODE   equ 0x600000    ; Task 0 code page (physical)
USER_TASK0_STACK  equ 0x601000    ; Task 0 stack page
USER_TASK0_DATA   equ 0x602000    ; Task 0 data page
USER_TASK1_CODE   equ 0x610000    ; Task 1 code page (physical)
USER_TASK1_STACK  equ 0x611000    ; Task 1 stack page
USER_TASK1_DATA   equ 0x612000    ; Task 1 data page
USER_CODE_SIZE    equ 96          ; 12 instructions x 8 bytes (port-based request/reply)

; VPNs for user pages (addr >> 12)
USER_TASK0_CODE_VPN  equ 0x600   ; 0x600000 >> 12
USER_TASK0_STACK_VPN equ 0x601
USER_TASK0_DATA_VPN  equ 0x602
USER_TASK1_CODE_VPN  equ 0x610   ; 0x610000 >> 12
USER_TASK1_STACK_VPN equ 0x611
USER_TASK1_DATA_VPN  equ 0x612

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
