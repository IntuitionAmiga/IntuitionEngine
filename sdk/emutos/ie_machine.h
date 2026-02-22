#ifndef IE_MACHINE_H
#define IE_MACHINE_H

/* Intuition Engine MMIO map used by EmuTOS IE target */
#define IE_VIDEO_CTRL      0x000F0000UL
#define IE_VIDEO_MODE      0x000F0004UL
#define IE_VIDEO_STATUS    0x000F0008UL
#define IE_VRAM_BASE       0x00100000UL
#define IE_VRAM_STRIDE     2560UL      /* 640 * 4 */
#define IE_VRAM_WIDTH      640U
#define IE_VRAM_HEIGHT     480U

#define IE_PSG_BASE        0x000F0C00UL

#define IE_MOUSE_X         0x000F0730UL
#define IE_MOUSE_Y         0x000F0734UL
#define IE_MOUSE_BUTTONS   0x000F0738UL
#define IE_MOUSE_STATUS    0x000F073CUL

#define IE_SCAN_CODE       0x000F0740UL
#define IE_SCAN_STATUS     0x000F0744UL
#define IE_SCAN_MODIFIERS  0x000F0748UL

#define IE_TERM_OUT        0x000F0700UL

/* Timer levels delivered by IE runtime */
#define IE_IRQ_TIMER_LEVEL 5U
#define IE_IRQ_VBL_LEVEL   4U

/* Simple MMIO helpers */
#define IE_MMIO8(a)   (*(volatile unsigned char *)(a))
#define IE_MMIO16(a)  (*(volatile unsigned short *)(a))
#define IE_MMIO32(a)  (*(volatile unsigned long *)(a))

void ie_screen_init(void);
void ie_putpixel(unsigned short x, unsigned short y, unsigned long rgba);
void ie_clear(unsigned char r, unsigned char g, unsigned char b);

unsigned short ie_kbd_has_code(void);
unsigned short ie_kbd_code(void);
unsigned short ie_kbd_mods(void);

unsigned short ie_mouse_x(void);
unsigned short ie_mouse_y(void);
unsigned short ie_mouse_buttons(void);
unsigned short ie_mouse_changed(void);

void ie_timer_install(void);
void ie_timer_handle_l5(void);
void ie_timer_handle_l4(void);

void ie_psg_write(unsigned int reg, unsigned int value);

void ie_fill_rect(unsigned short x, unsigned short y, unsigned short w, unsigned short h, unsigned long rgba);
void ie_scroll_up_pixels(unsigned short lines, unsigned long bg_rgba);

/* Pack RGBA bytes into a big-endian ULONG for direct VRAM writes */
static inline unsigned long ie_rgba(unsigned char r, unsigned char g, unsigned char b, unsigned char a) {
    return ((unsigned long)r << 24) | ((unsigned long)g << 16) | ((unsigned long)b << 8) | (unsigned long)a;
}

#endif
