#ifndef INTUITIONENGINE_H
#define INTUITIONENGINE_H

/*
 * Intuition Engine's freestanding hardware interface.  Include the selected
 * compiler's <stdint.h> before this file. The bundled IE64 toolchain ships
 * its freestanding standard headers; external toolchains provide their own.
 */
#include <stdint.h>

#if (defined(IE_TARGET_IE64) + defined(IE_TARGET_M68K) + defined(IE_TARGET_Z80) + defined(IE_TARGET_6502) + defined(IE_TARGET_X86)) != 1
#error "define exactly one IE_TARGET_IE64, IE_TARGET_M68K, IE_TARGET_Z80, IE_TARGET_6502 or IE_TARGET_X86"
#endif

#if defined(IE_TARGET_IE64)
# if !defined(__ie64__)
#  error "IE_TARGET_IE64 requires ie64-cproc"
# endif
# define IE_HAS_BANK_WINDOWS 0
# define IE_HAS_X86_PORT_IO 0
# define IE_HAS_IE64_CONTROL_REGISTERS 1
# define IE_HAS_IE64_ATOMICS 1
# define IE_HAS_IE64_FPU 1
# define IE_ADDRESS_BITS 64
# define IE_BIG_ENDIAN 0
#elif defined(IE_TARGET_M68K)
# if !defined(__GNUC__) && !defined(__VBCC__)
#  error "IE_TARGET_M68K requires GCC or VBCC"
# endif
# define IE_HAS_BANK_WINDOWS 0
# define IE_HAS_X86_PORT_IO 0
# define IE_HAS_IE64_CONTROL_REGISTERS 0
# define IE_HAS_IE64_ATOMICS 0
# define IE_HAS_IE64_FPU 0
# define IE_ADDRESS_BITS 32
# define IE_BIG_ENDIAN 1
#elif defined(IE_TARGET_Z80)
# if !defined(__VBCC__)
#  error "IE_TARGET_Z80 requires VBCC"
# endif
# define IE_HAS_BANK_WINDOWS 1
# define IE_HAS_X86_PORT_IO 0
# define IE_HAS_IE64_CONTROL_REGISTERS 0
# define IE_HAS_IE64_ATOMICS 0
# define IE_HAS_IE64_FPU 0
# define IE_ADDRESS_BITS 16
# define IE_BIG_ENDIAN 0
#elif defined(IE_TARGET_6502)
# if !defined(__VBCC__) && !defined(__SDCC)
#  error "IE_TARGET_6502 requires VBCC or SDCC"
# endif
# define IE_HAS_BANK_WINDOWS 1
# define IE_HAS_X86_PORT_IO 0
# define IE_HAS_IE64_CONTROL_REGISTERS 0
# define IE_HAS_IE64_ATOMICS 0
# define IE_HAS_IE64_FPU 0
# define IE_ADDRESS_BITS 16
# define IE_BIG_ENDIAN 0
#elif defined(IE_TARGET_X86)
# if !defined(__GNUC__)
#  error "IE_TARGET_X86 requires GCC"
# endif
# define IE_HAS_BANK_WINDOWS 0
# define IE_HAS_X86_PORT_IO 1
# define IE_HAS_IE64_CONTROL_REGISTERS 0
# define IE_HAS_IE64_ATOMICS 0
# define IE_HAS_IE64_FPU 0
# define IE_ADDRESS_BITS 32
# define IE_BIG_ENDIAN 0
#endif

#if !defined(UINT8_MAX) || !defined(UINT16_MAX) || !defined(UINT32_MAX)
#error "the selected compiler must provide uint8_t, uint16_t and uint32_t"
#endif
#if IE_HAS_IE64_CONTROL_REGISTERS && !defined(UINT64_MAX)
#error "ie64-cproc must provide uint64_t"
#endif

#define IE_REG8(address)  (*(volatile uint8_t *)(uintptr_t)(address))
#define IE_REG16(address) (*(volatile uint16_t *)(uintptr_t)(address))
#define IE_REG32(address) (*(volatile uint32_t *)(uintptr_t)(address))

/* Common, target-normalised hardware map.  See architecture.md for access semantics. */
#define IE_PROGRAM_START 0x001000u
#define IE_VIDEO_CTRL 0x0f0000u
#define IE_VIDEO_MODE 0x0f0004u
#define IE_VIDEO_STATUS 0x0f0008u
#define IE_VIDEO_STATUS_VBLANK 0x02u
#define IE_VIDEO_STATUS_FB_ERROR 0x04u
#define IE_VIDEO_COPPER_CTRL 0x0f000cu
#define IE_VIDEO_COPPER_PTR 0x0f0010u
#define IE_VIDEO_BLITTER_CTRL 0x0f001cu
#define IE_VIDEO_BLITTER_OP 0x0f0020u
#define IE_VIDEO_BLITTER_SRC 0x0f0024u
#define IE_VIDEO_BLITTER_DST 0x0f0028u
#define IE_VIDEO_BLITTER_WIDTH 0x0f002cu
#define IE_VIDEO_BLITTER_HEIGHT 0x0f0030u
#define IE_VIDEO_BLITTER_SOURCE_STRIDE 0x0f0034u
#define IE_VIDEO_BLITTER_DESTINATION_STRIDE 0x0f0038u
#define IE_VIDEO_BLITTER_COLOUR 0x0f003cu
#define IE_VIDEO_BLITTER_MASK 0x0f0040u
#define IE_VIDEO_BLITTER_STATUS 0x0f0044u
#define IE_VIDEO_RASTER_Y 0x0f0048u
#define IE_VIDEO_RASTER_HEIGHT 0x0f004cu
#define IE_VIDEO_RASTER_COLOUR 0x0f0050u
#define IE_VIDEO_RASTER_CONTROL 0x0f0054u
#define IE_VIDEO_MODE7_U0 0x0f0058u
#define IE_VIDEO_MODE7_V0 0x0f005cu
#define IE_VIDEO_MODE7_DU_COLUMN 0x0f0060u
#define IE_VIDEO_MODE7_DV_COLUMN 0x0f0064u
#define IE_VIDEO_MODE7_DU_ROW 0x0f0068u
#define IE_VIDEO_MODE7_DV_ROW 0x0f006cu
#define IE_VIDEO_MODE7_TEXTURE_WIDTH 0x0f0070u
#define IE_VIDEO_MODE7_TEXTURE_HEIGHT 0x0f0074u
#define IE_VIDEO_PALETTE_INDEX 0x0f0078u
#define IE_VIDEO_PALETTE_DATA 0x0f007cu
#define IE_VIDEO_COLOR_MODE 0x0f0080u
#define IE_VIDEO_FB_BASE 0x0f0084u
#define IE_INPUT_TERM_OUT 0x0f0700u
#define IE_INPUT_TERM_STATUS 0x0f0704u
#define IE_INPUT_TERM_IN 0x0f0708u
#define IE_INPUT_TERM_LINE_STATUS 0x0f070cu
#define IE_INPUT_TERM_ECHO 0x0f0710u
#define IE_INPUT_SCAN_CODE 0x0f0740u
#define IE_INPUT_SCAN_STATUS 0x0f0744u
#define IE_INPUT_SCAN_MODIFIERS 0x0f0748u
#define IE_INPUT_MOUSE_CTRL 0x0f074cu
#define IE_INPUT_MOUSE_DX 0x0f0754u
#define IE_INPUT_MOUSE_DY 0x0f0758u
#define IE_SYSTEM_RTC_EPOCH 0x0f0750u
#define IE_SYSTEM_RTC_MONO_USEC_LO 0x0f075cu
#define IE_SYSTEM_RTC_MONO_USEC_HI 0x0f0760u
#define IE_AUDIO_BASE 0x0f0800u
#define IE_AUDIO_CONTROL 0x0f0800u
#define IE_AUDIO_ENVELOPE_SHAPE 0x0f0804u
#define IE_AUDIO_FILTER_CUTOFF 0x0f0820u
#define IE_AUDIO_FILTER_RESONANCE 0x0f0824u
#define IE_AUDIO_FILTER_TYPE 0x0f0828u
#define IE_AUDIO_FILTER_OFF 0u
#define IE_AUDIO_FILTER_LOWPASS 1u
#define IE_AUDIO_FILTER_HIGHPASS 2u
#define IE_AUDIO_FILTER_BANDPASS 3u
#define IE_AUDIO_SQUARE_FREQUENCY 0x0f0900u
#define IE_AUDIO_SQUARE_VOLUME 0x0f0904u
#define IE_AUDIO_SQUARE_CONTROL 0x0f0908u
#define IE_AUDIO_TRIANGLE_FREQUENCY 0x0f0940u
#define IE_AUDIO_TRIANGLE_VOLUME 0x0f0944u
#define IE_AUDIO_TRIANGLE_CONTROL 0x0f0948u
#define IE_AUDIO_SINE_FREQUENCY 0x0f0980u
#define IE_AUDIO_SINE_VOLUME 0x0f0984u
#define IE_AUDIO_SINE_CONTROL 0x0f0988u
#define IE_AUDIO_NOISE_FREQUENCY 0x0f09c0u
#define IE_AUDIO_NOISE_VOLUME 0x0f09c4u
#define IE_AUDIO_NOISE_CONTROL 0x0f09c8u
#define IE_AUDIO_NOISE_WHITE 0u
#define IE_AUDIO_NOISE_PERIODIC 1u
#define IE_AUDIO_NOISE_METALLIC 2u
#define IE_AUDIO_NOISE_PSG 3u
#define IE_FILE_BASE 0x0f2000u
#define IE_COPROC_BASE 0x0f2340u
#define IE_VOODOO_BASE 0x0f8000u
#define IE_VOODOO_END 0x0f87ffu
#define IE_VOODOO_STATUS 0x0f8000u
#define IE_VOODOO_ENABLE 0x0f8004u
#define IE_VOODOO_VERTEX_AX 0x0f8008u
#define IE_VOODOO_VERTEX_AY 0x0f800cu
#define IE_VOODOO_VERTEX_BX 0x0f8010u
#define IE_VOODOO_VERTEX_BY 0x0f8014u
#define IE_VOODOO_VERTEX_CX 0x0f8018u
#define IE_VOODOO_VERTEX_CY 0x0f801cu
#define IE_VOODOO_TRIANGLE_COMMAND 0x0f8080u
#define IE_VOODOO_FAST_FILL_COMMAND 0x0f8124u
#define IE_VOODOO_SWAP_BUFFER_COMMAND 0x0f8128u
#define IE_VOODOO_TEXTURE_MODE 0x0f8300u
#define IE_VOODOO_TEXTURE_BASE0 0x0f830cu
#define IE_VOODOO_TEXTURE_WIDTH 0x0f8330u
#define IE_VOODOO_TEXTURE_HEIGHT 0x0f8334u
#define IE_VOODOO_COMMAND_POINTER 0x0f833cu
#define IE_VOODOO_COMMAND_COUNT 0x0f8340u
#define IE_VOODOO_COMMAND_SUBMIT 0x0f8344u
#define IE_VOODOO_COMMAND_SUBMIT_REPLAY 0x00000001u
#define IE_VOODOO_COMMAND_SUBMIT_REPLAY_LE 0x00000002u
#define IE_EXEC_BASE 0x0f5000u
#define IE_FILE_OPEN_READ 0x01u
#define IE_FILE_OPEN_WRITE 0x02u
#define IE_FILE_OPEN_CREATE 0x04u
#define IE_FILE_OPEN_TRUNCATE 0x08u
#define IE_FILE_OPEN_APPEND 0x10u
#define IE_FILE_OPEN_EXCLUSIVE 0x20u
#define IE_FILE_SEEK_SET 0
#define IE_FILE_SEEK_CUR 1
#define IE_FILE_SEEK_END 2
/* Legacy IE64 spellings retained for source compatibility. */
#define IE64_OPEN_READ IE_FILE_OPEN_READ
#define IE64_OPEN_WRITE IE_FILE_OPEN_WRITE
#define IE64_OPEN_CREATE IE_FILE_OPEN_CREATE
#define IE64_OPEN_TRUNCATE IE_FILE_OPEN_TRUNCATE
#define IE64_OPEN_APPEND IE_FILE_OPEN_APPEND
#define IE64_OPEN_EXCLUSIVE IE_FILE_OPEN_EXCLUSIVE
#define IE64_SEEK_SET IE_FILE_SEEK_SET
#define IE64_SEEK_CUR IE_FILE_SEEK_CUR
#define IE64_SEEK_END IE_FILE_SEEK_END
#define IE_NET_SOCKET_BASE 0x0f2500u
#define IE_NET_SOCKET_COMMAND (IE_NET_SOCKET_BASE + 0x00u)
#define IE_NET_SOCKET_REQUEST (IE_NET_SOCKET_BASE + 0x04u)
#define IE_NET_SOCKET_REQUEST_LENGTH (IE_NET_SOCKET_BASE + 0x08u)
#define IE_NET_SOCKET_RESULT1 (IE_NET_SOCKET_BASE + 0x0cu)
#define IE_NET_SOCKET_RESULT2 (IE_NET_SOCKET_BASE + 0x10u)
#define IE_NET_SOCKET_ERRNO (IE_NET_SOCKET_BASE + 0x14u)
#define IE_NET_SOCKET_HOSTERRNO (IE_NET_SOCKET_BASE + 0x18u)
#define IE_NET_SOCKET_STATUS (IE_NET_SOCKET_BASE + 0x1cu)
#define IE_NET_SOCKET_EVENTS (IE_NET_SOCKET_BASE + 0x20u)
#define IE_NET_CMD_SOCKET 1u
#define IE_NET_CMD_BIND 2u
#define IE_NET_CMD_LISTEN 3u
#define IE_NET_CMD_ACCEPT 4u
#define IE_NET_CMD_CONNECT 5u
#define IE_NET_CMD_SENDTO 6u
#define IE_NET_CMD_RECVFROM 7u
#define IE_NET_CMD_SHUTDOWN 8u
#define IE_NET_CMD_SETSOCKOPT 9u
#define IE_NET_CMD_GETSOCKOPT 10u
#define IE_NET_CMD_GETSOCKNAME 11u
#define IE_NET_CMD_GETPEERNAME 12u
#define IE_NET_CMD_IOCTL 13u
#define IE_NET_CMD_CLOSE 14u
#define IE_NET_CMD_WAITSELECT 15u
#define IE_NET_CMD_GETHOSTBYNAME 16u
#define IE_NET_CMD_GETHOSTBYADDR 17u
#define IE_NET_CMD_GETHOSTNAME 18u
#define IE_NET_CMD_DUP2 19u
#define IE_NET_CMD_GETEVENTS 20u
#define IE_NET_CMD_RELEASE 21u
#define IE_NET_CMD_RELEASECOPY 22u
#define IE_NET_CMD_OBTAIN 23u

#if IE_HAS_BANK_WINDOWS
# define IE_BANK1_WINDOW 0x2000u
# define IE_BANK2_WINDOW 0x4000u
# define IE_BANK3_WINDOW 0x6000u
# define IE_VRAM_WINDOW 0x8000u
# define IE_BANK_SIZE 0x2000u
# define IE_VRAM_BANK_SIZE 0x4000u
# define IE_BANK1_REG_LO 0xf700u
# define IE_BANK1_REG_HI 0xf701u
# define IE_BANK2_REG_LO 0xf702u
# define IE_BANK2_REG_HI 0xf703u
# define IE_BANK3_REG_LO 0xf704u
# define IE_BANK3_REG_HI 0xf705u
# define IE_VRAM_BANK_REG 0xf7f0u
static void ie_bank_select(volatile uint8_t *low, volatile uint8_t *high, uint16_t bank) {
	*low = (uint8_t)bank; *high = (uint8_t)(bank >> 8);
}
#endif

#if IE_HAS_X86_PORT_IO
static inline uint8_t ie_x86_in8(uint16_t port) { uint8_t value; __asm__ volatile ("inb %1, %0" : "=a"(value) : "d"(port)); return value; }
static inline void ie_x86_out8(uint16_t port, uint8_t value) { __asm__ volatile ("outb %0, %1" : : "a"(value), "d"(port)); }
#endif

#if IE_HAS_IE64_CONTROL_REGISTERS
enum ie64_control_register {
	IE64_CR_PTBR = 0, IE64_CR_FAULT_ADDR = 1, IE64_CR_FAULT_CAUSE = 2,
	IE64_CR_FAULT_PC = 3, IE64_CR_TRAP_VEC = 4, IE64_CR_MMU_CTRL = 5,
	IE64_CR_TP = 6, IE64_CR_INTR_VEC = 7, IE64_CR_KSP = 8,
	IE64_CR_TIMER_PERIOD = 9, IE64_CR_TIMER_COUNT = 10,
	IE64_CR_TIMER_CTRL = 11, IE64_CR_USP = 12, IE64_CR_PREV_MODE = 13,
	IE64_CR_SAVED_SUA = 14, IE64_CR_RAM_SIZE_BYTES = 15
};
uint64_t __builtin_ie64_mfcr(unsigned int); void __builtin_ie64_mtcr(unsigned int, uint64_t);
void __builtin_ie64_tlbinval(uint64_t); void __builtin_ie64_tlbflush(void); void __builtin_ie64_suaen(void); void __builtin_ie64_suadis(void);
_Noreturn void __builtin_ie64_eret(void); _Noreturn void __builtin_ie64_rti(void); _Noreturn void __builtin_ie64_halt(void);
void __builtin_ie64_nop(void); void __builtin_ie64_sei(void); void __builtin_ie64_cli(void); void __builtin_ie64_wait(unsigned int); uint64_t __builtin_ie64_syscall(unsigned int); uint64_t __builtin_ie64_smode(void);
#endif

#if IE_HAS_IE64_ATOMICS
uint64_t __builtin_ie64_cas(volatile uint64_t *, uint64_t, uint64_t); uint64_t __builtin_ie64_xchg(volatile uint64_t *, uint64_t);
uint64_t __builtin_ie64_faa(volatile uint64_t *, uint64_t); uint64_t __builtin_ie64_fand(volatile uint64_t *, uint64_t); uint64_t __builtin_ie64_for(volatile uint64_t *, uint64_t); uint64_t __builtin_ie64_fxor(volatile uint64_t *, uint64_t);
static uint64_t ie64_atomic_compare_exchange(volatile uint64_t *p, uint64_t a, uint64_t b) { return __builtin_ie64_cas(p, a, b); }
static uint64_t ie64_atomic_exchange(volatile uint64_t *p, uint64_t a) { return __builtin_ie64_xchg(p, a); }
static uint64_t ie64_atomic_fetch_add(volatile uint64_t *p, uint64_t a) { return __builtin_ie64_faa(p, a); }
static uint64_t ie64_atomic_fetch_and(volatile uint64_t *p, uint64_t a) { return __builtin_ie64_fand(p, a); }
static uint64_t ie64_atomic_fetch_or(volatile uint64_t *p, uint64_t a) { return __builtin_ie64_for(p, a); }
static uint64_t ie64_atomic_fetch_xor(volatile uint64_t *p, uint64_t a) { return __builtin_ie64_fxor(p, a); }
#endif

#if IE_HAS_IE64_FPU
float __builtin_ie64_fmovecr(unsigned int); double __builtin_ie64_dmovecr(unsigned int); float __builtin_ie64_fmod(float, float); double __builtin_ie64_dmod(double, double);
float __builtin_ie64_fabs(float); double __builtin_ie64_dabs(double); float __builtin_ie64_fint(float); double __builtin_ie64_dint(double); float __builtin_ie64_fcvtds(double); double __builtin_ie64_fcvtsd(float);
float __builtin_ie64_fsin(float); float __builtin_ie64_fcos(float); float __builtin_ie64_ftan(float); float __builtin_ie64_fatan(float); float __builtin_ie64_flog(float); float __builtin_ie64_fexp(float); float __builtin_ie64_fpow(float, float); float __builtin_ie64_fsqrt(float);
double __builtin_ie64_dsin(double); double __builtin_ie64_dcos(double); double __builtin_ie64_dtan(double); double __builtin_ie64_datan(double); double __builtin_ie64_dlog(double); double __builtin_ie64_dexp(double); double __builtin_ie64_dpow(double, double); double __builtin_ie64_dsqrt(double);
uint64_t __builtin_ie64_fmovsr(void); uint64_t __builtin_ie64_fmovcr(void);
void __builtin_ie64_fmovsc(unsigned int); void __builtin_ie64_fmovcc(unsigned int);
#endif

#if defined(IE_TARGET_IE64)
int __ie64_console_read(void *, unsigned long, unsigned long *); int __ie64_console_write(const void *, unsigned long, unsigned long *);
int __ie64_file_open(const char *, unsigned int, uint64_t *); int __ie64_file_read(uint64_t, void *, unsigned long, unsigned long *); int __ie64_file_write(uint64_t, const void *, unsigned long, unsigned long *);
int __ie64_file_seek(uint64_t, int64_t, int, int64_t *); int __ie64_file_close(uint64_t); int __ie64_file_remove(const char *); int __ie64_file_rename(const char *, const char *); int __ie64_file_tmp(uint64_t *); int __ie64_monotonic_ticks(uint64_t *); _Noreturn void __ie64_terminate(int);
static void ie64_nop(void) { __builtin_ie64_nop(); } static void ie64_enable_interrupts(void) { __builtin_ie64_sei(); } static void ie64_disable_interrupts(void) { __builtin_ie64_cli(); } static _Noreturn void __ie64_assert_fail(void) { __builtin_ie64_halt(); }
#endif

#endif
