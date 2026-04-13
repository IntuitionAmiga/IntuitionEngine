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

panic_msg:
    dc.b    "KERNEL PANIC: ", 0
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
