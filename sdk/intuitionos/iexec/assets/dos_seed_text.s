seed_startup:
    dc.b    "RESOURCES/hardware.resource", 0x0A
    dc.b    "DEVS/input.device", 0x0A
    dc.b    "LIBS/graphics.library", 0x0A
    dc.b    "LIBS/intuition.library", 0x0A
    dc.b    "VERSION", 0x0A
    dc.b    "ECHO Type HELP for commands and ASSIGN for layout", 0x0A, 0
seed_startup_end:
    align   8

seed_help_text:
    dc.b    "M15 help surface:", 0x0D, 0x0A
    dc.b    "Commands: VERSION AVAIL DIR TYPE ECHO ASSIGN LIST WHICH HELP", 0x0D, 0x0A
    dc.b    "Assigns: RAM: C: L: LIBS: DEVS: T: S: RESOURCES:", 0x0D, 0x0A, 0
seed_help_text_end:
    align   8

seed_loader_info:
    dc.b    "L: contains DOS helper assets and is not part of bare command search.", 0x0D, 0x0A, 0
seed_loader_info_end:
