/* ============================================================================
 * AROS ROTOZOOMER — C + Hardware Direct (Blitter COPY to screen VRAM)
 * ============================================================================
 * Opens an Intuition CUSTOMSCREEN (640x480 RGBA32), renders via Mode7 blitter
 * into an OS-allocated back buffer, then uses BLIT COPY to transfer directly
 * to the screen bitmap's VRAM (obtained via LockBitMapTagList).
 *
 * Build:
 *   AROS=../../../AROS/bin/ie-m68k/bin/ie-m68k/AROS/Developer
 *   ARCH=../../../AROS/arch/m68k-ie/include
 *   CC=../../../AROS/bin/ie-m68k/bin/linux-aarch64/tools/crosstools/m68k-aros-gcc
 *   $CC -O2 -m68020 -I$AROS/include -I$ARCH -L$AROS/lib \
 *       -o RotoHWc rotozoomer_aros_hw.c -lamiga -larossupport
 * ============================================================================ */

#include <proto/exec.h>
#include <proto/intuition.h>
#include <proto/cybergraphics.h>
#include <proto/dos.h>

#include <exec/memory.h>
#include <intuition/intuition.h>
#include <intuition/screens.h>
#include <cybergraphx/cybergraphics.h>
#include <utility/tagitem.h>

#include <ie_hwreg.h>

/* Screen dimensions */
#define RENDER_W        640
#define RENDER_H        480
#define LINE_BYTES      (RENDER_W * 4)
#define TEX_STRIDE      (256 * 4)
#define TEX_SIZE        (256 * 256 * 4)
#define BACKBUF_SIZE    (RENDER_W * RENDER_H * 4)
#define MEDIA_NAME_PTR  0xF2300
#define MEDIA_SUBSONG   0xF2304
#define MEDIA_CTRL      0xF2308
#define MEDIA_OP_PLAY   1
#define MEDIA_OP_STOP   2

/* Animation increments (8.8 fixed-point) */
#define ANGLE_INC       313
#define SCALE_INC       104

/* Raw key code for ESC */
#define RAWKEY_ESC      0x45

/* Sine table: 256 entries, signed 16-bit */
static const WORD sine_table[256] = {
    0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92,
    98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177,
    181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234,
    237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256,
    256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239,
    237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185,
    181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104,
    98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6,
    0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92,
    -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177,
    -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234,
    -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256,
    -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239,
    -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185,
    -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104,
    -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6
};

/* Reciprocal table: 256 entries, unsigned 16-bit */
static const UWORD recip_table[256] = {
    512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421,
    416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362,
    359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331,
    329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320,
    320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328,
    329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357,
    359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412,
    416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505,
    512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653,
    665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873,
    889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134,
    1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279,
    1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163,
    1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905,
    889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676,
    665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520
};

struct Library *CyberGfxBase = NULL;

/* Animation state */
static ULONG angle_accum, scale_accum;
static LONG var_ca, var_sa, var_u0, var_v0;
static char music_path[] = "sdk/examples/assets/music/chopper.ahx";

static void start_music(void)
{
    ie_write32(MEDIA_NAME_PTR, (ULONG)music_path);
    ie_write32(MEDIA_SUBSONG, 0);
    ie_write32(MEDIA_CTRL, MEDIA_OP_PLAY);
}

static void stop_music(void)
{
    ie_write32(MEDIA_CTRL, MEDIA_OP_STOP);
}

static void wait_vsync(void)
{
    while (ie_read32(IE_VIDEO_STATUS) & 2) {}   /* wait for vblank end */
    while (!(ie_read32(IE_VIDEO_STATUS) & 2)) {} /* wait for vblank start */
}

static void wait_blit(void)
{
    while (ie_read32(IE_BLT_CTRL) & IE_BLT_CTRL_BUSY) {}
}

static void compute_frame(void)
{
    ULONG angle_idx = (angle_accum >> 8) & 255;
    ULONG scale_idx = (scale_accum >> 8) & 255;

    LONG cos_val = sine_table[(angle_idx + 64) & 255];
    LONG sin_val = sine_table[angle_idx];
    LONG recip = (LONG)recip_table[scale_idx];

    LONG ca = cos_val * recip;
    LONG sa = sin_val * recip;
    var_ca = ca;
    var_sa = sa;

    /* u0 = 0x800000 - CA*320 + SA*240 */
    var_u0 = 0x800000 - (ca * 320) + (sa * 240);
    /* v0 = 0x800000 - SA*320 - CA*240 */
    var_v0 = 0x800000 - (sa * 320) - (ca * 240);
}

static void render_mode7(APTR texture_buf, APTR back_buf)
{
    ie_write32(IE_BLT_OP, IE_BLT_OP_MODE7);
    ie_write32(IE_BLT_SRC, (ULONG)texture_buf);
    ie_write32(IE_BLT_DST, (ULONG)back_buf);
    ie_write32(IE_BLT_WIDTH, RENDER_W);
    ie_write32(IE_BLT_HEIGHT, RENDER_H);
    ie_write32(IE_BLT_SRC_STRIDE, TEX_STRIDE);
    ie_write32(IE_BLT_DST_STRIDE, LINE_BYTES);
    ie_write32(IE_BLT_MODE7_TEX_W, 255);
    ie_write32(IE_BLT_MODE7_TEX_H, 255);
    ie_write32(IE_BLT_MODE7_U0, (ULONG)var_u0);
    ie_write32(IE_BLT_MODE7_V0, (ULONG)var_v0);
    ie_write32(IE_BLT_MODE7_DU_COL, (ULONG)var_ca);
    ie_write32(IE_BLT_MODE7_DV_COL, (ULONG)var_sa);
    ie_write32(IE_BLT_MODE7_DU_ROW, (ULONG)(-var_sa));
    ie_write32(IE_BLT_MODE7_DV_ROW, (ULONG)var_ca);
    ie_write32(IE_BLT_CTRL, IE_BLT_CTRL_START);
    wait_blit();
}

static int load_texture(APTR texture_buf)
{
    /* Load texture from PROGDIR:rotozoomtexture.raw */
    BPTR fh = Open("PROGDIR:rotozoomtexture.raw", MODE_OLDFILE);
    LONG bytes_read;
    if (!fh) {
        return 0;
    }
    bytes_read = Read(fh, texture_buf, TEX_SIZE);
    Close(fh);
    if (bytes_read != TEX_SIZE) {
        return 0;
    }
    return 1;
}

static void advance_animation(void)
{
    angle_accum = (angle_accum + ANGLE_INC) & 0xFFFF;
    scale_accum = (scale_accum + SCALE_INC) & 0xFFFF;
}

int main(void)
{
    struct Screen *screen = NULL;
    struct Window *window = NULL;
    APTR texture_buf = NULL, back_buf = NULL;
    ULONG display_id;
    int running = 1;

    /* Open cybergraphics.library */
    CyberGfxBase = OpenLibrary("cybergraphics.library", 40);
    if (!CyberGfxBase)
        return 20;

    /* Allocate buffers */
    texture_buf = AllocMem(TEX_SIZE, MEMF_ANY | MEMF_CLEAR);
    if (!texture_buf) goto cleanup;

    back_buf = AllocMem(BACKBUF_SIZE, MEMF_ANY | MEMF_CLEAR);
    if (!back_buf) goto cleanup;

    /* Load texture */
    if (!load_texture(texture_buf))
        goto cleanup;
    start_music();

    /* Find best display mode */
    {
        struct TagItem mode_tags[] = {
            { CYBRBIDTG_NominalWidth, RENDER_W },
            { CYBRBIDTG_NominalHeight, RENDER_H },
            { CYBRBIDTG_Depth, 32 },
            { TAG_DONE, 0 }
        };
        display_id = BestCModeIDTagList(mode_tags);
        if (display_id == (ULONG)INVALID_ID)
            goto cleanup;
    }

    /* Open screen */
    {
        struct TagItem scr_tags[] = {
            { SA_Type, CUSTOMSCREEN },
            { SA_DisplayID, display_id },
            { SA_Width, RENDER_W },
            { SA_Height, RENDER_H },
            { SA_Depth, 32 },
            { SA_Title, (IPTR)"Rotozoomer HW (C)" },
            { SA_ShowTitle, FALSE },
            { SA_Quiet, TRUE },
            { TAG_DONE, 0 }
        };
        screen = OpenScreenTagList(NULL, scr_tags);
        if (!screen) goto cleanup;
    }

    /* Open backdrop window */
    {
        struct TagItem win_tags[] = {
            { WA_CustomScreen, (IPTR)screen },
            { WA_Width, RENDER_W },
            { WA_Height, RENDER_H },
            { WA_IDCMP, IDCMP_RAWKEY },
            { WA_Borderless, TRUE },
            { WA_Backdrop, TRUE },
            { WA_Activate, TRUE },
            { TAG_DONE, 0 }
        };
        window = OpenWindowTagList(NULL, win_tags);
        if (!window) goto cleanup;
    }

    /* Init animation */
    angle_accum = 0;
    scale_accum = 0;

    /* Main loop */
    while (running) {
        compute_frame();
        render_mode7(texture_buf, back_buf);

        /* Lock bitmap, blit, unlock */
        {
            ULONG vram_addr = 0, vram_stride = 0;
            struct TagItem lbmi_tags[] = {
                { LBMI_BASEADDRESS, (IPTR)&vram_addr },
                { LBMI_BYTESPERROW, (IPTR)&vram_stride },
                { TAG_DONE, 0 }
            };
            APTR lock = LockBitMapTagList(screen->RastPort.BitMap, lbmi_tags);
            if (lock) {
                ie_write32(IE_BLT_OP, IE_BLT_OP_COPY);
                ie_write32(IE_BLT_SRC, (ULONG)back_buf);
                ie_write32(IE_BLT_DST, vram_addr);
                ie_write32(IE_BLT_WIDTH, RENDER_W);
                ie_write32(IE_BLT_HEIGHT, RENDER_H);
                ie_write32(IE_BLT_SRC_STRIDE, LINE_BYTES);
                ie_write32(IE_BLT_DST_STRIDE, vram_stride);
                ie_write32(IE_BLT_CTRL, IE_BLT_CTRL_START);
                wait_blit();
                UnLockBitMap(lock);
            }
        }

        wait_vsync();
        advance_animation();

        /* Check IDCMP */
        {
            struct IntuiMessage *msg;
            while ((msg = (struct IntuiMessage *)GetMsg(window->UserPort))) {
                ULONG cl = msg->Class;
                UWORD code = msg->Code;
                ReplyMsg((struct Message *)msg);
                if (cl == IDCMP_RAWKEY && code == RAWKEY_ESC)
                    running = 0;
            }
        }
    }

cleanup:
    stop_music();
    if (window)      CloseWindow(window);
    if (screen)      CloseScreen(screen);
    if (back_buf)    FreeMem(back_buf, BACKBUF_SIZE);
    if (texture_buf) FreeMem(texture_buf, TEX_SIZE);
    if (CyberGfxBase) CloseLibrary(CyberGfxBase);
    return 0;
}
