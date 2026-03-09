# IEWarpMon User Guide

## Overview

IEWarpMon is a MUI application for monitoring IE64 coprocessor activity on AROS. It reads live statistics from `iewarp.library` and displays them across five tabs: Summary, Operations, Tasks, Libraries, and Waiters. The window title includes "~ approximate" as a reminder that counters are updated at 250ms intervals and may not reflect exact instantaneous values.

## Tabs

### Summary

The default tab shows a live dashboard with four sections:

**Status row** -- Worker state (Running/Stopped), uptime in seconds, busy percentage (0-100%), and IRQ status (Enabled/Disabled).

**Throughput** -- Total ops dispatched, ops/sec, total bytes processed, bytes/sec, dispatch overhead in nanoseconds, and current threshold. The ring buffer section shows current depth (out of 16 slots), high-water mark, and the last completed ticket number.

**Errors** -- Queue Full (ring was full), Worker Down (IE64 not running), Stale Ticket (waited on an already-completed ticket), Enqueue Fail (dispatch failed).

**Batches** -- Completed batch count, average ops per batch, and maximum batch size.

**Controls:**

- **Start/Stop** -- Toggles the IE64 coprocessor worker via `IEWarpWorkerStop()` / `IEWarpWorkerStart()`. These library functions keep the internal `workerRunning` flag in sync so all consumers see the correct state.
- **Reset Stats** -- Calls `IEWarpResetAllStats()` and zeroes the rate counters. Use this to get a clean measurement baseline.
- **Toggle IRQ** -- Enables or disables the coprocessor completion interrupt.
- **Threshold override** -- Enter a byte value and click Set to call `IEWarpSetThreshold()`. This overrides the adaptive threshold for all subsequent operations.

### Operations

Lists every operation that has been dispatched at least once. Each row shows:

    Op#  Name          Accel:count   Fall:count   Accel%   Bytes

Operations are identified by WARP_OP codes (0-42). The accel/fallback split shows how many calls went to the IE64 coprocessor vs. fell back to M68K software. Accel% is `accelCount * 100 / (accelCount + fallbackCount)`.

Supported operations: NOP (0), MemCpy (1), MemCpyQuick (2), MemSet (3), MemMove (4), BlitCopy (10), BlitMask (11), BlitScale (12), BlitConvert (13), BlitAlpha (14), FillRect (15), AreaFill (16), PixelProcess (17), AudioMix (20), AudioResample (21), AudioDecode (22), FPBatch (30), MatrixMul (31), CRC32 (32), GradientFill (40), GlyphRender (41), Scroll (42).

### Tasks

Per-AROS-task breakdown of coprocessor usage. Each row shows the task name, total ops, and total bytes. Up to 32 tasks are tracked. When all slots are full, additional tasks share an overflow slot. Use Reset Stats to reclaim slots.

### Libraries

Per-consumer breakdown. AROS libraries call `IEWarpSetCaller(callerID)` before dispatching operations so their usage is attributed separately. Known callers: exec (1), graphics (2), IEGfx (3), cgfx (4), AHI (5), icon (6), math (7), MUI (8), freetype (9), datatypes (10), app (11).

### Waiters

Shows tasks currently blocked in `IEWarpWait()`, along with the ticket number they are waiting on. Up to 8 concurrent waiters are tracked. If no tasks are waiting, the list shows "(no tasks waiting)". Use this tab to debug stalls -- a task stuck waiting on a ticket that is far behind the last completed ticket may indicate a lost completion or a bug in the consumer.

## Threshold Tuning

The threshold controls the minimum byte count for an operation to be dispatched to the IE64 coprocessor. Operations below the threshold fall back to M68K software.

- **Lower threshold** -- More operations are accelerated, but small operations may be slower due to dispatch overhead (ring buffer submission, ticket wait). Good for workloads with many medium-sized blits.
- **Higher threshold** -- Only large operations are accelerated. Reduces dispatch overhead for small ops. Good for workloads dominated by tiny fills or single-pixel operations.

**Workflow:**

1. Open the Operations tab.
2. Reset Stats for a clean baseline.
3. Run your workload for a few seconds.
4. Check Accel% for each operation. If an operation has low Accel% but you want it accelerated, lower the threshold. If Overhead (Summary tab) is high relative to throughput, raise the threshold.
5. Enter a new value in the Threshold field on the Summary tab and click Set.
6. Reset Stats and re-run to compare.

The default threshold is 1024 bytes.

## Building

From the AROS source tree:

```
make arch-ie-m68k-utilities-iewarpmon
```

The binary is installed as `Utilities/IEWarpMon` in the AROS system partition. Requires `muimaster.library` (v19+) and `iewarp.library` at runtime.

## API Reference

IEWarpMon uses 11 library functions from `iewarp.library` (9 monitoring + 2 lifecycle):

| Function | Synopsis | Description |
|----------|----------|-------------|
| `WorkerStop()` | `VOID IEWarpWorkerStop() ()` | Stop the IE64 worker and clear the library's `workerRunning` flag (only if MMIO stop succeeds). |
| `WorkerStart()` | `VOID IEWarpWorkerStart() ()` | Restart the IE64 worker (programs NAME_PTR, issues START, sets `workerRunning` on success). |
| `SetCaller(callerID)` | `VOID IEWarpSetCaller(ULONG callerID) (D0)` | Tag subsequent ops from this task with a caller ID (IEWARP_CALLER_*). Enables per-library attribution on the Libraries tab. |
| `GetOpStats(buf, maxOps)` | `ULONG IEWarpGetOpStats(APTR buf, ULONG maxOps) (A0, D0)` | Copy per-op accel/fallback counters into an array of `IEWarpOpCounter` structs. |
| `GetCallerStats(buf, max)` | `ULONG IEWarpGetCallerStats(APTR buf, ULONG maxEntries) (A0, D0)` | Copy per-caller stats into an array of `IEWarpCallerEntry` structs. |
| `GetTaskStats(buf, max)` | `ULONG IEWarpGetTaskStats(APTR buf, ULONG maxEntries) (A0, D0)` | Copy per-task stats into an array of `IEWarpTaskEntry` structs. Returns number of active entries. |
| `GetErrorStats(buf)` | `ULONG IEWarpGetErrorStats(APTR buf) (A0)` | Copy error counters into an `IEWarpErrorStats` struct. |
| `GetBatchStats(buf)` | `ULONG IEWarpGetBatchStats(APTR buf) (A0)` | Copy batch counters into an `IEWarpBatchStats` struct. |
| `GetWaiterInfo(buf, max)` | `ULONG IEWarpGetWaiterInfo(APTR buf, ULONG maxEntries) (A0, D0)` | Copy blocked-task info into an array of `IEWarpWaiterInfo` structs. Returns number of active waiters. |
| `SetThreshold(threshold)` | `VOID IEWarpSetThreshold(ULONG threshold) (D0)` | Override the adaptive byte threshold for acceleration decisions. |
| `ResetAllStats()` | `VOID IEWarpResetAllStats() ()` | Zero all per-op, per-task, per-caller, error, and batch counters. Also writes 1 to COPROC_STATS_RESET to reset hardware-side counters. |

## MMIO Registers

IEWarpMon reads four coprocessor status registers directly via `ie_read32()`:

| Address | Name | Access | Description |
|---------|------|--------|-------------|
| `0xF23B0` | `IE_COPROC_RING_DEPTH` | R | Current IE64 ring buffer occupancy (0-16). |
| `0xF23B4` | `IE_COPROC_WORKER_UPTIME` | R | Seconds since the IE64 worker was started. |
| `0xF23B8` | `IE_COPROC_STATS_RESET` | W | Write 1 to zero hardware-side statistics counters. |
| `0xF23BC` | `IE_COPROC_BUSY_PCT` | R | Worker busy percentage (0-100). |

These supplement the existing coprocessor registers (IE_COPROC_CMD, IE_COPROC_WORKER_STATE, IE_COPROC_IRQ_CTRL, etc.) defined in `ie_hwreg.h`.

## Accuracy Note

Counters displayed by IEWarpMon have different accuracy guarantees:

- **Per-op, error, batch, and high-water counters** are updated without Disable/Enable protection and are approximate under contention. Multiple tasks dispatching simultaneously may cause minor undercounts.
- **Per-task and per-caller counters** are protected by Exec Disable/Enable and are accurate (slot allocation is atomic, so callers are never misattributed).
- **Rate displays** (ops/sec, bytes/sec) are computed from 250ms deltas and may jitter.
- **Reset Stats** provides a clean baseline -- use it before any measurement session.
