// voodoo_software_tiles_test.go - tile binning and worker pool differentials.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"math"
	"math/rand"
	"testing"
)

// tileTestBackend builds a backend with a deterministic non-blank starting
// framebuffer, so pixels the raster skips are still observable in the diff.
func tileTestBackend(t *testing.T, w, h int, seed int64) *VoodooSoftwareBackend {
	t.Helper()
	b := &VoodooSoftwareBackend{}
	if err := b.Init(w, h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	r := rand.New(rand.NewSource(seed))
	for i := range b.colorBuffer {
		b.colorBuffer[i] = byte(r.Intn(256))
	}
	for i := range b.depthBuffer {
		b.depthBuffer[i] = r.Float32()
	}
	return b
}

func tileTestVertex(r *rand.Rand, w, h int) VoodooVertex {
	return VoodooVertex{
		X: r.Float32() * float32(w),
		Y: r.Float32() * float32(h),
		Z: r.Float32(),
		R: r.Float32(), G: r.Float32(), B: r.Float32(), A: r.Float32(),
		S: r.Float32(), T: r.Float32(),
	}
}

// tileTestTriangles builds a mixed set: small triangles that never trigger the
// row-band path, tile-boundary-aligned ones, slivers and degenerate ones.
func tileTestTriangles(r *rand.Rand, n, w, h int) []VoodooTriangle {
	tris := make([]VoodooTriangle, 0, n)
	for i := range n {
		var tri VoodooTriangle
		switch i % 5 {
		case 0: // small, wholly inside one tile most of the time
			x := r.Float32() * float32(w)
			y := r.Float32() * float32(h)
			for v := range tri.Vertices {
				tri.Vertices[v] = tileTestVertex(r, w, h)
				tri.Vertices[v].X = x + r.Float32()*20 - 10
				tri.Vertices[v].Y = y + r.Float32()*20 - 10
			}
		case 1: // aligned to a tile boundary, so the fill convention is exercised
			tx := float32((r.Intn(w/voodooTileW + 1)) * voodooTileW)
			ty := float32((r.Intn(h/voodooTileH + 1)) * voodooTileH)
			tri.Vertices[0] = tileTestVertex(r, w, h)
			tri.Vertices[1] = tileTestVertex(r, w, h)
			tri.Vertices[2] = tileTestVertex(r, w, h)
			tri.Vertices[0].X, tri.Vertices[0].Y = tx, ty
			tri.Vertices[1].X, tri.Vertices[1].Y = tx+voodooTileW, ty
			tri.Vertices[2].X, tri.Vertices[2].Y = tx, ty+voodooTileH
		case 2: // sliver
			tri.Vertices[0] = tileTestVertex(r, w, h)
			tri.Vertices[1] = tri.Vertices[0]
			tri.Vertices[1].X += r.Float32() * float32(w)
			tri.Vertices[2] = tri.Vertices[0]
			tri.Vertices[2].Y += 0.25
		case 3: // degenerate, must be dropped identically by both paths
			v := tileTestVertex(r, w, h)
			tri.Vertices[0], tri.Vertices[1], tri.Vertices[2] = v, v, v
		default: // large, spanning many tiles and off-screen edges
			for v := range tri.Vertices {
				tri.Vertices[v] = tileTestVertex(r, w, h)
				tri.Vertices[v].X = tri.Vertices[v].X*2 - float32(w)/2
				tri.Vertices[v].Y = tri.Vertices[v].Y*2 - float32(h)/2
			}
		}
		tris = append(tris, tri)
	}
	return tris
}

func assertBuffersEqual(t *testing.T, what string, a, b *VoodooSoftwareBackend) {
	t.Helper()
	for i := range a.colorBuffer {
		if a.colorBuffer[i] != b.colorBuffer[i] {
			t.Fatalf("%s: colour byte %d = %#02x, want %#02x", what, i, b.colorBuffer[i], a.colorBuffer[i])
		}
	}
	for i := range a.depthBuffer {
		if math.Float32bits(a.depthBuffer[i]) != math.Float32bits(b.depthBuffer[i]) {
			t.Fatalf("%s: depth %d = %#08x, want %#08x", what, i,
				math.Float32bits(b.depthBuffer[i]), math.Float32bits(a.depthBuffer[i]))
		}
	}
}

// runTiledFlush replays a flush through the tiled path with an explicit worker
// count, bypassing the environment switch so the differential does not depend
// on process-wide state.
func runTiledFlush(b *VoodooSoftwareBackend, tris []VoodooTriangle, workers int) {
	b.mutex.RLock()
	live := b.captureLiveStateLocked()
	b.mutex.RUnlock()

	b.fbMu.Lock()
	defer b.fbMu.Unlock()
	if b.tileBinner == nil {
		b.tileBinner = &voodooTileBinner{}
	}
	tb := b.tileBinner
	tb.reset(b.width, b.height)
	var applied *VoodooRasterState
	state := live
	for i := range tris {
		if st := tris[i].State; st != nil && st != applied {
			state = softwareLiveStateFromSnapshot(st)
			applied = st
		}
		setup, minY, maxY, ok := b.buildTriangleSetup(&tris[i], &state)
		if !ok {
			continue
		}
		tb.add(&setup, minY, maxY)
	}
	tb.replay(b, workers)
}

func TestVoodooTileBinning_TriangleCoverageMatchesScanline(t *testing.T) {
	const w, h = 200, 152 // deliberately not a multiple of the tile size
	modes := []struct {
		name      string
		fbzMode   uint32
		alphaMode uint32
	}{
		{"rgb", VOODOO_FBZ_RGB_WRITE, 0},
		{"depth", VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_DEPTH_WRITE, 0},
		{"yflip", VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_Y_ORIGIN, 0},
		{"blend", VOODOO_FBZ_RGB_WRITE, VOODOO_ALPHA_BLEND_EN |
			(VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12)},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			r := rand.New(rand.NewSource(90210))
			tris := tileTestTriangles(r, 120, w, h)

			serial := tileTestBackend(t, w, h, 7)
			tiled := tileTestBackend(t, w, h, 7)
			for _, b := range []*VoodooSoftwareBackend{serial, tiled} {
				if err := b.UpdatePipelineState(mode.fbzMode, mode.alphaMode); err != nil {
					t.Fatalf("UpdatePipelineState: %v", err)
				}
			}
			serial.FlushTriangles(tris)
			runTiledFlush(tiled, tris, 1)
			assertBuffersEqual(t, "tiled serial replay", serial, tiled)
		})
	}
}

func TestVoodooWorkerPool_DeterministicOutput(t *testing.T) {
	const w, h = 192, 128
	r := rand.New(rand.NewSource(5150))
	tris := tileTestTriangles(r, 150, w, h)

	ref := tileTestBackend(t, w, h, 11)
	if err := ref.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_DEPTH_WRITE, 0); err != nil {
		t.Fatalf("UpdatePipelineState: %v", err)
	}
	ref.FlushTriangles(tris)

	for _, workers := range []int{1, 2, 3, 8} {
		for repeat := range 3 {
			got := tileTestBackend(t, w, h, 11)
			if err := got.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_DEPTH_WRITE, 0); err != nil {
				t.Fatalf("UpdatePipelineState: %v", err)
			}
			runTiledFlush(got, tris, workers)
			assertBuffersEqual(t, "workers replay", ref, got)
			_ = repeat
		}
	}
}

// TestVoodooWorkerPool_CommandOrderPreservedPerTile stacks overlapping
// translucent triangles, where any reordering inside a tile changes the result.
func TestVoodooWorkerPool_CommandOrderPreservedPerTile(t *testing.T) {
	const w, h = 128, 128
	alphaMode := uint32(VOODOO_ALPHA_BLEND_EN |
		(VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12))

	r := rand.New(rand.NewSource(1234))
	tris := make([]VoodooTriangle, 0, 64)
	for range 64 {
		var tri VoodooTriangle
		// Every triangle covers the whole frame, so every tile sees the same
		// list and the blend order is the only thing that can differ.
		tri.Vertices[0] = VoodooVertex{X: -16, Y: -16, Z: 0.5, A: 0.35}
		tri.Vertices[1] = VoodooVertex{X: float32(w) + 32, Y: -16, Z: 0.5, A: 0.35}
		tri.Vertices[2] = VoodooVertex{X: float32(w) / 2, Y: float32(h) + 32, Z: 0.5, A: 0.35}
		for v := range tri.Vertices {
			tri.Vertices[v].R = r.Float32()
			tri.Vertices[v].G = r.Float32()
			tri.Vertices[v].B = r.Float32()
		}
		tris = append(tris, tri)
	}

	serial := tileTestBackend(t, w, h, 3)
	if err := serial.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE, alphaMode); err != nil {
		t.Fatalf("UpdatePipelineState: %v", err)
	}
	serial.FlushTriangles(tris)

	for _, workers := range []int{1, 4, 16} {
		tiled := tileTestBackend(t, w, h, 3)
		if err := tiled.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE, alphaMode); err != nil {
			t.Fatalf("UpdatePipelineState: %v", err)
		}
		runTiledFlush(tiled, tris, workers)
		assertBuffersEqual(t, "translucent stack", serial, tiled)
	}
}

// TestVoodooTileRaster_SwitchSelectsTiledPath pins that the environment switch
// is what selects the path, and that both settings agree byte for byte.
func TestVoodooTileRaster_SwitchSelectsTiledPath(t *testing.T) {
	const w, h = 160, 96
	r := rand.New(rand.NewSource(24680))
	tris := tileTestTriangles(r, 80, w, h)

	t.Setenv("IE_VOODOO_TILE_RASTER", "0")
	if voodooTileRasterEnabled() {
		t.Fatal("tiled raster selected with the switch off")
	}
	off := tileTestBackend(t, w, h, 5)
	if err := off.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_DEPTH_WRITE, 0); err != nil {
		t.Fatalf("UpdatePipelineState: %v", err)
	}
	off.FlushTriangles(tris)
	if off.tileBinner != nil {
		t.Fatal("the serial path built binning state")
	}

	t.Setenv("IE_VOODOO_TILE_RASTER", "1")
	if !voodooTileRasterEnabled() {
		t.Fatal("tiled raster not selected with the switch on")
	}
	on := tileTestBackend(t, w, h, 5)
	if err := on.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_DEPTH_WRITE, 0); err != nil {
		t.Fatalf("UpdatePipelineState: %v", err)
	}
	on.FlushTriangles(tris)
	if on.tileBinner == nil {
		t.Fatal("the tiled path did not build binning state")
	}
	assertBuffersEqual(t, "switch differential", off, on)
}

func TestVoodooRasterWorkers_Override(t *testing.T) {
	t.Setenv("IE_VOODOO_WORKERS", "1")
	if got := voodooRasterWorkers(); got != 1 {
		t.Fatalf("workers = %d, want 1", got)
	}
	t.Setenv("IE_VOODOO_WORKERS", "6")
	if got := voodooRasterWorkers(); got != 6 {
		t.Fatalf("workers = %d, want 6", got)
	}
	t.Setenv("IE_VOODOO_WORKERS", "rubbish")
	if got := voodooRasterWorkers(); got < 1 {
		t.Fatalf("workers = %d for an unparsable value, want the default", got)
	}
}

// TestVoodooWorkerPool_WaitSwapIdleDrains pins that a swap after a tiled flush
// still observes every rasterised pixel, i.e. WaitSwapIdle keeps meaning that
// all raster work is complete.
func TestVoodooWorkerPool_WaitSwapIdleDrains(t *testing.T) {
	t.Setenv("IE_VOODOO_TILE_RASTER", "1")
	const w, h = 128, 128
	b := tileTestBackend(t, w, h, 9)
	if err := b.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE, 0); err != nil {
		t.Fatalf("UpdatePipelineState: %v", err)
	}
	b.ClearFramebuffer(0)

	tris := []VoodooTriangle{{Vertices: [3]VoodooVertex{
		{X: 0, Y: 0, R: 1, G: 1, B: 1, A: 1},
		{X: float32(w), Y: 0, R: 1, G: 1, B: 1, A: 1},
		{X: 0, Y: float32(h), R: 1, G: 1, B: 1, A: 1},
	}}}
	b.FlushTriangles(tris)

	painted := 0
	for i := 0; i < len(b.colorBuffer); i += 4 {
		if b.colorBuffer[i] != 0 || b.colorBuffer[i+1] != 0 || b.colorBuffer[i+2] != 0 {
			painted++
		}
	}
	if painted < w*h/4 {
		t.Fatalf("only %d pixels painted after the flush returned, want at least %d", painted, w*h/4)
	}
}

func BenchmarkVoodooRasterScene(b *testing.B) {
	const w, h = 640, 480
	be := &VoodooSoftwareBackend{}
	if err := be.Init(w, h); err != nil {
		b.Fatalf("Init: %v", err)
	}
	if err := be.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_DEPTH_WRITE, 0); err != nil {
		b.Fatalf("UpdatePipelineState: %v", err)
	}
	r := rand.New(rand.NewSource(1))
	tris := make([]VoodooTriangle, 0, 512)
	for range 512 {
		var tri VoodooTriangle
		x := r.Float32() * float32(w)
		y := r.Float32() * float32(h)
		for v := range tri.Vertices {
			tri.Vertices[v] = VoodooVertex{
				X: x + r.Float32()*48, Y: y + r.Float32()*48,
				Z: r.Float32(), R: r.Float32(), G: r.Float32(), B: r.Float32(), A: 1,
			}
		}
		tris = append(tris, tri)
	}

	b.Run("serial", func(b *testing.B) {
		for range b.N {
			be.FlushTriangles(tris)
		}
	})
	b.Run("tiled", func(b *testing.B) {
		for range b.N {
			runTiledFlush(be, tris, voodooRasterWorkers())
		}
	})
}

// BenchmarkVoodooRasterTextured covers the textured, large-triangle shape,
// where the serial path already parallelises over row bands, so it is the
// regression guard for the tiled path rather than a win.
func BenchmarkVoodooRasterTextured(b *testing.B) {
	const w, h = 640, 480
	be := &VoodooSoftwareBackend{}
	if err := be.Init(w, h); err != nil {
		b.Fatalf("Init: %v", err)
	}
	if err := be.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DEPTH_ENABLE|VOODOO_FBZ_DEPTH_WRITE, 0); err != nil {
		b.Fatalf("UpdatePipelineState: %v", err)
	}
	tex := make([]byte, 64*64*4)
	r := rand.New(rand.NewSource(2))
	for i := range tex {
		tex[i] = byte(r.Intn(256))
	}
	be.SetTextureData(64, 64, tex, 0)
	be.SetTextureEnabled(true)

	tris := make([]VoodooTriangle, 0, 16)
	for range 16 {
		var tri VoodooTriangle
		for v := range tri.Vertices {
			tri.Vertices[v] = VoodooVertex{
				X: r.Float32() * float32(w), Y: r.Float32() * float32(h),
				Z: r.Float32(), R: r.Float32(), G: r.Float32(), B: r.Float32(), A: 1,
				S: r.Float32(), T: r.Float32(),
			}
		}
		tris = append(tris, tri)
	}

	b.Run("serial", func(b *testing.B) {
		for range b.N {
			be.FlushTriangles(tris)
		}
	})
	b.Run("tiled", func(b *testing.B) {
		for range b.N {
			runTiledFlush(be, tris, voodooRasterWorkers())
		}
	})
}

// BenchmarkVoodooRasterBlendShare measures what alpha blending costs in the
// software rasteriser. Blended setups are the last class routed away from the
// SIMD row kernel, so the three cases here bound what vectorising blend could
// win: opaque under SIMD, opaque with the SIMD kernel unwired (the scalar
// oracle, i.e. the speedup vectorisation currently buys), and blended, which is
// scalar whatever the build.
func BenchmarkVoodooRasterBlendShare(b *testing.B) {
	const w, h = 640, 480
	blendMode := uint32(VOODOO_ALPHA_BLEND_EN |
		(VOODOO_BLEND_SRC_ALPHA << 8) | (VOODOO_BLEND_INV_SRC_A << 12))

	scenes := []struct {
		name  string
		count int
		size  float32
	}{
		{"small", 512, 48},
		{"large", 16, 0}, // 0 means full-frame spread
	}
	for _, scene := range scenes {
		be := &VoodooSoftwareBackend{}
		if err := be.Init(w, h); err != nil {
			b.Fatalf("Init: %v", err)
		}
		// Slope registers are what make a setup SIMD-eligible, so bind them:
		// without them every case here would take the barycentric scalar path
		// and the SIMD comparison would measure nothing.
		be.SetSlopes(VoodooSlopes{
			DRDX: 0x00000100, DGDX: 0x00000080, DBDX: 0x00000040,
			DADX: 0x00000020, DZDX: 0x00000010,
			DRDY: 0x00000080, DGDY: 0x00000040, DBDY: 0x00000020,
			DADY: 0x00000010, DZDY: 0x00000008,
		}, true)
		r := rand.New(rand.NewSource(1))
		tris := make([]VoodooTriangle, 0, scene.count)
		for range scene.count {
			var tri VoodooTriangle
			x := r.Float32() * float32(w)
			y := r.Float32() * float32(h)
			for v := range tri.Vertices {
				vx, vy := r.Float32()*float32(w), r.Float32()*float32(h)
				if scene.size != 0 {
					vx, vy = x+r.Float32()*scene.size, y+r.Float32()*scene.size
				}
				tri.Vertices[v] = VoodooVertex{
					X: vx, Y: vy, Z: r.Float32(),
					R: r.Float32(), G: r.Float32(), B: r.Float32(), A: 0.5,
				}
			}
			tris = append(tris, tri)
		}

		cases := []struct {
			name      string
			alphaMode uint32
			noSIMD    bool
		}{
			{"opaque", 0, false},
			{"opaque_scalar", 0, true},
			{"blended", blendMode, false},
		}
		for _, c := range cases {
			b.Run(scene.name+"/"+c.name, func(b *testing.B) {
				if err := be.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE, c.alphaMode); err != nil {
					b.Fatalf("UpdatePipelineState: %v", err)
				}
				saved := voodooRasterizeRowsSIMDFn
				if c.noSIMD {
					voodooRasterizeRowsSIMDFn = nil
				}
				defer func() { voodooRasterizeRowsSIMDFn = saved }()
				for range b.N {
					be.FlushTriangles(tris)
				}
			})
		}
	}
}
