#include "ie_machine.h"

/*
 * ST-compatible make/break scancode MMIO helpers.
 *
 * Current IE runtime drains IE_SCAN_CODE into EmuTOS ikbdiorec from the host
 * side on the 5 ms timer cadence. Target code must not poll IE_SCAN_CODE from
 * kb_timerc_int(); reading IE_SCAN_CODE dequeues the host event and races the
 * runtime IOREC pump. Keep these helpers only for diagnostics or legacy ports
 * that do not use the runtime IOREC pump.
 */
unsigned short ie_kbd_has_code(void) { return IE_MMIO16(IE_SCAN_STATUS); }
unsigned short ie_kbd_code(void) { return IE_MMIO16(IE_SCAN_CODE); }
unsigned short ie_kbd_mods(void) { return IE_MMIO16(IE_SCAN_MODIFIERS); }
