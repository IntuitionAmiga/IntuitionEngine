# Host SDK include files

The host SDK public include directory contains one C hardware header,
`intuitionengine.h`, and the six assembly files `ie32.inc`, `ie64.inc`,
`ie68.inc`, `ie80.inc`, `ie65.inc` and `ie86.inc`. The
6502 linker configurations are `share/cc65/ie65.cfg`,
`share/cc65/ie65_bindata.cfg` and `share/cc65/ie65_service.cfg`.

`intuitionengine.h` defines `IE_VIDEO_CTRL_ENABLE` and
`IE_VIDEO_CTRL_PRESENT_HOLD`. Set both bits while updating a retained
framebuffer, then write only `IE_VIDEO_CTRL_ENABLE` after final blit completion
to present the completed frame without exposing intermediate updates.

IE64 freestanding standard headers are private to `ie64-cproc` at
`lib/ie64-unknown-none/include`; the driver adds that path automatically.
They are deliberately outside `include` so an external compiler using
`-I <sdk>/include` retains its own standard library headers.
