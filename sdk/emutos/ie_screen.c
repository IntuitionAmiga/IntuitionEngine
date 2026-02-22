#include "emutos.h"
#include "ie_machine.h"

static inline volatile ULONG *ie_vram_ptr(ULONG byte_off)
{
    return (volatile ULONG *)(IE_VRAM_BASE + byte_off);
}

void ie_screen_init(void) {
    UWORD i;
    IE_MMIO32(IE_VIDEO_MODE) = 0; /* native 640x480 RGBA mode */
    IE_MMIO32(IE_VIDEO_CTRL) = 1; /* enable video */
    ie_clear(0, 0, 0);
    /* Boot marker: visible without any VDI/VT52 path involvement. */
    for (i = 0; i < 64; i++) {
        ie_putpixel(i, i, ie_rgba(255, 255, 0, 255));
    }
}

void ie_putpixel(unsigned short x, unsigned short y, unsigned long rgba) {
    ULONG px;
    ULONG row_off;
    ULONG byte_off;

    if (x >= IE_VRAM_WIDTH || y >= IE_VRAM_HEIGHT) {
        return;
    }
    px = (ULONG)x;
    row_off = (ULONG)y * IE_VRAM_STRIDE;
    byte_off = row_off + (px << 2);
    *ie_vram_ptr(byte_off) = (ULONG)rgba;
}

void ie_clear(unsigned char r, unsigned char g, unsigned char b) {
    ULONG c;
    ULONG y;
    ULONG x;

    c = ie_rgba(r, g, b, 0xFF);
    for (y = 0; y < (ULONG)IE_VRAM_HEIGHT; y++) {
        for (x = 0; x < (ULONG)IE_VRAM_WIDTH; x++) {
            ie_putpixel((UWORD)x, (UWORD)y, c);
        }
    }
}

void ie_fill_rect(unsigned short x, unsigned short y, unsigned short w, unsigned short h, unsigned long rgba) {
    ULONG yy;
    ULONG xx;
    ULONG x0;
    ULONG y0;
    ULONG w0;
    ULONG h0;

    if (x >= IE_VRAM_WIDTH || y >= IE_VRAM_HEIGHT || w == 0 || h == 0) {
        return;
    }
    x0 = (ULONG)x;
    y0 = (ULONG)y;
    w0 = (ULONG)w;
    h0 = (ULONG)h;
    if ((x0 + w0) > (ULONG)IE_VRAM_WIDTH) {
        w0 = (ULONG)IE_VRAM_WIDTH - x0;
    }
    if ((y0 + h0) > (ULONG)IE_VRAM_HEIGHT) {
        h0 = (ULONG)IE_VRAM_HEIGHT - y0;
    }
    for (yy = 0; yy < h0; yy++) {
        for (xx = 0; xx < w0; xx++) {
            ie_putpixel((UWORD)(x0 + xx), (UWORD)(y0 + yy), rgba);
        }
    }
}

void ie_scroll_up_pixels(unsigned short lines, unsigned long bg_rgba) {
    ULONG lines32;
    ULONG swapped32;

    if (lines == 0) {
        return;
    }
    lines32 = (ULONG)lines;
    /* Defensive recovery: on some paths a 16-bit line count can arrive byte-swapped
     * (e.g. 0x1000 instead of 0x0010). If the swapped value is sane, use it.
     */
    if (lines32 >= (ULONG)IE_VRAM_HEIGHT) {
        swapped32 = (ULONG)((UWORD)((lines >> 8) | (lines << 8)));
        if (swapped32 > 0 && swapped32 < (ULONG)IE_VRAM_HEIGHT) {
            lines32 = swapped32;
        } else {
            lines32 = 16UL;
        }
    }
    if (lines32 >= (ULONG)IE_VRAM_HEIGHT) {
        lines32 = (ULONG)IE_VRAM_HEIGHT - 1UL;
    }

    /*
     * IE-safe fallback: avoid read+copy scroll path while bus-fault source
     * is being isolated. Keep console usable by clearing to background.
     */
    ie_fill_rect(0, 0, IE_VRAM_WIDTH, IE_VRAM_HEIGHT, bg_rgba);
}
