#include "ie_machine.h"

/* ST-compatible make/break scancode polling. */
unsigned short ie_kbd_has_code(void) { return IE_MMIO16(IE_SCAN_STATUS); }
unsigned short ie_kbd_code(void) { return IE_MMIO16(IE_SCAN_CODE); }
unsigned short ie_kbd_mods(void) { return IE_MMIO16(IE_SCAN_MODIFIERS); }
