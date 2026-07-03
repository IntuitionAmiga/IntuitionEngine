# Video Compositor

The video compositor blends all registered video devices into the display frame used by the output backend, recorder, and IEScript visual APIs.

## Signal Flow

1. Video sources register with `RegisterSource` or `RegisterSourceWithID`.
2. The compositor ticks at `COMPOSITOR_REFRESH_RATE` (60 Hz).
3. Each tick advances `FrameTicker` sources, drains pending resolution changes, clears the working frame, composites enabled sources, records the frame snapshot metadata, updates the output backend when needed, and fires the frame callback once.
4. Scripts and recorders read snapshots through `GetCurrentFrame` or `GetFrameSnapshot`.

`Close` stops the refresh goroutine and drops source references. A closed compositor cannot be restarted.

## Layer Rules

Sources are stored in stable ascending layer order. Lower layers render first; higher layers overwrite later when their source pixel alpha is nonzero. Stable sorting means equal-layer sources keep registration order.

`RegisterSource(VideoSource)` keeps the historical no-return API. `RegisterSourceWithID(VideoSource)` returns a monotonic id for `UnregisterSource(id)`.

## Scanline Rules

If at least one enabled source implements `ScanlineAware`, the compositor uses the scanline-aware path:

1. It marks scanline-capable sources as compositor-managed and waits for in-flight render goroutines to idle.
2. It calls `StartFrame` on those sources.
3. It walks scanlines from 0 to the maximum scanline-aware source height, calling `ProcessScanline` in layer order. Smaller sources receive their last valid scanline for out-of-range rows.
4. It calls `FinishFrame` and stores each scanline source frame.
5. It blends every enabled source in global layer order. Scanline-aware sources use their finished frame; opaque sources use `GetFrame`.

This preserves copper-style per-scanline effects while allowing opaque sources below, between, or above scanline-aware layers.

If exactly one scanline-aware source also implements `ScanlineBatchAware`, the compositor calls `ProcessScanlineRange(0, height)` once instead of taking the source lock once per scanline. The multi-source path intentionally keeps per-scanline interleaving because palette and register side effects can be visible to higher layers.

## Alpha Mask

Alpha is a binary mask. Alpha 0 is transparent. Any nonzero alpha, including partial alpha, replaces the destination pixel. Real alpha blending, multi-format pixels, bilinear filtering, and blend-mode work are future pipeline tasks.

Sources that implement `OpaqueFrameSource` may use a faster copy path. That path still promotes pixels with zero alpha to `0xFFRRGGBB`, so it must only be used by sources whose frame is known to contain no transparent holes.

## Dirty Regions

Sources can implement `DirtyFrameSource` to atomically take source-space rectangles changed since the previous frame. The compositor scales those rectangles into presentation coordinates while still rendering the full software frame. If the output implements `RegionUpdatingOutput`, only those regions are uploaded. Backends without region updates continue to receive `UpdateFrame`.

IEVideoChip reports dirty rectangles by atomically taking and clearing its tile bitmap in one operation. CLUT8 VideoChip frames are marked opaque because palette conversion writes alpha 255 for every pixel. RAM-backed CLUT8 framebuffers are not cached because guest CPU stores can bypass VideoChip dirty tracking.

## Timing

The compositor tick is fixed at 60 Hz because AROS and EmuTOS depend on 60 Hz VBlank behavior. `GetTickRate()` reports this fixed tick. `GetRefreshRate()` reports the output backend refresh rate and falls back to 60 when the backend is unavailable or reports an invalid rate.

Ticker ownership is centralised in `VideoScheduler`. The compositor registers
its compose task with that scheduler, and migrated VGA, ULA, TED video, and
ANTIC render-loop compatibility shims use the same scheduler abstraction rather
than owning `time.NewTicker` directly. Tests can use the manual scheduler path
to advance video work deterministically without sleeping on wall-clock
goroutines.

The frame callback fires exactly once per composite pass, including all-idle frames. A transition from visible content to no content pushes one cleared frame to avoid stale output; repeated empty frames do not spam the backend.

Same-size software blending is split into 60-line strips. The compositor owns a persistent worker pool for those strips and shuts it down from `Close`, avoiding per-frame goroutine allocation on the common 1:1 path.

## Resolution

Default desktop boot starts locked at `DefaultPresentationWidth` by `DefaultPresentationHeight` (`1920x1080`). Native video sources can still change their own dimensions; those frames are scaled into the presentation frame during blending. The default native IEVideoChip mode is `MODE_960x540`, which fills 1080p at exact 2x scale.

`LockResolution` pins a presentation size and ignores later notifications until `UnlockResolution`. `SetDimensions` is also ignored while locked. Runtime notifications still update native mouse-coordinate mapping outside the compositor.

The default scale mode is aspect-fit. Non-16:9 sources are letterboxed or pillarboxed; `F11` toggles stretch-fill only when the active native source is not already 16:9. `Shift+F11` toggles fullscreen/windowed mode unless fullscreen is locked by the live image launcher.

`pendingResolution` is a packed `uint64` with zero as the no-pending sentinel. Public resolution-change paths reject non-positive dimensions before packing, so a valid pending resolution cannot be zero.

## Fault Isolation

Source callbacks are wrapped with compositor-local panic recovery. A panicking source can lose that frame, but it does not kill the compositor loop. Compositor-managed sources are released with deferred `SetCompositorManaged(false)` calls even if scanline processing panics.
