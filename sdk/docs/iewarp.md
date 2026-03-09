# iewarp.library Reference

## Overview

iewarp.library accelerates AROS compute operations by offloading them to the IE64 coprocessor. Acceleration happens through two paths: AROS kernel and workbench subsystems (exec, graphics, IEGfx HIDD, cybergraphics, AHI, etc.) dispatch directly to the coprocessor via MMIO registers, while iewarp.library provides a public API for operations that have no natural kernel-level consumer (FP_BATCH, MATRIX_MUL, CRC32, AUDIO_MIX, GRADIENT_FILL) and for application-level use. Both paths write coprocessor MMIO registers at `0xF2340`, which enqueues work into the IE64 worker's ring buffer. The IE64 worker executes the operation using JIT-compiled native code on the host (amd64 or arm64).

Dispatch functions (IEWarpMemCpy, IEWarpBlitCopy, etc.) return a **ticket** (`ULONG`). Callers use `IEWarpWait(ticket)` to block until the operation completes. A ticket value of `0` means the operation was handled inline by M68K fallback code and is already complete. Control and query functions (IEWarpWait, IEWarpPoll, IEWarpGetThreshold, IEWarpGetStats) return status codes or query results, not tickets.

## Architecture

Most AROS subsystems dispatch directly to the coprocessor via MMIO, bypassing iewarp.library entirely:

```
Path 1 — Kernel/workbench direct dispatch (17 ops):
  M68K app → AROS library impl (e.g. CopyMem, BltBitMap)
    → ie_write32(IE_COPROC_*) → ring buffer → IE64 worker (JIT)

Path 2 — iewarp.library public API (5 ops + all ops for app use):
  M68K app → iewarp.library (IEWarpFPBatch, IEWarpCRC32, etc.)
    → ie_write32(IE_COPROC_*) → ring buffer → IE64 worker (JIT)
```

The library opens at resident priority -5 during AROS boot. On init it starts the IE64 coprocessor worker from `sdk/examples/asm/iewarp_service.ie64`, calibrates dispatch overhead with a NOP round-trip, and installs a Level 6 completion interrupt handler.

## API Reference

All functions use Amiga register calling conventions. LVO slot numbers match the `.conf` file (slot 1 is reserved/skip).

### Memory Operations

#### IEWarpMemCpy

```c
ULONG IEWarpMemCpy(APTR dst, APTR src, ULONG len)  /* (A0, A1, D0) */
```

Copy `len` bytes from `src` to `dst`. Regions must not overlap. Falls back to `CopyMem()` when below threshold.

**LVO**: -36 (slot 6)

#### IEWarpMemCpyQuick

```c
ULONG IEWarpMemCpyQuick(APTR dst, APTR src, ULONG len)  /* (A0, A1, D0) */
```

Copy `len` bytes assuming ULONG (4-byte) alignment of both `src` and `dst`. Falls back to `CopyMemQuick()`.

**LVO**: -42 (slot 7)

#### IEWarpMemSet

```c
ULONG IEWarpMemSet(APTR dst, ULONG val, ULONG len)  /* (A0, D0, D1) */
```

Fill `len` bytes at `dst` with the low byte of `val`. M68K fallback is a byte-fill loop.

**LVO**: -48 (slot 8)

#### IEWarpMemMove

```c
ULONG IEWarpMemMove(APTR dst, APTR src, ULONG len)  /* (A0, A1, D0) */
```

Copy `len` bytes handling overlapping regions correctly (copies backward when necessary).

**LVO**: -54 (slot 9)

### Graphics Operations

#### IEWarpBlitCopy

```c
ULONG IEWarpBlitCopy(APTR src, APTR dst, UWORD width, UWORD height,
                     UWORD srcStride, UWORD dstStride, UBYTE minterm)
                     /* (A0, A1, D0, D1, D2, D3, D4) */
```

Rectangular blit copy. `width` is in bytes. Currently only minterm `0xC0` (straight copy) is supported — any other minterm returns ticket 0 immediately (no-op, caller falls back to software). Width and height are packed into `reqLen` as `width | (height << 16)`. Strides packed into `respCap`.

**LVO**: -60 (slot 10)

#### IEWarpBlitMask

```c
ULONG IEWarpBlitMask(APTR src, APTR dst, APTR mask, UWORD width, UWORD height,
                     UWORD srcStride, UWORD dstStride)
                     /* (A0, A1, A2, D0, D1, D2, D3) */
```

Masked blit. Pixels are copied only where the corresponding `mask` byte is non-zero. The mask pointer is passed via the `flags` field (written to `COPROC_TIMEOUT` before ENQUEUE). No M68K fallback; returns 0 if below threshold.

**LVO**: -66 (slot 11)

#### IEWarpBlitScale

```c
ULONG IEWarpBlitScale(APTR src, APTR dst, UWORD srcW, UWORD srcH,
                      UWORD dstW, UWORD dstH, UWORD srcStride, UWORD dstStride)
                      /* (A0, A1, D0, D1, D2, D3, D4, D5) */
```

Scale blit from source dimensions to destination dimensions using DDA nearest-neighbor sampling. Source and destination strides are passed via the `flags` field (packed as `srcStride | (dstStride << 16)`). No M68K fallback.

**LVO**: -72 (slot 12)

#### IEWarpBlitConvert

```c
ULONG IEWarpBlitConvert(APTR src, APTR dst, UWORD width, UWORD height,
                        UWORD srcFmt, UWORD dstFmt)
                        /* (A0, A1, D0, D1, D2, D3) */
```

Pixel format conversion. `srcFmt` and `dstFmt` specify bytes-per-pixel (e.g. srcFmt=1 for CLUT8, dstFmt=4 for RGBA32). CLUT8→RGBA32 uses a 256-entry palette at PALETTE_BASE (0xF0200). Generic BPP conversion copies min(srcBpp,dstBpp) bytes per pixel and zero-fills any extra destination bytes. No M68K fallback.

**LVO**: -78 (slot 13)

#### IEWarpBlitAlpha

```c
ULONG IEWarpBlitAlpha(APTR src, APTR dst, UWORD width, UWORD height,
                      UWORD srcStride, UWORD dstStride)
                      /* (A0, A1, D0, D1, D2, D3) */
```

Alpha-blended blit using the source alpha channel. No M68K fallback.

**LVO**: -84 (slot 14)

#### IEWarpFillRect

```c
ULONG IEWarpFillRect(APTR dst, UWORD width, UWORD height,
                     UWORD stride, ULONG color)
                     /* (A0, D0, D1, D2, D3) */
```

Fill a rectangular region with `color`. M68K fallback fills whole ULONGs per row then fills remaining tail bytes individually.

**LVO**: -90 (slot 15)

#### IEWarpPixelProcess

```c
ULONG IEWarpPixelProcess(APTR data, UWORD width, UWORD height,
                         UWORD stride, ULONG operation, ULONG param)
                         /* (A0, D0, D1, D2, D3, D4) */
```

Per-pixel processing on RGBA32 data. `operation` selects the sub-operation (POP_BRIGHTEN=2, POP_DARKEN=3, POP_SETALPHA=4, POP_COLOR2GREY=6, POP_NEGATIVE=7) and `param` provides the sub-operation parameter. No M68K fallback.

**LVO**: -96 (slot 16)

### Audio Operations

#### IEWarpAudioMix

```c
ULONG IEWarpAudioMix(APTR channels, UWORD numChannels, APTR dst,
                     ULONG numSamples, APTR volumes)
                     /* (A0, D0, A1, D1, A2) */
```

Mix `numChannels` audio channel buffers into a single output buffer `dst`. `volumes` points to per-channel volume levels. Threshold is `threshold / 4` (sample-count based).

**LVO**: -102 (slot 17)

#### IEWarpAudioResample

```c
ULONG IEWarpAudioResample(APTR src, APTR dst, ULONG srcRate, ULONG dstRate,
                          ULONG numSamples)
                          /* (A0, A1, D0, D1, D2) */
```

Resample audio from `srcRate` Hz to `dstRate` Hz. Rates are packed as `(srcRate & 0xFFFF) | (dstRate << 16)` in `respCap`.

**LVO**: -108 (slot 18)

#### IEWarpAudioDecode

```c
ULONG IEWarpAudioDecode(APTR src, APTR dst, ULONG srcLen, ULONG codec)
                        /* (A0, A1, D0, D1) */
```

Decode compressed audio. `codec` identifies the compression format (1 = IMA-ADPCM). The codec value is passed via `respCap`. IMA-ADPCM decodes 4-bit nibbles to 16-bit PCM samples with quadratic step size adaptation.

**LVO**: -114 (slot 19)

### Math Operations

#### IEWarpFPBatch

```c
ULONG IEWarpFPBatch(APTR descriptors, APTR results, ULONG count)  /* (A0, A1, D0) */
```

Batch `count` IEEE 754 float32 operations in a single dispatch. `descriptors` points to an array of 12-byte `IEWarpFPDesc` structures (opcode, operand A, operand B). `results` points to a separate packed float32 array where the worker writes one result per operation (4 bytes each). The two buffers must not overlap.

**LVO**: -120 (slot 20)

#### IEWarpMatrixMul

```c
ULONG IEWarpMatrixMul(APTR matA, APTR matB, APTR matOut)  /* (A0, A1, A2) */
```

Multiply two 4×4 float32 matrices: `matOut = matA × matB`. Each matrix is 64 bytes (16 floats, row-major). Worker register mapping: reqPtr=matA (r12), respPtr=matB (r14), flags=matOut (r29).

**LVO**: -126 (slot 21)

#### IEWarpCRC32

```c
ULONG IEWarpCRC32(APTR data, ULONG len, ULONG initial, ULONG *result)  /* (A0, D0, D1, A1) */
```

Compute CRC32 of `len` bytes at `data`, starting from `initial` CRC value. Use `0xFFFFFFFF` for standard CRC32. The `initial` parameter is passed via the flags register (r29). The worker writes the computed CRC directly to `*result` via respPtr (r14). Returns a ticket; after `IEWarpWait(ticket)`, `*result` contains the final CRC.

Usage:
```c
ULONG crc;
ULONG ticket = IEWarpCRC32(data, len, 0xFFFFFFFF, &crc);
IEWarpWait(ticket);
/* crc now contains the CRC32 checksum */
```

**LVO**: -132 (slot 22)

#### IEWarpGradientFill

```c
ULONG IEWarpGradientFill(APTR dst, UWORD width, UWORD height, UWORD stride,
                          ULONG startColor, ULONG endColor, UWORD direction)
                          /* (A0, D0, D1, D2, D3, D4, D5) */
```

Fill a rectangular region with a gradient from `startColor` to `endColor`. `direction`: 0 = vertical (per-row interpolation), 1 = horizontal (per-column interpolation). Colors are RGBA32 packed.

**LVO**: -138 (slot 23)

#### IEWarpScroll

```c
ULONG IEWarpScroll(APTR fb, UWORD width, UWORD height, UWORD xMin, UWORD yMin,
                    WORD dx, WORD dy, UWORD stride)
                    /* (A0, D0, D1, D2, D3, D4, D5, D6) */
```

Scroll a rectangular region of a framebuffer by `(dx, dy)` pixels, filling the exposed area with the background color. `fb` is the base of the framebuffer, `width`/`height` define the scroll area, `xMin`/`yMin` the top-left corner, and `stride` the row pitch in bytes.

**LVO**: -234 (slot 39)

#### IEWarpGlyphRender

```c
ULONG IEWarpGlyphRender(APTR descs, ULONG count, APTR dst, ULONG dstStride,
                          APTR alphaData)
                          /* (A0, D0, A1, D1, A2) */
```

Batch-render `count` font glyphs described by an array of 16-byte `GlyphDesc` structures. Each descriptor specifies destination X/Y, source alpha offset, glyph width/height, and foreground color. `dst` is the framebuffer base, `dstStride` is the row pitch, and `alphaData` points to the packed 8-bit alpha maps for all glyphs.

**LVO**: -240 (slot 40)

#### IEWarpAreaFill

```c
ULONG IEWarpAreaFill(APTR vertices, ULONG count, APTR dstBase, ULONG stride)
                      /* (A0, D0, A1, D1) */
```

Fill a polygon defined by `count` vertex pairs (WORD x, WORD y) in the `vertices` array. The polygon is rendered into the framebuffer at `dstBase` with row pitch `stride` using the even-odd fill rule.

**LVO**: -246 (slot 41)

#### IEWarpWorkerStop

```c
VOID IEWarpWorkerStop()  /* () */
```

Stop the IE64 coprocessor worker and update the library's internal `workerRunning` flag. All subsequent iewarp dispatch calls will fall back to M68K until `IEWarpWorkerStart` is called. Use this instead of raw MMIO `IE_COPROC_CMD_STOP` to keep the library in sync.

**LVO**: -252 (slot 42)

#### IEWarpWorkerStart

```c
VOID IEWarpWorkerStart()  /* () */
```

Restart the IE64 coprocessor worker using the service binary path from library init. Programs `IE_COPROC_NAME_PTR` before issuing START and updates the library's `workerRunning` flag on success.

**LVO**: -258 (slot 43)

### Control Operations

#### IEWarpBatchBegin

```c
ULONG IEWarpBatchBegin()  /* () */
```

Enter batch mode. Subsequent operations are queued without individual synchronization. Returns 0.

**LVO**: -144 (slot 24)

#### IEWarpBatchEnd

```c
ULONG IEWarpBatchEnd()  /* () */
```

End batch mode. Returns the last ticket from the batch. The caller can `IEWarpWait()` on this single ticket to wait for all batched operations.

**LVO**: -150 (slot 25)

#### IEWarpWait

```c
ULONG IEWarpWait(ULONG ticket)  /* (D0) */
```

Block until the operation identified by `ticket` completes. Returns a ticket status code:

| Status | Value | Meaning |
|--------|-------|---------|
| `IE_COPROC_ST_PENDING` | 0 | Still pending |
| `IE_COPROC_ST_RUNNING` | 1 | Currently executing |
| `IE_COPROC_ST_OK` | 2 | Completed successfully |
| `IE_COPROC_ST_ERROR` | 3 | Completed with error |
| `IE_COPROC_ST_TIMEOUT` | 4 | Timed out (~5 seconds) |
| `IE_COPROC_ST_WORKER_DOWN` | 5 | IE64 worker not running |

`IEWarpWait(0)` returns `IE_COPROC_ST_OK` immediately (ticket 0 = inline fallback, already complete).

**Wait mechanism**: `IEWarpWait` allocates a per-task signal bit and a waiter slot (up to `IEWARP_MAX_WAITERS` = 8 concurrent waiters). It opens `timer.device` and starts a 5-second one-shot timer, then enters an interrupt-driven POLL + `Wait(completionSig | timerSig)` loop: issue `COPROC_CMD_POLL` to check ticket status, and if not yet complete, call `Wait()` to yield the CPU until either the completion interrupt or the timer fires. If the timer expires first (completion IRQ missed or worker hung), `IEWarpWait` falls back to `COPROC_CMD_WAIT` which has a real Go-side timeout guarantee. This ensures `IEWarpWait` never blocks indefinitely. If signal bits, waiter slots, or `timer.device` are unavailable, it falls back to `COPROC_CMD_WAIT` immediately.

**LVO**: -156 (slot 26)

#### IEWarpPoll

```c
ULONG IEWarpPoll(ULONG ticket)  /* (D0) */
```

Non-blocking check of ticket status. Returns the same status codes as `IEWarpWait`. Does not block.

**LVO**: -162 (slot 27)

#### IEWarpGetThreshold

```c
ULONG IEWarpGetThreshold()  /* () */
```

Returns the current adaptive threshold in bytes. Operations smaller than this are handled by M68K fallback code.

**LVO**: -168 (slot 28)

#### IEWarpGetStats

```c
ULONG IEWarpGetStats(APTR statsBuf)  /* (A0) */
```

Fills an `IEWarpStats` structure at `statsBuf`. Returns 0.

```c
struct IEWarpStats
{
    ULONG ops;              /* Total operations dispatched */
    ULONG bytes;            /* Total bytes processed */
    ULONG overheadNs;       /* Dispatch overhead in nanoseconds */
    ULONG completedTicket;  /* Last completed ticket */
    ULONG threshold;        /* Current adaptive threshold in bytes */
    ULONG ringDepth;        /* IE64 ring buffer occupancy (0-16) */
    ULONG ringHighWater;    /* Peak ring occupancy since last reset */
    ULONG uptimeSecs;       /* Seconds since IE64 worker started */
    ULONG irqEnabled;       /* Completion IRQ enabled flag */
    ULONG workerState;      /* Bitmask of running workers */
    ULONG busyPct;          /* Worker busy % over last 1 second (0-100) */
};
```

Fields `ops` and `bytes` are read from MMIO registers `COPROC_STATS_OPS` (0xF2378) and `COPROC_STATS_BYTES` (0xF237C). `overheadNs` comes from `COPROC_DISPATCH_OVERHEAD` (0xF2384). `completedTicket` from `COPROC_COMPLETED_TICKET` (0xF2388). `threshold` is the library's local calibrated value. `ringDepth`, `uptimeSecs`, and `busyPct` come from the extended monitor registers at `0xF23B0-0xF23BC`. `ringHighWater` is tracked library-side. `irqEnabled` and `workerState` come from `COPROC_IRQ_CTRL` and `COPROC_WORKER_STATE`.

**LVO**: -174 (slot 29)

### Monitoring and Diagnostics

The following functions support the IEWarpMon coprocessor monitor application. See `sdk/docs/iewarpmon.md` for the full user guide.

#### IEWarpSetCaller

```c
VOID IEWarpSetCaller(ULONG callerID)  /* (D0) */
```

Tag the current task with a caller ID for per-consumer attribution. Call immediately before each iewarp dispatch. Caller IDs are defined as `IEWARP_CALLER_*` constants (EXEC=1, GRAPHICS=2, IEGFX=3, CGFX=4, AHI=5, ICON=6, MATH=7, MUI=8, FREETYPE=9, DATATYPES=10, APP=11). Allocates a task slot (up to 32) if not already present.

**LVO**: -180 (slot 30)

#### IEWarpGetOpStats

```c
ULONG IEWarpGetOpStats(APTR buf, ULONG maxOps)  /* (A0, D0) */
```

Copy per-operation counters into `buf` (array of `IEWarpOpCounter` structs). Returns `maxOps`. All entries are copied preserving index — `buf[i]` corresponds to operation code `i`. Entries with zero accel and fallback counts are unused ops.

```c
struct IEWarpOpCounter
{
    ULONG accelCount;       /* Dispatched to IE64 */
    ULONG accelBytes;
    ULONG fallbackCount;    /* Fell back to M68K */
    ULONG fallbackBytes;
};
```

**LVO**: -186 (slot 31)

#### IEWarpGetCallerStats

```c
ULONG IEWarpGetCallerStats(APTR buf, ULONG maxEntries)  /* (A0, D0) */
```

Copy per-caller (per-consumer-library) counters into `buf` (array of `IEWarpCallerEntry` structs). Returns the number of active entries.

```c
struct IEWarpCallerEntry
{
    ULONG callerID;         /* IEWARP_CALLER_* */
    ULONG ops;
    ULONG bytes;
};
```

**LVO**: -192 (slot 32)

#### IEWarpGetTaskStats

```c
ULONG IEWarpGetTaskStats(APTR buf, ULONG maxEntries)  /* (A0, D0) */
```

Copy per-task counters into `buf` (array of `IEWarpTaskEntry` structs). Returns the number of active entries. Slot 0 is "(overflow)" for tasks that couldn't be allocated a dedicated slot. Dead tasks are not automatically reclaimed — use `IEWarpResetAllStats()` to clear.

```c
struct IEWarpTaskEntry
{
    struct Task *task;      /* NULL = free slot */
    char         name[32];
    ULONG        ops;
    ULONG        bytes;
};
```

**LVO**: -198 (slot 33)

#### IEWarpGetErrorStats

```c
ULONG IEWarpGetErrorStats(APTR buf)  /* (A0) */
```

Copy error counters into `buf` (an `IEWarpErrorStats` struct). Returns 0.

```c
struct IEWarpErrorStats
{
    ULONG queueFull;
    ULONG workerDown;
    ULONG staleTicket;
    ULONG enqueueFail;
};
```

**LVO**: -204 (slot 34)

#### IEWarpGetBatchStats

```c
ULONG IEWarpGetBatchStats(APTR buf)  /* (A0) */
```

Copy batch statistics into `buf` (an `IEWarpBatchStats` struct). Returns 0.

```c
struct IEWarpBatchStats
{
    ULONG batchCount;       /* Completed BatchBegin/BatchEnd brackets */
    ULONG totalBatchedOps;  /* Total ops inside all batches */
    ULONG maxBatchSize;     /* Largest single batch */
    ULONG currentBatchOps;  /* Ops in current open batch (0 if not batching) */
};
```

**LVO**: -210 (slot 35)

#### IEWarpGetWaiterInfo

```c
ULONG IEWarpGetWaiterInfo(APTR buf, ULONG maxEntries)  /* (A0, D0) */
```

Copy active waiter slot information into `buf` (array of `IEWarpWaiterInfo` structs). Returns the number of active waiters (tasks currently blocked in `IEWarpWait`).

```c
struct IEWarpWaiterInfo
{
    char   taskName[32];
    ULONG  ticket;
};
```

**LVO**: -216 (slot 36)

#### IEWarpSetThreshold

```c
VOID IEWarpSetThreshold(ULONG threshold)  /* (D0) */
```

Override the adaptive threshold. Operations smaller than `threshold` bytes are handled by M68K fallback code. Use in conjunction with the Operations tab in IEWarpMon to tune the accel/fallback balance interactively.

**LVO**: -222 (slot 37)

#### IEWarpResetAllStats

```c
VOID IEWarpResetAllStats()  /* () */
```

Clear all statistics on both AROS and Go sides. Resets per-op counters, per-task slots, per-caller stats, error counters, batch stats, ring high-water mark, and Go-side global counters + busy buckets. Reset is best-effort, not epoch-atomic — transient mixed-epoch values may appear for one 250ms update cycle. Task slots (except overflow slot 0) are freed.

**LVO**: -228 (slot 38)

## Operation Codes

Operation codes are defined in `<libraries/iewarp.h>` and must match the IE64 worker service.

| Code | Name | Category | Description |
|------|------|----------|-------------|
| 0 | `WARP_OP_NOP` | Control | No operation (used for calibration) |
| 1 | `WARP_OP_MEMCPY` | Memory | Copy bytes (non-overlapping) |
| 2 | `WARP_OP_MEMCPY_QUICK` | Memory | Copy ULONG-aligned data |
| 3 | `WARP_OP_MEMSET` | Memory | Fill memory with byte value |
| 4 | `WARP_OP_MEMMOVE` | Memory | Copy with overlap handling |
| 10 | `WARP_OP_BLIT_COPY` | Graphics | Rectangular blit copy |
| 11 | `WARP_OP_BLIT_MASK` | Graphics | Masked blit |
| 12 | `WARP_OP_BLIT_SCALE` | Graphics | Scaled blit |
| 13 | `WARP_OP_BLIT_CONVERT` | Graphics | Pixel format conversion |
| 14 | `WARP_OP_BLIT_ALPHA` | Graphics | Alpha-blended blit |
| 15 | `WARP_OP_FILL_RECT` | Graphics | Rectangular fill |
| 16 | `WARP_OP_AREA_FILL` | Graphics | Polygon area fill |
| 17 | `WARP_OP_PIXEL_PROCESS` | Graphics | Per-pixel processing |
| 20 | `WARP_OP_AUDIO_MIX` | Audio | Multi-channel audio mixing |
| 21 | `WARP_OP_AUDIO_RESAMPLE` | Audio | Sample rate conversion |
| 22 | `WARP_OP_AUDIO_DECODE` | Audio | Compressed audio decode |
| 30 | `WARP_OP_FP_BATCH` | Math | Batched floating-point operations |
| 31 | `WARP_OP_MATRIX_MUL` | Math | Matrix multiplication |
| 32 | `WARP_OP_CRC32` | Math | CRC32 checksum |
| 40 | `WARP_OP_GRADIENT_FILL` | Rendering | Gradient fill |
| 41 | `WARP_OP_GLYPH_RENDER` | Rendering | Font glyph rendering |
| 42 | `WARP_OP_SCROLL` | Rendering | Scroll/shift pixel data |

## Batch Operations

`IEWarpBatchBegin()` / `IEWarpBatchEnd()` bracket a group of operations. During batch mode, each dispatched operation records its ticket internally, but the caller does not need to wait on each one individually. `IEWarpBatchEnd()` returns the last ticket; waiting on it guarantees all prior batched operations have completed (ring buffer is FIFO).

Example: ScrollRaster acceleration

```c
#include <proto/iewarp.h>

/* Scroll a region up by 16 pixels */
void FastScrollUp(APTR fb, UWORD width, UWORD height, UWORD stride)
{
    ULONG ticket;

    IEWarpBatchBegin();

    /* Copy rows 16..height-1 up to rows 0..height-17 */
    IEWarpBlitCopy(fb + 16 * stride,   /* src: row 16 */
                   fb,                  /* dst: row 0 */
                   width, height - 16,
                   stride, stride,
                   0xC0);              /* minterm: copy */

    /* Clear the newly exposed rows at the bottom */
    IEWarpFillRect(fb + (height - 16) * stride,
                   width, 16, stride, 0x00000000);

    ticket = IEWarpBatchEnd();
    IEWarpWait(ticket);
}
```

## Completion Interrupt

When the IE64 worker completes an operation, the Go-side coprocessor manager detects completion and calls `AssertInterrupt(6)`, which triggers an M68K Level 6 interrupt. The AROS interrupt server chain invokes the iewarp.library handler (`coprocCompletionHandler`), which iterates the waiter table (up to `IEWARP_MAX_WAITERS` = 8 slots) and calls `Signal()` on each waiting task using its allocated signal bit.

`IEWarpWait()` allocates a per-task signal bit via `AllocSignal(-1)`, claims a waiter slot in `IEWarpBase->waiters[]`, and opens `timer.device` to start a 5-second one-shot timer. It then enters an interrupt-driven loop: issue `COPROC_CMD_POLL` to check the ticket status, and if not yet complete, call `Wait(completionSig | timerSig)` to yield the M68K CPU to the AROS scheduler. When the completion interrupt fires, the handler signals all waiting tasks, which wake up and re-poll. If the timer expires first (IRQ missed or worker hung), the function cancels the interrupt-driven path and falls back to `COPROC_CMD_WAIT`, which has a real Go-side timeout guarantee. This ensures `IEWarpWait` never blocks indefinitely. On exit, the timer is cancelled, `timer.device` is closed, the waiter slot is freed, and the signal bit released via `FreeSignal()`.

If signal bits, waiter slots, or `timer.device` are unavailable, `IEWarpWait` falls back to `COPROC_CMD_WAIT` immediately. This fallback path is functional but not multitasking-friendly (blocks the M68K CPU in the Go-side coprocessor manager).

This design supports up to 8 tasks waiting on coprocessor results concurrently without busy-waiting.

## Adaptive Threshold

On library initialization, the library dispatches a `WARP_OP_NOP` and measures the round-trip time through the coprocessor MMIO path. The calibrated overhead in nanoseconds is read from `COPROC_DISPATCH_OVERHEAD` (0xF2384).

The threshold formula:

```
threshold = overhead_ns * 28 / 1000
```

The constant 28 reflects that the M68K CPU copies approximately 28 bytes per microsecond at 7 MHz. Operations smaller than this threshold are faster to execute inline on the M68K than to dispatch through the coprocessor.

The threshold is rounded up to the next power of 2, with a minimum of 64 bytes. The default before calibration is 1024 bytes.

`IEWarpGetThreshold()` returns the current calibrated value.

## Statistics

`IEWarpGetStats()` fills a 20-byte `IEWarpStats` structure by reading MMIO registers:

| Field | Source | MMIO Address |
|-------|--------|-------------|
| `ops` | `COPROC_STATS_OPS` | 0xF2378 |
| `bytes` | `COPROC_STATS_BYTES` | 0xF237C |
| `overheadNs` | `COPROC_DISPATCH_OVERHEAD` | 0xF2384 |
| `completedTicket` | `COPROC_COMPLETED_TICKET` | 0xF2388 |
| `threshold` | `IEWarpBase->threshold` | (library-local) |

## Fallback Behavior

Every operation checks two conditions before dispatching to the coprocessor:

1. **Worker running**: `IEWarpBase->workerRunning` must be TRUE
2. **Size threshold**: The data size must meet or exceed `IEWarpBase->threshold`

When either condition fails, the function executes inline M68K fallback code and returns ticket 0. `IEWarpWait(0)` returns `IE_COPROC_ST_OK` immediately.

Functions with no simple M68K equivalent (BlitMask, BlitScale, BlitConvert, BlitAlpha, PixelProcess) return 0 without performing the operation when falling back. The caller is expected to handle this (e.g. the AROS HIDD falls through to its own software path).

Audio operations use `threshold / 4` as the comparison value since their size parameter is a sample count rather than a byte count.

## Accelerated AROS APIs

The following AROS subsystems issue raw coprocessor MMIO dispatch (`ie_write32(IE_COPROC_*)`) to offload work to the IE64 worker. These consumers write coprocessor registers directly rather than calling iewarp.library entry points. The iewarp.library API exists for future consumers and application-level use.

All 22 worker operations are implemented in the IE64 worker service. All 22 have AROS-integrated consumers. 17 are consumed by AROS kernel/workbench subsystems that dispatch directly via MMIO; the remaining 5 (FP_BATCH, MATRIX_MUL, CRC32, AUDIO_MIX, GRADIENT_FILL) are consumed through iewarp.library API functions callable by any AROS application. Consumer-side code uses `#ifdef __mc68000__` guards in shared workbench source files or arch-specific overrides under `arch/m68k-ie/`.

### Integrated Consumers

These subsystems have consumer-side dispatch code that matches the IE64 worker ABI. Operations complete on the IE64 coprocessor.

| Subsystem | Functions | Warp Operations | Notes |
|-----------|-----------|-----------------|-------|
| exec.library | CopyMem, CopyMemQuick | MEMCPY, MEMMOVE, MEMCPY_QUICK | Arch-specific override; CopyMem uses MEMMOVE (overlap-safe) or MEMCPY depending on overlap detection |
| IEGfx HIDD | FillRect, Clear, CopyBox, CopyBoxMasked, PutImage, PutAlphaTemplate, BitMap::New | FILL_RECT, BLIT_COPY, BLIT_MASK, BLIT_CONVERT, BLIT_ALPHA, MEMSET | CopyBoxMasked expands 1-bit PLANEPTR to 8-bit byte mask; PutImage converts ARGB32/BGRA32/ABGR32/RGB24/BGR24→RGBA32; PutAlphaTemplate uses source-over compositing with FG color in r29; BitMap::New uses MEMSET for large framebuffer clears |
| graphics.library | ScrollRaster, AreaEnd, BitMapScale, Text | SCROLL, AREA_FILL, BLIT_SCALE, GLYPH_RENDER | Arch-specific overrides; ScrollRaster uses batch (copy+fill); Text batch-renders ≥4 chars with 1-bit→8-bit glyph expansion and 16-byte descriptors with per-glyph FG color |
| cybergraphics.library | ProcessPixelArray | PIXEL_PROCESS | `#ifdef __mc68000__` guard in processpixelarray.c |
| icon.library | ScaleRect | BLIT_SCALE | `#ifdef __mc68000__` guard in layouticon.c |
| MUI/Zune | TrueDitherV, TrueDitherH | GRADIENT_FILL | `#ifdef __mc68000__` guard in imspec_gradientdraw.c; direction flag in respCap bit 16 (0=vertical, 1=horizontal) |
| WAV datatype | IMA-ADPCM decode | AUDIO_DECODE | `#ifdef __mc68000__` guard in decoders.c |
| AHI ie-audio driver | Sample rate conversion | AUDIO_RESAMPLE | Resamples mixer output when AHI mix rate differs from DAC rate (44100Hz); deinterleaves stereo, dispatches per-channel, re-interleaves |
| iewarp.library | IEWarpAudioMix | AUDIO_MIX | Public API for N-channel mixing with per-channel volume; called by applications needing custom audio mixing |
| iewarp.library | IEWarpFPBatch | FP_BATCH | Public API for batching N IEEE FP operations in a single dispatch; 14 sub-ops via native IE64 FPU |
| iewarp.library | IEWarpMatrixMul | MATRIX_MUL | Public API for 4×4 float32 matrix multiply; matB in respPtr (r14), output in flags (r29) |
| iewarp.library | IEWarpCRC32 | CRC32 | Public API for CRC32 computation on memory buffers; initial CRC in flags (r29) |
| iewarp.library | IEWarpGradientFill | GRADIENT_FILL | Public API for gradient rendering; wraps MMIO dispatch with direction flag |
| iewarp.library init | Calibration NOP | NOP | Dispatched during library init to measure coprocessor round-trip overhead for adaptive threshold |

### Consumer Architecture Notes

All 22 worker operations have AROS-integrated consumers. The consumer tiers are:

**Tier 1 — Kernel/workbench subsystem dispatch** (17 ops): AROS kernel libraries and HIDD classes dispatch directly via coprocessor MMIO registers. These are transparent to applications — e.g., calling `CopyMem()` or `BltBitMap()` automatically uses the IE64 worker for large operations.

**Tier 2 — iewarp.library API** (5 ops): Consumed through public iewarp.library functions that any AROS application can call. These ops have no natural kernel-level consumer but are fully integrated via the library API:

| Operation | Library Function | Notes |
|-----------|-----------------|-------|
| AUDIO_MIX | `IEWarpAudioMix()` | N-channel mixing with per-channel volume scaling. AHI's internal mixer is architecturally coupled (sample cursors, interpolation, EOS callbacks), so the library function serves applications needing custom mixing |
| FP_BATCH | `IEWarpFPBatch()` | Batch N IEEE FP operations in one dispatch. Math library APIs are synchronous single-op (`IEEESPMul(a,b)` returns immediately), so FP_BATCH serves application-level batch callers (matrix computations, signal processing, physics) |
| MATRIX_MUL | `IEWarpMatrixMul()` | 4×4 float32 matrix multiply for 3D transforms, affine geometry, etc. |
| CRC32 | `IEWarpCRC32()` | CRC32 computation on memory buffers; result written to caller-provided ULONG via respPtr |
| GRADIENT_FILL | `IEWarpGradientFill()` | Gradient rendering with direction control. Also consumed directly by MUI/Zune TrueDitherV/H |

## IE64 Worker Service

The IE64 worker binary is assembled from `sdk/examples/asm/iewarp_service.asm` and loaded as `iewarp_service.ie64`.

The worker runs a simple poll loop on its assigned ring buffer (ring index 5, base address `MAILBOX_BASE + 5 * 0x300 = 0x820F00`). It spins comparing head vs tail bytes. When head != tail, it reads the request descriptor at the tail slot, dispatches the operation, writes the response descriptor, and advances the tail pointer.

The worker implements all 22 operations:

| Operation | Description |
|-----------|-------------|
| NOP | No-op for calibration round-trips |
| MEMCPY | Byte copy with ULONG-aligned fast path |
| MEMCPY_QUICK | Copy assuming ULONG-aligned src/dst |
| MEMSET | Fill memory with a byte value |
| MEMMOVE | Overlap-safe copy (backward copy when dst > src) |
| BLIT_COPY | Row-by-row rectangular blit copy |
| BLIT_MASK | Byte-per-pixel masked blit (skip where mask=0) |
| BLIT_SCALE | DDA nearest-neighbor scaling via `mulu.l`/`divu.l` |
| BLIT_CONVERT | Pixel format conversion: format-aware (ARGB32/BGRA32/ABGR32/RGB24/BGR24→RGBA32) + legacy BPP copy |
| BLIT_ALPHA | Source-over alpha compositing: `dst = fg * alpha/255 + dst * (255-alpha)/255`; FG color in r29 (RGBA32 packed) |
| FILL_RECT | Rectangular fill with LONG-aligned fast path and byte fallback |
| AREA_FILL | Scanline polygon fill with edge intersection, insertion sort, even-odd spans |
| PIXEL_PROCESS | Sub-operations: brighten, darken, set-alpha, greyscale (ITU-R BT.601), negative |
| AUDIO_MIX | N-channel mix with per-channel volume (0-256), int16 clamping |
| AUDIO_RESAMPLE | Linear interpolation with fractional sample position |
| AUDIO_DECODE | IMA-ADPCM 4-bit decoder with quadratic step approximation |
| FP_BATCH | 14 FP sub-ops via native IE64 FPU (add, sub, mul, div, sqrt, sin, cos, tan, atan, log, exp, pow, abs, neg); 12-byte descriptors |
| MATRIX_MUL | 4×4 float32 matrix multiply using FPU triple-nested loop |
| CRC32 | Bit-by-bit with polynomial 0xEDB88320 |
| GRADIENT_FILL | Vertical/horizontal gradient with per-channel interpolation via `muls.l`/`divs.l`; direction in respCap bit 16 |
| GLYPH_RENDER | Batch glyph compositing from 16-byte descriptors (dstX, dstY, srcOffset, width, height, fgColor) with source-over alpha compositing per glyph |
| SCROLL | Overlap-safe copy with exposed area fill; handles both dx and dy |

### Operation Parameter Formats

#### FP_BATCH Descriptors (12 bytes each)

The request buffer at `reqPtr` contains N packed descriptors. `reqLen` = N (operation count). Each descriptor:

| Offset | Size | Field |
|--------|------|-------|
| +0x00 | 4 | sub-op (0=add, 1=sub, 2=mul, 3=div, 4=sqrt, 5=sin, 6=cos, 7=tan, 8=atan, 9=log, 10=exp, 11=pow, 12=abs, 13=neg) |
| +0x04 | 4 | operand A (IEEE 754 float32) |
| +0x08 | 4 | operand B (IEEE 754 float32, unused for unary ops) |

Results are written to a separate packed float32 array at `respPtr` (4 bytes per result), NOT back into the descriptor buffer.

#### GLYPH_RENDER Descriptors (16 bytes each)

The request buffer at `reqPtr` contains N glyph descriptors. `reqLen` = N (glyph count). Source bitmap data starts at the pointer in the `flags` field. Each descriptor:

| Offset | Size | Field |
|--------|------|-------|
| +0x00 | 4 | packed position: `dstX \| (dstY << 16)` |
| +0x04 | 4 | srcOffset (byte offset from source data base) |
| +0x08 | 4 | packed dimensions: `width \| (height << 16)` |
| +0x0C | 4 | fgColor (RGBA32 packed foreground color for source-over compositing) |

Destination is at `respPtr` with stride in `respCap`. Source data base pointer is in the `flags` field. Each glyph uses source-over alpha compositing with its own foreground color.

#### PIXEL_PROCESS Sub-Operations

| Value | Name | Effect |
|-------|------|--------|
| 0 | POP_TINT | Tint pixels (reserved) |
| 2 | POP_BRIGHTEN | Add `param` to each RGB channel, clamp to 255 |
| 3 | POP_DARKEN | Subtract `param` from each RGB channel, clamp to 0 |
| 4 | POP_SETALPHA | Set alpha channel to `param` for all pixels |
| 6 | POP_COLOR2GREY | Convert to greyscale using ITU-R BT.601 (R×77 + G×150 + B×29) >> 8 |
| 7 | POP_NEGATIVE | Invert RGB channels (255 - value), preserve alpha |

#### GRADIENT_FILL Parameters

| Register | Field | Meaning |
|----------|-------|---------|
| reqPtr | startColor | Start color (RGBA32: R bits 0-7, G 8-15, B 16-23, A 24-31) |
| reqLen | dimensions | `width \| (height << 16)` |
| respPtr | dst | Destination buffer pointer |
| respCap | stride+direction | `stride \| (direction << 16)` — stride in low 16 bits (bytes per row), direction in bit 16: 0=vertical (interpolate per row), 1=horizontal (interpolate per column) |
| flags | endColor | End color (same RGBA32 packing as startColor) |

Vertical gradient: each row is a solid color interpolated between start and end.
Horizontal gradient: each column is a solid color interpolated between start and end.

#### AUDIO_DECODE Codec IDs

| Value | Name | Description |
|-------|------|-------------|
| 1 | CODEC_IMA_ADPCM | IMA/DVI ADPCM 4-bit codec |

### Ring Buffer Layout

Each ring has 768 bytes (`RING_STRIDE = 0x300`):

| Offset | Size | Field |
|--------|------|-------|
| +0x00 | 1 byte | head (producer write index) |
| +0x01 | 1 byte | tail (consumer read index) |
| +0x02 | 1 byte | capacity (16) |
| +0x08 | 512 bytes | 16 request descriptors (32 bytes each) |
| +0x208 | 256 bytes | 16 response descriptors (16 bytes each) |

### Request Descriptor (32 bytes)

| Offset | Size | Field |
|--------|------|-------|
| +0x00 | 4 | ticket |
| +0x04 | 4 | cpuType |
| +0x08 | 4 | op |
| +0x0C | 4 | flags |
| +0x10 | 4 | reqPtr |
| +0x14 | 4 | reqLen |
| +0x18 | 4 | respPtr |
| +0x1C | 4 | respCap |

### Response Descriptor (16 bytes)

| Offset | Size | Field |
|--------|------|-------|
| +0x00 | 4 | ticket |
| +0x04 | 4 | status (0=pending, 2=OK, 3=error) |
| +0x08 | 4 | resultCode |
| +0x0C | 4 | respLen |

## Memory Map

| Region | Address Range | Size |
|--------|--------------|------|
| IE64 Worker code/data | 0x3A0000 - 0x41FFFF | 512 KB |
| Mailbox (6 CPU rings) | 0x790000 - 0x7917FF | 6 KB |
| IE64 ring (index 5) | 0x790F00 - 0x7911FF | 768 bytes |
| Coprocessor MMIO | 0xF2340 - 0xF238F | 80 bytes |
| Coprocessor Extended | 0xF23B0 - 0xF23BF | 16 bytes |

### Coprocessor MMIO Register Map

| Address | Name | Access | Description |
|---------|------|--------|-------------|
| 0xF2340 | COPROC_CMD | W | Command (triggers action) |
| 0xF2344 | COPROC_CPU_TYPE | R/W | Target CPU type |
| 0xF2348 | COPROC_CMD_STATUS | R | 0 = OK, 1 = error |
| 0xF234C | COPROC_CMD_ERROR | R | Error code |
| 0xF2350 | COPROC_TICKET | R/W | Ticket ID |
| 0xF2354 | COPROC_TICKET_STATUS | R | Per-ticket status |
| 0xF2358 | COPROC_OP | W | Operation code |
| 0xF235C | COPROC_REQ_PTR | W | Request data pointer |
| 0xF2360 | COPROC_REQ_LEN | W | Request data length |
| 0xF2364 | COPROC_RESP_PTR | W | Response buffer pointer |
| 0xF2368 | COPROC_RESP_CAP | W | Response buffer capacity |
| 0xF236C | COPROC_TIMEOUT | W | Timeout in ms |
| 0xF2370 | COPROC_NAME_PTR | W | Service filename pointer |
| 0xF2374 | COPROC_WORKER_STATE | R | Bitmask of running workers |
| 0xF2378 | COPROC_STATS_OPS | R | Total ops dispatched |
| 0xF237C | COPROC_STATS_BYTES | R | Total bytes processed |
| 0xF2380 | COPROC_IRQ_CTRL | R/W | IRQ enable (bit 0) |
| 0xF2384 | COPROC_DISPATCH_OVERHEAD | R | Calibrated overhead (ns) |
| 0xF2388 | COPROC_COMPLETED_TICKET | R | Last completed ticket |

#### Extended Monitor Registers (0xF23B0 - 0xF23BF)

| Address | Name | Access | Description |
|---------|------|--------|-------------|
| 0xF23B0 | COPROC_RING_DEPTH | R | IE64 ring buffer occupancy (0-16) |
| 0xF23B4 | COPROC_WORKER_UPTIME | R | Seconds since IE64 worker started |
| 0xF23B8 | COPROC_STATS_RESET | W | Write 1 to zero Go-side stats + busy buckets |
| 0xF23BC | COPROC_BUSY_PCT | R | Worker busy % (rolling 1s, 10x100ms buckets) |

### Coprocessor Commands

| Value | Name | Description |
|-------|------|-------------|
| 1 | COPROC_CMD_START | Start worker from file (NAME_PTR + CPU_TYPE) |
| 2 | COPROC_CMD_STOP | Stop worker |
| 3 | COPROC_CMD_ENQUEUE | Submit request, returns ticket in COPROC_TICKET |
| 4 | COPROC_CMD_POLL | Check ticket status (non-blocking) |
| 5 | COPROC_CMD_WAIT | Block until ticket completes or timeout |

### Coprocessor Error Codes

| Value | Name | Description |
|-------|------|-------------|
| 0 | COPROC_ERR_NONE | No error |
| 1 | COPROC_ERR_INVALID_CPU | Invalid CPU type |
| 2 | COPROC_ERR_NOT_FOUND | Service file not found |
| 3 | COPROC_ERR_PATH_INVALID | Invalid path |
| 4 | COPROC_ERR_LOAD_FAILED | Failed to load service binary |
| 5 | COPROC_ERR_QUEUE_FULL | Ring buffer full |
| 6 | COPROC_ERR_NO_WORKER | No worker running for CPU type |
| 7 | COPROC_ERR_STALE_TICKET | Ticket has been evicted |

## Source Files

| File | Description |
|------|-------------|
| `AROS/arch/m68k-ie/include/libraries/iewarp.h` | Public header (op codes, IEWarpStats struct) |
| `AROS/arch/m68k-ie/libs/iewarp/iewarp.conf` | AROS library function table |
| `AROS/arch/m68k-ie/libs/iewarp/iewarp_intern.h` | Internal structures (IEWarpBase, dispatch prototype) |
| `AROS/arch/m68k-ie/libs/iewarp/iewarp_init.c` | Library init/expunge, calibration, IRQ handler |
| `AROS/arch/m68k-ie/libs/iewarp/iewarp_dispatch.c` | Core dispatch, Wait/Poll/Batch/Threshold/Stats |
| `AROS/arch/m68k-ie/libs/iewarp/iewarp_ops.c` | Per-operation wrappers with threshold fallback |
| `sdk/examples/asm/iewarp_service.asm` | IE64 worker source (poll loop + op handlers) |
| `coprocessor_constants.go` | Go-side MMIO addresses, ring layout, worker regions |
| `AROS/arch/m68k-ie/include/ie_hwreg.h` | C MMIO register definitions |
| `AROS/arch/m68k-ie/utilities/IEWarpMon/` | IEWarpMon coprocessor monitor (MUI app) |

## Endianness

No byte-swapping is required anywhere in the iewarp pipeline. Each data path is natively little-endian or byte-order agnostic:

- **MMIO register writes**: The M68K I/O path for addresses in the range `0xF0000`-`0x100000` passes values through without byte-swap. When the M68K CPU writes a ULONG to `COPROC_OP` (0xF2358), the Go-side MMIO handler receives the value directly. The coprocessor registers are defined as host-native integers, not byte buffers.

- **Ring buffer (0x790000)**: Ring buffer descriptors are written by the Go-side coprocessor manager in little-endian format. The IE64 CPU reads memory in native LE order. Since both sides agree on LE, request and response descriptors are read correctly without conversion.

- **Bulk data at reqPtr/respPtr**: Memory operations (memcpy, memset, memmove) treat data as opaque bytes. The IE64 worker copies bytes from source to destination without interpreting them, so byte order is irrelevant. VRAM pixel data is stored in LE format because the IEGfx HIDD works in bus-native LE throughout (RGBA32 pixels are written as `uint32` via the blitter, which is LE on the bus).

- **Structured shared data**: Some operations pass packed descriptors through shared memory (FP_BATCH uses 12-byte `IEWarpFPDesc` structs, GLYPH_RENDER uses 16-byte glyph descriptors). These are laid out as sequences of ULONG-sized fields (u32 opcode, f32 operands, u32 packed coordinates). Both M68K and IE64 read ULONGs from bus memory in the same LE byte order, so these shared layouts are read identically by both CPUs without byte-swapping.

## Consumer Guide

To integrate iewarp acceleration into an AROS library or HIDD, follow this pattern.

### Dispatch Pattern

Every accelerated function follows the same two-step check: verify the worker is running and the data size meets the adaptive threshold. If either check fails, fall back to the existing M68K implementation.

```c
#include <ie_hwreg.h>
#include <libraries/iewarp.h>
#include <proto/iewarp.h>

void MyAcceleratedCopy(APTR dst, APTR src, ULONG len)
{
    if (IEWarpBase->workerRunning && len >= IEWarpBase->threshold)
    {
        ULONG ticket = IEWarpMemCpy(dst, src, len);
        IEWarpWait(ticket);
    }
    else
    {
        /* M68K fallback */
        CopyMem(src, dst, len);
    }
}
```

For functions without a simple M68K equivalent (e.g. alpha blending, pixel format conversion), check the return ticket. A ticket of 0 means the operation was not performed and the caller must use its own software path.

### Architecture-Specific Override

AROS uses arch-specific source trees to override generic implementations. To accelerate a subsystem for the IE platform:

1. Create files under `arch/m68k-ie/<subsystem>/` that provide the accelerated versions.
2. Use `%build_archspecific` in the mmakefile.src so the build system picks the arch-specific source over the generic one.
3. Include `ie_hwreg.h` for MMIO register addresses and `libraries/iewarp.h` for operation codes and structures.

Example `mmakefile.src` for an arch-specific override:

```makefile
include $(SRCDIR)/config/aros.cfg

FILES := mysubsystem_accel

USER_INCLUDES := -I$(AROS_INCLUDES)
USER_CFLAGS := -D__NOLIBBASE__

%build_archspecific \
    mainmmake=workbench-libs-mysubsystem \
    maindir=Libs \
    arch=ie-m68k \
    files=$(FILES)
```

### Opening the Library

iewarp.library is a standard AROS shared library. Open it during your library or HIDD init:

```c
struct Library *IEWarpBase = NULL;

BOOL InitAccel(void)
{
    IEWarpBase = OpenLibrary("iewarp.library", 1);
    return (IEWarpBase != NULL);
}
```

If `OpenLibrary` returns NULL, the IE64 coprocessor is not available. All code paths must handle this gracefully by falling through to M68K implementations.

## Performance

### Expected Speedups

The IE64 coprocessor JIT-compiles worker code to native amd64 or arm64 instructions on the host. This eliminates the interpretive overhead of M68K emulation for compute-intensive operations.

- **Memory operations**: Large copies and fills see approximately 100x speedup compared to an M68K byte-copy loop. The IE64 JIT emits native `memcpy`/`memset` equivalents that use the host CPU's full memory bandwidth (wide registers, cache-line-aligned stores). A 64KB `CopyMem` that takes ~9ms on emulated M68K completes in ~90us via iewarp.

- **Graphics operations**: Pixel format conversion (CLUT8 to RGBA32), alpha blending, and bitmap scaling benefit substantially. These operations involve per-pixel arithmetic that the M68K performs with 32-bit multiply-accumulate sequences. The IE64 JIT uses native SIMD-width operations where possible.

- **Audio mixing**: Multi-channel audio mixing replaces per-sample M68K fixed-point multiply-accumulate with native floating-point or integer MAC instructions. Mixing 8 channels of 1024 samples drops from ~2ms (M68K) to ~20us (IE64 JIT).

- **Glyph rendering and gradient fills**: These operations involve inner loops with conditional branches and arithmetic that benefit from branch prediction and out-of-order execution on the host CPU, neither of which exists in the M68K interpreter.

### Dispatch Overhead

The coprocessor dispatch path (M68K MMIO write, Go handler, ring buffer enqueue, IE64 worker pickup) has a typical overhead of approximately 50us, measured by the NOP calibration round-trip during library init. This overhead is constant regardless of operation size.

### Break-Even Threshold

Operations below approximately 1KB of data are faster to execute inline on the emulated M68K than to dispatch through the coprocessor. The adaptive threshold formula accounts for this:

```
threshold = overhead_ns * 28 / 1000
```

At 50us overhead, the threshold is approximately 1400 bytes (rounded up to 2048 as the next power of 2). The M68K copies about 28 bytes per microsecond at 7 MHz equivalent speed, so operations below the threshold complete on M68K before the dispatch round-trip would even finish.

### Pipelining

The ring buffer holds 16 slots, allowing up to 16 operations to be queued without the M68K blocking. In batch mode (`IEWarpBatchBegin` / `IEWarpBatchEnd`), the M68K enqueues multiple operations and waits only once at the end. This hides dispatch latency and keeps the IE64 worker continuously busy. For workloads like ScrollRaster (copy + fill), the two operations execute back-to-back on the IE64 while the M68K continues other work.

## IEWarpMon

IEWarpMon is a SysMon-style MUI application that provides real-time visibility into IE64 coprocessor activity. It uses the monitoring functions (slots 30-38) and extended MMIO registers to display:

- **Summary**: Worker status, uptime, busy%, throughput (ops/sec, bytes/sec), ring buffer health, errors, batch stats
- **Operations**: Per-op accel/fallback counts with acceleration percentage — the key metric for threshold tuning
- **Tasks**: Per-AROS-task breakdown of coprocessor usage
- **Libraries**: Per-consumer-library attribution (exec, IEGfx, graphics, cgfx, AHI, etc.)
- **Waiters**: Tasks currently blocked in IEWarpWait

See `sdk/docs/iewarpmon.md` for the full user guide.
