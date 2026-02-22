#include "ie_machine.h"

/* PSG write path for IE MMIO. */
void ie_psg_write(unsigned int reg, unsigned int value) {
    IE_MMIO8(IE_PSG_BASE + (reg & 0x0FUL)) = (unsigned char)value;
}
