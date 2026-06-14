# AROS Host Sockets

The AROS host socket bridge exposes a ROM-resident `bsdsocket.library` in the
`m68k-ie` AROS build. Guest calls are forwarded to the `ArosHostSocketDevice`
MMIO block in IntuitionEngine.

## MMIO Block

The socket bridge uses `0xF2500-0xF257F`. The initial planning note proposed
`0xF2400-0xF247F`, but `0xF2400-0xF24FF` is already reserved for SYSINFO, so the
socket block was moved to the next clear 128-byte range.

Registers are 32-bit:

| Offset | Name | Description |
| --- | --- | --- |
| `0x00` | `CMD` | Command code, write triggers dispatch |
| `0x04` | `REQ_PTR` | Guest pointer to the fixed request descriptor |
| `0x08` | `REQ_LEN` | Descriptor length, currently 96 bytes |
| `0x0C` | `RES1` | Primary result |
| `0x10` | `RES2` | Secondary result |
| `0x14` | `ERRNO` | BSD socket errno |
| `0x18` | `HERRNO` | Resolver `h_errno` |
| `0x1C` | `STATUS` | `0` ready, `1` busy, `2` error |
| `0x20` | `EVENTS` | Pending readiness and socket event bits |

The rest of the 128-byte block is reserved for event masks, descriptor
statistics, async queue pointers and ABI extensions.

## Descriptor Contract

Requests use a fixed 24-word descriptor in guest RAM. Words are big-endian, as
written by the m68k guest. One socket operation uses one descriptor. Payloads
and sockaddr structures are passed as guest pointers and copied in bulk by the
host after span validation.

`send`, `sendto`, `recv` and `recvfrom` clamp payloads to 64 KiB per call.
Unsupported pointer spans, short descriptors and oversized buffers fail before
host allocation or host I/O.

## Scope

V1 targets IPv4 TCP and UDP. Raw sockets, packet filters, route manipulation,
interface configuration and monitoring APIs are out of scope and return stable
unsupported errors.

Host file descriptors are made non-blocking and are hidden behind a
guest-visible descriptor table. The guest can only operate on socket handles
created or duplicated through this bridge; arbitrary emulator process file
descriptors are rejected with `EBADF`. `WaitSelect` translates guest fd_sets
through that table, calls host `select`, and maps ready descriptors back into the
guest ABI fd_set layout. The current host device provides the MMIO ABI,
non-blocking Unix socket primitives, resolver marshalling, event register access
and deterministic disabled-networking behaviour.
