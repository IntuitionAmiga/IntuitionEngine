# AROS HostFS

AROS HostFS is the Go-side filesystem bridge used by the AROS m68k-ie handler. The AROS handler translates normal AmigaDOS packets into the existing `IE_DOS_*` MMIO command ABI, and `ArosDOSDevice` performs host filesystem work against the configured host root.

HostFS does not use IEWarp. Existing command numbers and packet semantics remain compatible, including lock, examine, find, read, write, seek, close, create, delete, rename, disk info, and file handle examination commands.

## Fast Paths

File payloads and metadata blocks are copied with bulk guest-memory helpers. `FileInfoBlock`, `InfoData`, C strings, and BSTR fields are built in local byte slices using m68k big-endian field encoding, then copied to guest RAM with `WriteGuestBytes`.

Sequential file reads use a transparent read-ahead cache. Guest reads still observe the same file position semantics, while the host side can satisfy many small reads from one larger `ReadAt` window. Seeks outside the cached range, non-sequential reads, writes, truncates, and close operations invalidate the cache.

`ADOS_CMD_EXAMINE_ALL` accelerates DOS `ACTION_EXAMINE_ALL`. The AROS handler passes a fixed guest request descriptor because the MMIO block has only four argument registers. The descriptor contains five 32-bit big-endian fields: lock key, buffer pointer, buffer size, ExAll type, and control pointer. The Go side packs `ExAllData` entries directly into the guest buffer and uses `eac_LastKey` as the continuation index.

Unsupported ExAll match strings or match hooks return `ERROR_ACTION_NOT_KNOWN` before mutating the control block, so DOS fallback remains available.

## Cache Coherency

Directory entry and case-name caches are invalidated on create, delete, rename, truncate, and dirty close paths. External host changes are best-effort: new lock and examine flows refresh state, but changes made outside the emulator are not continuously watched.

## Console Compatibility

The console handler implements a minimal `ACTION_EXAMINE_FH` response with a synthetic `FileInfoBlock`. This is a compatibility cleanup for callers that examine console handles and is not a HostFS dependency.
