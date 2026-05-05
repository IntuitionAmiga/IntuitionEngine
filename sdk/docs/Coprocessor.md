# Coprocessor MMIO Contract

The coprocessor subsystem lets guest code start secondary CPU workers, enqueue requests through mailbox rings, and poll or wait for completion.

## Register Map

Primary registers are at `COPROC_BASE = 0xF2340` through `COPROC_END = 0xF238F`.

| Register | Offset | Access | Meaning |
|---|---:|---|---|
| `COPROC_CMD` | `0x00` | W | Write `START`, `STOP`, `ENQUEUE`, `POLL`, or `WAIT`. |
| `COPROC_CPU_TYPE` | `0x04` | RW | Target worker type, one of `EXEC_TYPE_*`. |
| `COPROC_CMD_STATUS` | `0x08` | R | Last command status: `OK` or `ERROR`. |
| `COPROC_CMD_ERROR` | `0x0C` | R | Error detail for failed commands. |
| `COPROC_TICKET` | `0x10` | RW | `ENQUEUE` output ticket; `POLL`/`WAIT` input ticket. |
| `COPROC_TICKET_STATUS` | `0x14` | R | Ticket state after `POLL` or `WAIT`. |
| `COPROC_OP` | `0x18` | RW | Service-defined operation code. |
| `COPROC_REQ_PTR` | `0x1C` | RW | Guest RAM pointer for request data. |
| `COPROC_REQ_LEN` | `0x20` | RW | Request byte length. |
| `COPROC_RESP_PTR` | `0x24` | RW | Guest RAM pointer for response data. |
| `COPROC_RESP_CAP` | `0x28` | RW | Response buffer capacity. |
| `COPROC_TIMEOUT` | `0x2C` | RW | `WAIT` timeout in milliseconds. |
| `COPROC_NAME_PTR` | `0x30` | RW | Guest RAM pointer to a null-terminated worker filename. |
| `COPROC_WORKER_STATE` | `0x34` | R | Bitmask of live workers. |
| `COPROC_STATS_OPS` | `0x38` | R | Total dispatched operations. |
| `COPROC_STATS_BYTES` | `0x3C` | R | Total request bytes dispatched. |
| `COPROC_IRQ_CTRL` | `0x40` | RW | Bit 0 enables completion IRQs. |
| `COPROC_DISPATCH_OVERHEAD` | `0x44` | R | Calibrated dispatch overhead in nanoseconds. |
| `COPROC_COMPLETED_TICKET` | `0x48` | R | Most recent completed ticket. |

Monitor registers are at `COPROC_EXT_BASE = 0xF23B0` through `COPROC_EXT_END = 0xF23BF`.

| Register | Access | Meaning |
|---|---|---|
| `COPROC_RING_DEPTH` | R | Ring occupancy for the selected `COPROC_CPU_TYPE`. |
| `COPROC_WORKER_UPTIME` | R | Seconds since the selected worker started. |
| `COPROC_STATS_RESET` | W | Write `1` to clear stats and busy buckets. |
| `COPROC_BUSY_PCT` | R | Aggregate busy percentage across all workers over the rolling window. |

Write `COPROC_CPU_TYPE` before reading `COPROC_RING_DEPTH` or `COPROC_WORKER_UPTIME`. If the selected type is invalid, the manager falls back to the first running worker.

## Worker Visibility Window

`COPROC_WORKER_STATE` reports a bit per live worker (bit `n` = `EXEC_TYPE_n` worker exists). `computeWorkerState` reaps dead workers *before* reporting, so the bit is only set while the worker goroutine is still scheduled. A worker binary that halts on entry (e.g. `OP_HALT64` as first instruction) may be reaped before any subsequent poll observes it: there is no "ever-existed" latch.

To probe creation, load a worker that stays alive long enough for the poll — a single-instruction self-loop (`OP_BRA` with displacement `0`, since IE64 BRA displacements are relative to the current instruction PC) or a busy-wait on a host-set MMIO flag is sufficient. Tests asserting "worker visible after creation" must use a non-halting binary.

## Command Flow

1. Write `COPROC_CPU_TYPE` and `COPROC_NAME_PTR`, then write `COPROC_CMD_START`.
2. Write `COPROC_CPU_TYPE`, `COPROC_OP`, request fields, response fields, and optional `COPROC_TIMEOUT`, then write `COPROC_CMD_ENQUEUE`.
3. Read `COPROC_TICKET`. Ticket `0` is reserved as the "already complete" sentinel and is never allocated by the manager.
4. Write `COPROC_TICKET`, then write `COPROC_CMD_POLL` or `COPROC_CMD_WAIT`.
5. Read `COPROC_TICKET_STATUS`.

Ticket states are `PENDING`, `RUNNING`, `OK`, `ERROR`, `TIMEOUT`, and `WORKER_DOWN`. `WORKER_DOWN` means the worker slot is empty or the worker goroutine exited before the ticket reached a terminal response.

Command errors include invalid CPU type, missing worker binary, invalid path, load failure, full queue, no worker, and stale ticket.

## Ring Layout

Mailbox RAM starts at `MAILBOX_BASE = 0x790000` and has `MAILBOX_SIZE = 0x1800` bytes. There are six rings, one per CPU type, each with `RING_CAPACITY = 16` and `RING_STRIDE = 0x300`.

Each ring contains:

| Offset | Meaning |
|---:|---|
| `RING_HEAD_OFFSET` | Producer head byte. |
| `RING_TAIL_OFFSET` | Consumer tail byte. |
| `RING_CAPACITY_OFFSET` | Capacity byte, normally `16`. |
| `RING_ENTRIES_OFFSET` | Request descriptors. |
| `RING_RESPONSES_OFFSET` | Response descriptors. |

Request descriptors are 32 bytes:

| Offset | Meaning |
|---:|---|
| `REQ_TICKET_OFF` | Ticket ID. |
| `REQ_CPU_TYPE_OFF` | Worker CPU type. |
| `REQ_OP_OFF` | Service operation. |
| `REQ_TIMEOUT_OFF` | Effective timeout metadata. Workers currently treat this as informational; per-request enforcement is not part of the current contract. |
| `REQ_REQ_PTR_OFF` | Request pointer. |
| `REQ_REQ_LEN_OFF` | Request length. |
| `REQ_RESP_PTR_OFF` | Response pointer. |
| `REQ_RESP_CAP_OFF` | Response capacity. |

Response descriptors are 16 bytes: ticket, status, result code, and response length.

## Worker Memory

Workers run from dedicated memory regions:

| CPU | Region |
|---|---|
| IE32 | `WORKER_IE32_BASE` through `WORKER_IE32_END` |
| 6502 | `WORKER_6502_BASE` through `WORKER_6502_END` |
| M68K | `WORKER_M68K_BASE` through `WORKER_M68K_END` |
| Z80 | `WORKER_Z80_BASE` through `WORKER_Z80_END` |
| x86 | `WORKER_X86_BASE` through `WORKER_X86_END` |
| IE64 | `WORKER_IE64_BASE` through `WORKER_IE64_END` |

## 8-Bit Gateway

6502 and Z80 workers cannot address the primary coprocessor MMIO range directly. Their adapters redirect CPU addresses `0xF200` through `0xF24F` to `COPROC_BASE` through `COPROC_END`.

The 8-bit mailbox window is CPU address `0x2000` through `0x2000 + MAILBOX_SIZE - 1`. With the current `MAILBOX_SIZE` this is `0x2000` through `0x37FF`. Addresses `0x3800` through `0x3FFF` are normal worker RAM.

## IRQ And Completion

When completion IRQs are enabled, the manager records the latest completed ticket and can assert the configured M68K interrupt target. The completion watcher is started by M68K boot paths today; non-M68K modes should use `POLL` or `WAIT` to drive completion observation and busy-state cleanup.

## Monitoring

`COPROC_RING_DEPTH` and `COPROC_WORKER_UPTIME` are per selected CPU type. Write `COPROC_CPU_TYPE` before reading them.

`COPROC_BUSY_PCT` is global. It aggregates all worker activity because busy buckets are currently manager-wide, not per worker.
