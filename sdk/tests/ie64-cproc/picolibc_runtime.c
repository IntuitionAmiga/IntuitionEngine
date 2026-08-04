#include <errno.h>
#include <intuitionengine.h>
#include <limits.h>
#include <stdint.h>
#include <stdbit.h>
#include <stdckdint.h>
#include <stdio.h>
#include <stdlib.h>

#define RESULT (*(volatile uint64_t *)(uintptr_t)0x00080000)
#define PASS UINT64_C(0x5049434f50415353)
#define HANDLE_BASE UINT64_C(0x1234567800000000)

static unsigned int next_handle;
static unsigned int close_count;
static unsigned int failures;
static int64_t positions[8];

int
__ie64_file_open(const char *path, unsigned int flags, uint64_t *handle)
{
    unsigned int expected;
    if (path[0] == 'a')
        expected = IE64_OPEN_READ | IE64_OPEN_WRITE | IE64_OPEN_CREATE
                   | IE64_OPEN_TRUNCATE;
    else if (path[0] == 'b')
        expected = IE64_OPEN_READ;
    else
        expected = IE64_OPEN_WRITE | IE64_OPEN_CREATE | IE64_OPEN_APPEND;
    if (flags != expected)
        failures |= 1u;
    *handle = HANDLE_BASE + ++next_handle;
    return 0;
}

int
__ie64_file_read(uint64_t handle, void *buffer, size_t size, size_t *done)
{
    (void)buffer;
    if (handle < HANDLE_BASE || handle > HANDLE_BASE + 7)
        failures |= 2u;
    *done = size ? 0 : 0;
    return 0;
}

int
__ie64_file_write(uint64_t handle, const void *buffer, size_t size, size_t *done)
{
    (void)buffer;
    if (handle < HANDLE_BASE || handle > HANDLE_BASE + 7)
        failures |= 2u;
    *done = size;
    return 0;
}

int
__ie64_file_seek(uint64_t handle, int64_t offset, int whence, int64_t *position)
{
    unsigned int index = (unsigned int)(handle - HANDLE_BASE);
    int64_t base;
    if (index == 0 || index >= 8) {
        failures |= 2u;
        return EBADF;
    }
    if (whence == IE64_SEEK_SET)
        base = 0;
    else if (whence == IE64_SEEK_CUR)
        base = positions[index];
    else if (whence == IE64_SEEK_END)
        base = INT64_C(0x200000000);
    else
        return EINVAL;
    if (offset < -base)
        return EINVAL;
    positions[index] = base + offset;
    *position = positions[index];
    return 0;
}

int
__ie64_file_close(uint64_t handle)
{
    if (handle < HANDLE_BASE || handle > HANDLE_BASE + 7)
        failures |= 4u;
    close_count++;
    return handle == HANDLE_BASE + 2 ? EIO : 0;
}

int __ie64_file_remove(const char *path) { return path[0] == 'x' ? 0 : ENOENT; }
int __ie64_file_rename(const char *old_path, const char *new_path)
{
    return old_path[0] == 'x' && new_path[0] == 'y' ? 0 : ENOENT;
}
int
__ie64_file_tmp(uint64_t *handle)
{
    *handle = HANDLE_BASE + ++next_handle;
    return 0;
}

_Noreturn void __builtin_ie64_halt(void);

_Noreturn void
__ie64_terminate(int status)
{
    if (status == 23 && close_count == 4 && failures == 0)
        RESULT = PASS;
    else
        RESULT = ((uint64_t)(unsigned int)status << 32)
                 | ((uint64_t)close_count << 16) | failures;
    __builtin_ie64_halt();
    __builtin_unreachable();
}

int
main(void)
{
    FILE *first;
    FILE *second;
    FILE *temporary;
    void *allocation;
    void *reused;
    unsigned int checked;
    unsigned int side_effect = 8;
    int signed_checked;
    int mixed_checked;
    char formatted[32];
    int scanned = 0;

    first = fopen("alpha", "w+");
    second = fopen("beta", "r");
    if (!first || !second)
        failures |= 8u;
    if (fseeko(first, INT64_C(0x100000007), SEEK_SET) != 0
        || ftello(first) != INT64_C(0x100000007))
        failures |= 16u;
    if (fwrite("xy", 1, 2, first) != 2 || fflush(first) != 0)
        failures |= 65536u;
    if (fclose(first) != 0)
        failures |= 32u;
    if (freopen("console", "a", stdout) != stdout)
        failures |= 64u;
    if (freopen(NULL, "a+", stdout) != stdout)
        failures |= 128u;
    if (fprintf(stdout, "%s:%d", "ok", 42) < 0 || fflush(stdout) != 0)
        failures |= 131072u;
    temporary = tmpfile();
    if (!temporary)
        failures |= 256u;
    if (remove("x") != 0 || rename("x", "y") != 0)
        failures |= 512u;
    if (snprintf(formatted, sizeof formatted, "%u:%s", 73u, "c23") != 6
        || sscanf(formatted, "%d:c23", &scanned) != 1 || scanned != 73)
        failures |= 262144u;

    allocation = malloc(128);
    free(allocation);
    reused = malloc(128);
    if (!allocation || reused != allocation)
        failures |= 1024u;
    free(reused);
    if (stdc_leading_zeros(1u) != 31 || stdc_trailing_zeros(8u) != 3
        || stdc_count_ones(0xf0u) != 4 || stdc_bit_width(9u) != 4
        || stdc_bit_floor(9u) != 8 || stdc_bit_ceil(9u) != 16
        || !stdc_has_single_bit(16u)
        || stdc_trailing_zeros(side_effect++) != 3 || side_effect != 9)
        failures |= 2048u;
    if (ckd_add(&checked, UINT_MAX, 1u) == false || checked != 0)
        failures |= 4096u;
    if (ckd_mul(&checked, 6u, 7u) || checked != 42u)
        failures |= 8192u;
    if (!ckd_add(&signed_checked, INT_MAX, 1))
        failures |= 16384u;
    if (signed_checked != INT_MIN)
        failures |= 32768u;
    if (!ckd_add(&mixed_checked, UINT_MAX, 1u)
        || (unsigned int)mixed_checked != 0u)
        failures |= 524288u;
    allocation = malloc(32);
    free_sized(allocation, 32);
    allocation = aligned_alloc(16, 32);
    free_aligned_sized(allocation, 16, 32);
    return 23;
}
