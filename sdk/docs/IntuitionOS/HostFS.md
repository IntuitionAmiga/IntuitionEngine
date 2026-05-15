# HostFS Bootstrap Contract

## Overview

The bootstrap HostFS bridge is the narrow host-backed file surface used
before `dos.library` is online. In the M15.2+ runtime this is the
backing for `SYS:` during early boot.

The interface is intentionally small:

- discover whether a host root is mounted
- open / read / close a file
- stat a file or directory
- enumerate a directory
- create / truncate / write files in the writable `SYS:` overlay path

The bootstrap bridge is not a general filesystem API. It exists only to
get the early IntuitionOS boot chain onto the machine safely.

## Host Root Resolution

Developer builds default `SYS:` to `sdk/intuitionos/system/SYS` and boot
the IExec kernel from `sdk/intuitionos/iexec/iexec.ie64`.

The x64 live image passes an explicit live root and kernel path when the
FAT32 share is mounted:

- `SYS:` root: `/var/ie/share/Systems/IntuitionOS`
- IExec kernel: `/var/ie/share/Systems/IntuitionOS/Boot/iexec.ie64`

The guest-visible layout is unchanged by that host path. Inside
IntuitionOS, `SYS:` is the mounted host root and `IOSSYS:` remains
`SYS:IOSSYS`; `Systems/IntuitionOS` is only a live-media staging path.

`-intuitionos-root <dir>` overrides the host-backed `SYS:` root,
`-intuitionos-image <file>` overrides the kernel image, and
`INTUITIONOS_HOST_ROOT` remains a root-only environment override. All
IntuitionOS launch paths use the same resolver, including the BASIC
`INTUITIONOS` command and `EXEC_OP_IEXEC`.

## Security Invariants

M15.6 R5 hardens bootstrap HostFS around two path-resolution rules:

- every path walk is **per-component `NOFOLLOW`**
- every successfully resolved path is **case-normalized to one canonical host path**

Those rules are non-bypassable because all path-based HostFS operations
share the same resolver.

## Per-Component `NOFOLLOW`

Bootstrap HostFS never traverses a symlink component while resolving a
guest path.

That applies to:

- read/open paths
- `stat`
- `readdir`
- writable-overlay create/truncate paths

If any existing component in the walk is a symlink, resolution fails
with the HostFS permission/security error instead of following the link.

This is stricter than only checking the final target. A crafted middle
component such as `SYS:Link/Tools/Shell` must not be able to redirect
resolution through an attacker-chosen subtree, even if that subtree
still points somewhere inside the configured host root.

## Case Normalization

HostFS resolution is case-insensitive for lookup but canonical for the
resolved result.

Example:

```text
host root contains: MiXeD/Name.Txt
guest asks for:     mixed/name.txt
guest asks for:     MIXED/NAME.TXT
resolved host path: MiXeD/Name.Txt
```

This prevents case-variant drift where multiple guest spellings would
otherwise appear to refer to different host paths.

For create/truncate operations, an existing parent or leaf component is
canonicalized to the host's actual spelling before the final path is
used. A missing leaf is created under the canonicalized parent path.

## Writable `SYS:` Overlay

The writable bootstrap path still preserves the M15.3 overlay policy:

- `IOSSYS` is read-only
- `.` / `..` path traversal is rejected
- absolute guest paths are rejected
- final paths must remain under the configured HostFS root

M15.6 R5 adds the stronger guarantee that pre-planted symlinks cannot be
used as intermediate redirectors inside that writable tree.

## Test Pins

R5 is pinned by bootstrap HostFS tests covering:

- middle-component symlink rejection on read resolution
- middle-component symlink rejection on create resolution
- case-variant lookups collapsing to one canonical host path
- existing-leaf create resolution preserving canonical spelling

## M16.4.3 Universal Userland Residency

Universal Userland Residency keeps command lookup assign-driven. A
`DOS_RESIDENT_ADD` request resolves the command through the same canonical
assign/HostFS layering used by `DOS_RUN`, validates the IOSM command metadata,
and snapshots the file into the dos.library resident command cache. Later disk
changes do not alter that cached snapshot until `DOS_RESIDENT_REMOVE` and a
new add.
