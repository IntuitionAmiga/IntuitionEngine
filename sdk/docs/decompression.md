# Decompression

Intuition Engine accepts compressed music/archive data from user-supplied PSG/YM/SNDH/VTX files and compressed snapshot memory. Decoders are strict by default: once a format sniffer positively identifies a payload, malformed compressed data returns an error instead of silently falling through to another parser.

## Supported Formats

- LHA: `-lh0-`, `-lh1-`, `-lh4-`, `-lh5-`, `-lh6-`, `-lh7-`
- ICE!: Atari ST SNDH packed data
- gzip: snapshot memory and VGM gzip paths
- VTX: metadata wrapper with LH5-compressed AY/YM register data

## Allocation Caps

- LHA original and compressed member sizes: 64 MiB
- ICE! decrunched output: 16 MiB
- VTX uncompressed register data: 8 MiB
- Snapshot memory: 512 MiB
- Snapshot registers: 1024 entries

## Error Contract

Positive sniffers are authoritative. If `isLHAData`, `isICE`, `isVTXData`, or gzip magic matches and decompression fails, callers must propagate the decompression error. Decoders reject truncation, invalid zero-length LH blocks, oversubscribed Huffman tables, invalid back-references, and allocation sizes above the format cap.

LHA extraction returns the first non-directory member. Directory entries (`-lhd-`) are skipped. `-lh0-` stored files require `compressedSize == originalSize`.

## Known Gaps

- LHA file-data CRC16 validation is deferred until real fixtures are added.
- LHA level-2 header CRC validation is not implemented.
- Multi-file LHA listing and extract-by-name APIs are out of scope.
- PP20, RNC, DMS, Shrinkler, ZX0, APLib, ZIP, TAR, 7z, and RAR are tracked separately.
