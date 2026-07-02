//go:build headless

package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// SST-1 binds raster state at triangleCMD time: registers written after
// a triangle is submitted must not affect it, even though the engine
// defers rasterisation to the swap-time batch flush. These tests lock
// that contract (Option A: state-stamped triangle batching).

const (
	sbFbzOpaque = VOODOO_FBZ_RGB_WRITE | VOODOO_FBZ_DEPTH_ENABLE | VOODOO_FBZ_DEPTH_WRITE |
		(VOODOO_DEPTH_ALWAYS << 5)
	sbTexOn = 1 /* TEX_ENABLE */ | (10 << 8) /* ARGB8888 */
)

func sbWriteVertex(v *VoodooEngine, idx int, x, y float32) {
	xr := []uint32{VOODOO_VERTEX_AX, VOODOO_VERTEX_BX, VOODOO_VERTEX_CX}[idx]
	yr := []uint32{VOODOO_VERTEX_AY, VOODOO_VERTEX_BY, VOODOO_VERTEX_CY}[idx]
	v.HandleWrite(xr, uint32(x*16))
	v.HandleWrite(yr, uint32(y*16))
}

// sbSubmitTri submits a flat-shaded triangle covering the given points.
func sbSubmitTri(v *VoodooEngine, x0, y0, x1, y1, x2, y2 float32, r, g, b float32) {
	sbWriteVertex(v, 0, x0, y0)
	sbWriteVertex(v, 1, x1, y1)
	sbWriteVertex(v, 2, x2, y2)
	v.HandleWrite(VOODOO_START_R, uint32(r*4096))
	v.HandleWrite(VOODOO_START_G, uint32(g*4096))
	v.HandleWrite(VOODOO_START_B, uint32(b*4096))
	v.HandleWrite(VOODOO_START_A, 4096)
	v.HandleWrite(VOODOO_START_Z, 2048)
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
}

// sbUploadTexture1x1 uploads a single-texel ARGB8888 texture. The
// texture window stores u32 values little-endian into the backend's
// RGBA byte stream, so the value is R | G<<8 | B<<16 | A<<24.
func sbUploadTexture1x1(v *VoodooEngine, r, g, b byte) {
	v.HandleTexMemWrite(VOODOO_TEXMEM_BASE,
		uint32(r)|uint32(g)<<8|uint32(b)<<16|0xFF000000)
	v.HandleWrite(VOODOO_TEX_WIDTH, 1)
	v.HandleWrite(VOODOO_TEX_HEIGHT, 1)
	v.HandleWrite(VOODOO_TEX_UPLOAD, 1)
}

func sbPixel(t *testing.T, sw *VoodooSoftwareBackend, x, y int) (byte, byte, byte) {
	t.Helper()
	idx := (y*sw.width + x) * 4
	if idx+3 >= len(sw.colorBuffer) {
		t.Fatalf("pixel (%d,%d) outside colour buffer", x, y)
	}
	return sw.colorBuffer[idx], sw.colorBuffer[idx+1], sw.colorBuffer[idx+2]
}

func sbExpectColor(t *testing.T, sw *VoodooSoftwareBackend, x, y int, wr, wg, wb byte, what string) {
	t.Helper()
	r, g, b := sbPixel(t, sw, x, y)
	if !sbNear(r, wr) || !sbNear(g, wg) || !sbNear(b, wb) {
		t.Fatalf("%s: pixel (%d,%d) = %d,%d,%d, want ~%d,%d,%d", what, x, y, r, g, b, wr, wg, wb)
	}
}

func sbNear(got, want byte) bool {
	d := int(got) - int(want)
	return d >= -8 && d <= 8
}

// A frame that mixes untextured and textured triangles must rasterise
// each under the state current when its TRIANGLE_CMD was written.
func TestVoodoo_StateBindsAtTriangleCmd_TwoStateFrame(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Untextured red triangle on the left.
	v.HandleWrite(VOODOO_TEXTURE_MODE, 0)
	sbSubmitTri(v, 40, 40, 160, 40, 40, 160, 1, 0, 0)

	// Enable texturing with a solid green texel; triangle on the right.
	sbUploadTexture1x1(v, 0, 255, 0)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	sbSubmitTri(v, 300, 40, 420, 40, 300, 160, 1, 1, 1)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	sbExpectColor(t, sw, 60, 60, 255, 0, 0, "untextured triangle keeps its own state")
	sbExpectColor(t, sw, 320, 60, 0, 255, 0, "textured triangle samples its texture")
}

// A texture uploaded after a triangle was submitted must not
// retroactively change what that triangle samples.
func TestVoodoo_MidFrameTextureSwitch(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)

	sbUploadTexture1x1(v, 255, 0, 0)
	sbSubmitTri(v, 40, 40, 160, 40, 40, 160, 1, 1, 1)

	sbUploadTexture1x1(v, 0, 0, 255)
	sbSubmitTri(v, 300, 40, 420, 40, 300, 160, 1, 1, 1)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	sbExpectColor(t, sw, 60, 60, 255, 0, 0, "first triangle keeps the red texture")
	sbExpectColor(t, sw, 320, 60, 0, 0, 255, "second triangle samples the blue texture")
}

// The scissor rectangle is part of the bound state.
func TestVoodoo_ScissorBindsPerTriangle(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque|VOODOO_FBZ_CLIPPING)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_TEXTURE_MODE, 0)

	// Clip rect excludes the left triangle entirely.
	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, (200<<16)|640)
	v.HandleWrite(VOODOO_CLIP_LOW_Y_HIGH, (0<<16)|480)
	sbSubmitTri(v, 40, 40, 160, 40, 40, 160, 1, 0, 0)

	// Full-screen clip for the right triangle.
	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, (0<<16)|640)
	v.HandleWrite(VOODOO_CLIP_LOW_Y_HIGH, (0<<16)|480)
	sbSubmitTri(v, 300, 40, 420, 40, 300, 160, 0, 1, 0)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	sbExpectColor(t, sw, 60, 60, 0, 0, 0, "left triangle scissored by its own clip rect")
	sbExpectColor(t, sw, 320, 60, 0, 255, 0, "right triangle visible under its own clip rect")
}

// Consecutive triangles under unchanged state share one snapshot; a
// state write between triangles produces a new one.
func TestVoodoo_StateSnapshotsSharedUntilDirty(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)

	sbSubmitTri(v, 10, 10, 20, 10, 10, 20, 1, 0, 0)
	sbSubmitTri(v, 30, 10, 40, 10, 30, 20, 1, 0, 0)
	v.HandleWrite(VOODOO_ALPHA_MODE, 1<<4)
	sbSubmitTri(v, 50, 10, 60, 10, 50, 20, 1, 0, 0)

	if len(v.triangleBatch) != 3 {
		t.Fatalf("batch length = %d, want 3", len(v.triangleBatch))
	}
	if v.triangleBatch[0].State == nil {
		t.Fatal("triangles must carry a state snapshot")
	}
	if v.triangleBatch[0].State != v.triangleBatch[1].State {
		t.Fatal("unchanged state must share one snapshot")
	}
	if v.triangleBatch[1].State == v.triangleBatch[2].State {
		t.Fatal("a state write between triangles must produce a new snapshot")
	}
	if v.triangleBatch[2].State.AlphaMode != 1<<4 {
		t.Fatalf("new snapshot alphaMode = %#x, want %#x", v.triangleBatch[2].State.AlphaMode, 1<<4)
	}
}

func TestVoodoo_ResetClearsRasterSnapshotCache(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	sbUploadTexture1x1(v, 255, 0, 0)
	sbSubmitTri(v, 10, 10, 20, 10, 10, 20, 1, 0, 0)

	if len(v.triangleBatch) != 1 || v.triangleBatch[0].State == nil {
		t.Fatal("pre-reset triangle did not capture raster state")
	}
	oldState := v.triangleBatch[0].State

	v.Reset()
	if stale := v.triangleBatch[:1][0].State; stale != nil {
		t.Fatalf("reset retained a stale raster snapshot in the batch backing slot: %v", stale)
	}
	sbSubmitTri(v, 30, 10, 40, 10, 30, 20, 0, 1, 0)

	if len(v.triangleBatch) != 1 {
		t.Fatalf("post-reset batch length = %d, want 1", len(v.triangleBatch))
	}
	newState := v.triangleBatch[0].State
	if newState == nil {
		t.Fatal("post-reset triangle did not capture raster state")
	}
	if newState == oldState {
		t.Fatal("post-reset triangle reused pre-reset raster snapshot")
	}
	if newState.TextureMode != 0 || newState.Texture != nil {
		t.Fatalf("post-reset texture state = mode %#x texture %v, want defaults", newState.TextureMode, newState.Texture)
	}
	if newState.FbzMode != v.fbzMode || newState.ClipRight != int(VOODOO_DEFAULT_WIDTH) || newState.ClipBottom != int(VOODOO_DEFAULT_HEIGHT) {
		t.Fatalf("post-reset snapshot did not capture power-on defaults: %+v", newState)
	}
}

func TestVoodoo_SwapClearsStampedTriangleSlots(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	sbUploadTexture1x1(v, 255, 0, 0)
	sbSubmitTri(v, 10, 10, 20, 10, 10, 20, 1, 1, 1)
	sbUploadTexture1x1(v, 0, 0, 255)
	sbSubmitTri(v, 30, 10, 40, 10, 30, 20, 1, 1, 1)

	if len(v.triangleBatch) != 2 {
		t.Fatalf("pre-swap batch length = %d, want 2", len(v.triangleBatch))
	}
	for i := range v.triangleBatch {
		if v.triangleBatch[i].State == nil || v.triangleBatch[i].State.Texture == nil {
			t.Fatalf("pre-swap triangle %d missing stamped texture state", i)
		}
	}

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	if len(v.triangleBatch) != 0 {
		t.Fatalf("post-swap batch length = %d, want 0", len(v.triangleBatch))
	}
	for i, tri := range v.triangleBatch[:2] {
		if tri.State != nil {
			t.Fatalf("post-swap backing slot %d retained state %v", i, tri.State)
		}
	}
}

func TestVoodoo_DebugSnapshotRestoresPendingRasterState(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	sbUploadTexture1x1(v, 17, 34, 51)
	v.HandleWrite(VOODOO_FBZCOLOR_PATH, 0x12345678)

	version, data, err := v.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}

	v.Reset()
	if err := v.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}

	sbSubmitTri(v, 10, 10, 20, 10, 10, 20, 1, 1, 1)
	if len(v.triangleBatch) != 1 {
		t.Fatalf("batch length = %d, want 1", len(v.triangleBatch))
	}
	st := v.triangleBatch[0].State
	if st == nil {
		t.Fatal("restored triangle did not capture raster state")
	}
	if st.Texture == nil {
		t.Fatal("restored pending texture was not stamped onto the triangle")
	}
	if st.Texture.Width != 1 || st.Texture.Height != 1 || len(st.Texture.Data) < 4 {
		t.Fatalf("restored texture = %+v, want 1x1 RGBA data", st.Texture)
	}
	if got := st.Texture.Data[:4]; got[0] != 17 || got[1] != 34 || got[2] != 51 || got[3] != 255 {
		t.Fatalf("restored texture bytes = %v, want [17 34 51 255]", got)
	}
	if !st.ColorPathWritten || st.FbzColorPath != 0x12345678 {
		t.Fatalf("restored colour path = written %v value %#x, want written %#x", st.ColorPathWritten, st.FbzColorPath, uint32(0x12345678))
	}
}

func TestVoodoo_DebugSnapshotSharesTextureStateTable(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	texture := make([]byte, VOODOO_TEXMEM_SIZE)
	for i := range texture {
		texture[i] = byte((i*31 + 7) & 0xff)
	}
	copy(v.textureMemory, texture)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	v.HandleWrite(VOODOO_TEX_WIDTH, 128)
	v.HandleWrite(VOODOO_TEX_HEIGHT, 128)
	v.HandleWrite(VOODOO_TEX_UPLOAD, 1)
	for i := 0; i < 64; i++ {
		x := float32(10 + i%8*20)
		y := float32(10 + i/8*20)
		sbSubmitTri(v, x, y, x+8, y, x, y+8, 1, 1, 1)
	}

	version, data, err := v.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	if version != voodooSnapshotVersion {
		t.Fatalf("snapshot version = %d, want %d", version, voodooSnapshotVersion)
	}

	encodedTexture := []byte(base64.StdEncoding.EncodeToString(texture))
	if got := bytes.Count(data, encodedTexture); got != 2 {
		t.Fatalf("full texture data appears %d times in snapshot, want 2 (texture memory plus shared texture table)", got)
	}

	v.Reset()
	if err := v.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	if len(v.triangleBatch) != 64 {
		t.Fatalf("restored batch length = %d, want 64", len(v.triangleBatch))
	}
	first := v.triangleBatch[0].State
	if first == nil || first.Texture == nil {
		t.Fatal("restored first triangle missing shared texture state")
	}
	for i := range v.triangleBatch {
		if v.triangleBatch[i].State != first {
			t.Fatalf("restored triangle %d did not share the raster state table entry", i)
		}
	}
	if v.batchState != first {
		t.Fatal("restored batchState did not reuse the shared raster state table entry")
	}
	if v.currentTexture != first.Texture {
		t.Fatal("restored currentTexture did not reuse the shared texture table entry")
	}
}

func TestVoodoo_DebugSnapshotRestoresDirtyStateAfterStampedTriangle(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	sbUploadTexture1x1(v, 255, 0, 0)
	sbSubmitTri(v, 10, 10, 20, 10, 10, 20, 1, 1, 1)

	sbUploadTexture1x1(v, 0, 0, 255)
	version, data, err := v.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}

	v.Reset()
	if err := v.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	sbSubmitTri(v, 30, 10, 40, 10, 30, 20, 1, 1, 1)

	if len(v.triangleBatch) != 2 {
		t.Fatalf("batch length = %d, want 2", len(v.triangleBatch))
	}
	first := v.triangleBatch[0].State
	second := v.triangleBatch[1].State
	if first == nil || second == nil {
		t.Fatalf("restored states = %v, %v; want both non-nil", first, second)
	}
	if first == second {
		t.Fatal("dirty restored state reused the pre-upload batch snapshot")
	}
	if first.Texture == nil || len(first.Texture.Data) < 4 {
		t.Fatalf("first restored texture = %+v, want red texture", first.Texture)
	}
	if second.Texture == nil || len(second.Texture.Data) < 4 {
		t.Fatalf("second restored texture = %+v, want blue texture", second.Texture)
	}
	if got := first.Texture.Data[:4]; got[0] != 255 || got[1] != 0 || got[2] != 0 || got[3] != 255 {
		t.Fatalf("first texture bytes = %v, want red", got)
	}
	if got := second.Texture.Data[:4]; got[0] != 0 || got[1] != 0 || got[2] != 255 || got[3] != 255 {
		t.Fatalf("second texture bytes = %v, want blue", got)
	}
}

func TestVoodoo_DebugSnapshotRestoresSharedBatchState(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	sbSubmitTri(v, 10, 10, 20, 10, 10, 20, 1, 0, 0)

	version, data, err := v.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}

	v.Reset()
	if err := v.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	sbSubmitTri(v, 30, 10, 40, 10, 30, 20, 1, 0, 0)

	if len(v.triangleBatch) != 2 {
		t.Fatalf("batch length = %d, want 2", len(v.triangleBatch))
	}
	if v.triangleBatch[0].State == nil || v.triangleBatch[1].State == nil {
		t.Fatalf("restored states = %v, %v; want both non-nil", v.triangleBatch[0].State, v.triangleBatch[1].State)
	}
	if v.triangleBatch[0].State != v.triangleBatch[1].State {
		t.Fatal("unchanged restored state did not keep sharing the batch snapshot")
	}
}

// Triangles without a snapshot (nil State) keep rasterising under the
// backend's current global state, preserving the direct-backend paths.
func TestVoodoo_NilStateTrianglesUseBackendGlobals(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(64, 64); err != nil {
		t.Fatal(err)
	}
	b.UpdatePipelineState(sbFbzOpaque, 0)
	b.ClearFramebuffer(0xFF000000)

	tri := VoodooTriangle{}
	coords := [3][2]float32{{8, 8}, {56, 8}, {8, 56}}
	for i := range tri.Vertices {
		tri.Vertices[i].X = coords[i][0]
		tri.Vertices[i].Y = coords[i][1]
		tri.Vertices[i].R = 1
		tri.Vertices[i].A = 1
		tri.Vertices[i].W = 1
		tri.Vertices[i].Z = 0.5
	}
	b.FlushTriangles([]VoodooTriangle{tri})

	idx := (16*64 + 16) * 4
	if b.colorBuffer[idx] < 200 {
		t.Fatalf("nil-state triangle not rasterised (r=%d)", b.colorBuffer[idx])
	}
}

// The engine-level SetTextureData API (bypassing VOODOO_TEX_UPLOAD)
// must feed state snapshots exactly like the register path.
func TestVoodoo_SetTextureDataAPI_StampsSnapshots(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)

	v.SetTextureData(1, 1, []byte{0, 255, 0, 255})
	sbSubmitTri(v, 40, 40, 160, 40, 40, 160, 1, 1, 1)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	sbExpectColor(t, sw, 60, 60, 0, 255, 0, "SetTextureData texture bound to stamped triangle")
}

// Flushing stamped triangles must not leave the backend mutated to the
// last snapshot: registers written after the last TRIANGLE_CMD are the
// live state and must survive the flush.
func TestVoodoo_FlushRestoresLiveBackendState(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque)
	v.HandleWrite(VOODOO_TEXTURE_MODE, 0)
	sbSubmitTri(v, 40, 40, 160, 40, 40, 160, 1, 0, 0)

	// Live state moves on after the last triangle: texturing enabled,
	// new clip rectangle.
	sbUploadTexture1x1(v, 0, 0, 255)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)
	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, (100<<16)|500)

	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)

	if !sw.textureEnabled {
		t.Fatal("flush must restore live textureEnabled state")
	}
	if sw.textureData == nil {
		t.Fatal("flush must restore live texture data")
	}
	if sw.scissorLeft != 100 || sw.scissorRight != 500 {
		t.Fatalf("flush must restore live scissor (got %d..%d)", sw.scissorLeft, sw.scissorRight)
	}
}
