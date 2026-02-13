# EhBASIC IE64 Coprocessor API

## Overview

The coprocessor subsystem allows EhBASIC programs to launch worker CPUs (IE32, 6502, M68K, Z80, x86) that run service binaries. Workers poll a shared mailbox ring buffer for requests, process them, and write results. The IE64 host manages worker lifecycle and request routing via MMIO registers.

## Statements

### COSTART cpuType, "serviceFile"

Loads and starts a worker CPU from a service binary file.

```basic
COSTART 3, "svc_6502.ie65"     ' Start 6502 worker
COSTART 1, "svc_ie32.iex"     ' Start IE32 worker
```

**cpuType values**: 1=IE32, 3=6502, 4=M68K, 5=Z80, 6=x86 (2=IE64 not supported)

### COSTOP cpuType

Stops a running worker CPU.

```basic
COSTOP 3                       ' Stop 6502 worker
```

### COWAIT ticket [, timeoutMs]

Blocks until the given ticket's request completes or the timeout expires. Default timeout is 1000ms. After COWAIT returns, use `COSTATUS(ticket)` to check the outcome.

```basic
T = COCALL(3, 1, &H1000, 8, &H2000, 16)
COWAIT T, 5000                 ' Wait up to 5 seconds
IF COSTATUS(T) = 2 THEN PRINT "OK"
IF COSTATUS(T) = 4 THEN PRINT "TIMEOUT"
```

## Functions

### COCALL(cpuType, op, reqPtr, reqLen, respPtr, respCap)

Enqueues an async request to a worker CPU. Returns a ticket number.

- **cpuType**: Target worker CPU type (1-6)
- **op**: Operation code (service-defined; op=1 is conventionally "add")
- **reqPtr**: Bus memory address of request data
- **reqLen**: Length of request data in bytes
- **respPtr**: Bus memory address for response data
- **respCap**: Capacity of response buffer in bytes

```basic
POKE &H1000, 10 : POKE &H1004, 20
T = COCALL(3, 1, &H1000, 8, &H2000, 16)
```

### COSTATUS(ticket)

Returns the status of a ticket. Does not block.

| Value | Meaning |
|-------|---------|
| 0 | Pending |
| 1 | Running |
| 2 | OK (completed successfully) |
| 3 | Error |
| 4 | Timeout |
| 5 | Worker down |

```basic
S = COSTATUS(T)
IF S = 2 THEN PRINT "Done"
```

## Ticket Lifecycle

1. `COCALL()` enqueues a request and returns a ticket
2. Worker polls its ring buffer, picks up the request
3. Worker processes the request and writes a response descriptor
4. `COWAIT ticket` blocks until the response is written (or timeout)
5. `COSTATUS(ticket)` reads the outcome (2=ok, 3=error, 4=timeout, 5=worker_down)
6. After two reads of a terminal status via `COSTATUS()`, the ticket entry is evicted

## Memory Map

### MMIO Registers (0xF2340-0xF237F)

| Address | Name | R/W | Description |
|---------|------|-----|-------------|
| 0xF2340 | COPROC_CMD | W | Command register (triggers action) |
| 0xF2344 | COPROC_CPU_TYPE | W | Target CPU type |
| 0xF2348 | COPROC_CMD_STATUS | R | 0=ok, 1=error |
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

### Shared Mailbox RAM (0x820000-0x820FFF)

4KB ring buffer region shared between the Go manager and all worker CPUs.

- 5 rings (one per supported CPU type), each 768 bytes (RING_STRIDE=0x300)
- Ring header: head(u8), tail(u8), capacity(u8) at ring base
- 16 request descriptors (32 bytes each) starting at ring base + 0x08
- 16 response descriptors (16 bytes each) starting at ring base + 0x208

### Worker Load Regions

| CPU Type | Base | End | Size |
|----------|------|-----|------|
| IE32 | 0x200000 | 0x27FFFF | 512KB |
| M68K | 0x280000 | 0x2FFFFF | 512KB |
| 6502 | 0x300000 | 0x30FFFF | 64KB |
| Z80 | 0x310000 | 0x31FFFF | 64KB |
| x86 | 0x320000 | 0x39FFFF | 512KB |

User data buffers (reqPtr/respPtr) should be placed at 0x400000-0x7FFFFF.

## Service Binary Contract

Each service binary must implement:

1. **Init**: start executing at load address (PC set by adapter)
2. **Main loop**: poll head/tail bytes; if tail == head, ring is empty
3. **Read request**: compute entry address from tail index, read descriptor fields
4. **Dispatch**: switch on `op` field
5. **Process**: read request data from reqPtr, write result to respPtr
6. **Complete**: write response descriptor (status=2 for ok, 3 for error)
7. **Advance tail**: write `(tail + 1) % 16` to ring header tail byte

For 6502/Z80, the mailbox is mapped at CPU address $2000-$3FFF via the bus adapter.

### Ring Protocol

**Producer** (Go manager): writes request at entries[head], advances head.
**Consumer** (worker CPU): reads request at entries[tail], writes response at responses[tail], advances tail.
Ring full when `(head + 1) % capacity == tail`.

## Error Codes (COPROC_CMD_ERROR)

| Code | Name | Meaning |
|------|------|---------|
| 0 | NONE | No error |
| 1 | INVALID_CPU | Invalid cpuType (0, 2, or >6) |
| 2 | NOT_FOUND | Service file not found |
| 3 | PATH_INVALID | Path traversal or absolute path |
| 4 | LOAD_FAILED | Failed to load service binary |
| 5 | QUEUE_FULL | Ring buffer full (16 pending requests) |
| 6 | NO_WORKER | No worker running for cpuType |
| 7 | STALE_TICKET | Ticket not found (evicted or invalid) |

## Complete Example

```basic
10 REM Start a 6502 coprocessor worker
20 COSTART 3, "svc_6502.ie65"
30 REM Set up request data: two numbers to add
40 POKE &H1000, 10
50 POKE &H1004, 20
60 REM Submit async request (op=1: add)
70 T = COCALL(3, 1, &H1000, 8, &H2000, 16)
80 REM Wait for result with 5s timeout
90 COWAIT T, 5000
100 IF COSTATUS(T) <> 2 THEN PRINT "ERROR": END
110 REM Read result
120 R = PEEK(&H2000)
130 PRINT "10 + 20 ="; R
140 REM Clean up
150 COSTOP 3
```
