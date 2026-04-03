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

; Per-task state (simplified layout for milestone 1)
; Two tasks stored at: KERN_DATA_BASE + 64 + (task * 16)
; Each task: 8 bytes PC + 8 bytes USP
KD_TASK_BASE      equ 64          ; offset of task state array
KD_TASK_STRIDE    equ 16          ; bytes per task entry
; Pre-computed offsets for task 0 and task 1
KD_TASK0_PC       equ 64          ; KD_TASK_BASE + 0*16 + 0
KD_TASK0_USP      equ 72          ; KD_TASK_BASE + 0*16 + 8
KD_TASK1_PC       equ 80          ; KD_TASK_BASE + 1*16 + 0
KD_TASK1_USP      equ 88          ; KD_TASK_BASE + 1*16 + 8
; PTBR per task at: KERN_DATA_BASE + 96 + (task * 8)
KD_PTBR_BASE      equ 96          ; offset of PTBR array
KD_PTBR0          equ 96          ; KD_PTBR_BASE + 0
KD_PTBR1          equ 104         ; KD_PTBR_BASE + 8

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
USER_CODE_SIZE    equ 40          ; 5 instructions x 8 bytes

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
