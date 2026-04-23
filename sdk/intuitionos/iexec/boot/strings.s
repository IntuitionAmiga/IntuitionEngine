; ============================================================================

boot_banner:
    dc.b    "exec.library M11 boot", 0x0D, 0x0A, 0
    align   4

fault_msg_prefix:
    dc.b    "GURU MEDITATION cause=", 0
    align   4

fault_msg_pc:
    dc.b    " PC=", 0
    align   4

fault_msg_addr:
    dc.b    " ADDR=", 0
    align   4

deadlock_msg:
    dc.b    "DEADLOCK: no runnable tasks", 0x0D, 0x0A, 0
    align   4

fault_msg_task:
    dc.b    " task=", 0
    align   4

fault_msg_access:
    dc.b    " ACCESS=", 0
    align   4

fault_msg_mode:
    dc.b    " MODE=", 0
    align   4

fault_msg_class:
    dc.b    " CLASS=", 0
    align   4

fault_msg_pte:
    dc.b    " PTE=", 0
    align   4

fault_mode_user:
    dc.b    "user", 0
    align   4

fault_mode_supervisor:
    dc.b    "supervisor", 0
    align   4

fault_access_unknown:
    dc.b    "unknown", 0
    align   4

fault_access_read:
    dc.b    "read", 0
    align   4

fault_access_write:
    dc.b    "write", 0
    align   4

fault_access_exec:
    dc.b    "execute", 0
    align   4

fault_class_unknown:
    dc.b    "unknown", 0
    align   4

fault_class_not_present:
    dc.b    "not-present", 0
    align   4

fault_class_read:
    dc.b    "read-denied", 0
    align   4

fault_class_write:
    dc.b    "write-denied", 0
    align   4

fault_class_exec:
    dc.b    "exec-denied", 0
    align   4

fault_class_user_super:
    dc.b    "user-supervisor", 0
    align   4

fault_class_priv:
    dc.b    "privileged", 0
    align   4

fault_class_misaligned:
    dc.b    "misaligned", 0
    align   4

fault_class_timer:
    dc.b    "timer", 0
    align   4

fault_class_syscall:
    dc.b    "syscall", 0
    align   4

panic_msg:
    dc.b    "KERNEL PANIC: ", 0
    align   4

panic_stack_canary_msg:
    dc.b    "kernel stack canary", 0x0D, 0x0A, 0
    align   4

no_tasks_msg:
    dc.b    "PANIC: no programs loaded", 0x0D, 0x0A, 0
    align   4

boot_fail_msg:
    dc.b    "BOOT FAIL", 0x0D, 0x0A, 0
    align   4

boot_host_relpath_doslib:
    dc.b    "IOSSYS/LIBS/dos.library", 0
    align   4

boot_host_relpath_console:
    dc.b    "IOSSYS/L/console.handler", 0
    align   4

boot_host_relpath_shell:
    dc.b    "IOSSYS/Tools/Shell", 0
    align   4

boot_shell_path_full:
    dc.b    "IOSSYS:Tools/Shell", 0
    align   4

boot_shell_path_resolved:
    dc.b    "SYS/IOSSYS/Tools/Shell", 0
    align   4
