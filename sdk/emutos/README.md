# EmuTOS IE Machine Target

This folder contains Intuition Engine machine-target skeleton files for wiring EmuTOS to IE MMIO.

Files:
- `ie_machine.h`: MMIO addresses and helper macros
- `ie_screen.c`: 640x480 RGBA32 framebuffer helpers
- `ie_sound.c`: YM2149/PSG register writes at `0xF0C00`
- `ie_mouse.c`: mouse poll helpers (`0xF0730` block)
- `ie_kbd.c`: scancode/modifier poll helpers (`0xF0740` block)
- `ie_timer.c`: level 5 (200Hz) and level 4 (VBlank) hook skeleton

These are intentionally thin target-side shims to be integrated into an EmuTOS source tree.
