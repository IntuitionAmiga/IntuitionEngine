# HOST Keyword Bridge

The HOST bridge exposes a small, security-sensitive MMIO aperture for guests
that need to ask the Intuition Engine host process to run a controlled
maintenance action. The aperture is mapped in the normal VM bus; command
execution is gated by `-ehbasic-host`.

## MMIO Layout

| Offset | Address | Name | Access | Description |
|--------|---------|------|--------|-------------|
| `0x00` | `0xF1400` | `HOST_CMD` | read/write | Command selector |
| `0x04` | `0xF1404` | `HOST_TRIGGER` | write | Write non-zero to start the selected command |
| `0x08` | `0xF1408` | `HOST_STATUS` | read | Current command status |
| `0x0C` | `0xF140C` | `HOST_EXIT` | read | 32-bit helper exit code |

The claimed window is `0xF1400` through `0xF140F`. Byte, 16-bit, and 32-bit
reads of `HOST_EXIT` return the addressed part of the 32-bit value.

## Commands

| Value | Command | Meaning |
|-------|---------|---------|
| `1` | `HOST_CMD_NET` | Configure networking through the host helper |
| `2` | `HOST_CMD_UPDATE` | Run the update helper |
| `3` | `HOST_CMD_REBOOT` | Reboot the host |
| `4` | `HOST_CMD_POWEROFF` | Power off the host |

Invalid command writes clear the command selector to `0`.

## Status Values

| Value | Status | Meaning |
|-------|--------|---------|
| `0` | `HOST_STATUS_RUNNING` | Command is in progress |
| `1` | `HOST_STATUS_OK` | Command completed successfully |
| `2` | `HOST_STATUS_ERR` | Command failed |
| `3` | `HOST_STATUS_CANCEL` | Operator cancelled the command |
| `4` | `HOST_STATUS_DISABLED` | Host helper is not enabled |
| `5` | `HOST_STATUS_IDLE` | No command has been triggered yet |

Guests should treat any status other than running as a terminal state.

## Polling Contract

1. Write one command value to `HOST_CMD`.
2. Write a non-zero value to `HOST_TRIGGER`.
3. Poll `HOST_STATUS` until it is no longer `HOST_STATUS_RUNNING`.
4. Read `HOST_EXIT` for helper-specific detail.

Only one command runs at a time. If a second trigger arrives while the helper is
running, it is ignored and the active command remains authoritative.

## Security Model

The MMIO bridge is mapped by default, but host command execution is disabled
unless Intuition Engine is launched with `-ehbasic-host`. `HOST UPDATE`
requires an installed confirmer in normal mode; `-ehbasic-host-appliance`
bypasses that confirmation only for appliance deployments. The host process
delegates privileged work to `/usr/libexec/intuitionengine-host-helper` using
fixed command verbs.
