#include "ie_machine.h"

/* Mouse polling helpers for AES integration. */
unsigned short ie_mouse_x(void) { return IE_MMIO16(IE_MOUSE_X); }
unsigned short ie_mouse_y(void) { return IE_MMIO16(IE_MOUSE_Y); }
unsigned short ie_mouse_buttons(void) { return IE_MMIO16(IE_MOUSE_BUTTONS); }
unsigned short ie_mouse_changed(void) { return IE_MMIO16(IE_MOUSE_STATUS); }
