seed_startup:
    incbin  "assets/system/S/Startup-Sequence"
    dc.b    0
seed_startup_end:
    align   8

seed_help_text:
    incbin  "assets/system/S/Help"
    dc.b    0
seed_help_text_end:
    align   8

seed_loader_info:
    incbin  "assets/system/L/Loader-Info"
    dc.b    0
seed_loader_info_end:
