//go:build headless

package main

import (
	"os"
	"testing"
	"time"
)

func TestVoodoo_SwapBufferCmd_ClearBit_Honored(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_COLOR0, 0xFF112233)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)
	v.HandleWrite(VOODOO_COLOR0, 0xFF000000)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, VOODOO_SWAP_CLEAR)

	for i := 0; i < 16; i += 4 {
		if got := sw.colorBuffer[i : i+4]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0xFF {
			t.Fatalf("post-swap-clear pixel bytes = %v, want opaque black", got)
		}
	}
}

func TestVoodoo_ClipMask_UsesFull16BitRegisterFields(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, 0x1234ABCD)
	v.HandleWrite(VOODOO_CLIP_LOW_Y_HIGH, 0x2345BCDE)

	if v.clipLeft != 0x1234 || v.clipRight != 0xABCD || v.clipTop != 0x2345 || v.clipBottom != 0xBCDE {
		t.Fatalf("clip = L:%#x R:%#x T:%#x B:%#x", v.clipLeft, v.clipRight, v.clipTop, v.clipBottom)
	}
}

func TestVoodoo_VertexW_DefaultsTo1(t *testing.T) {
	_, v := newMappedTestVoodoo(t)

	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	tri := v.triangleBatch[0]
	for i, vertex := range tri.Vertices {
		if vertex.W != 1.0 {
			t.Fatalf("vertex %d W = %f, want 1.0", i, vertex.W)
		}
	}
}

func TestVoodoo_CombineColor_AllEightModes(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(4, 4); err != nil {
		t.Fatal(err)
	}
	vert := [4]float32{0.2, 0.4, 0.6, 0.5}
	tex := [4]float32{0.7, 0.3, 0.5, 0.25}
	tests := []struct {
		mode uint32
		want [4]float32
	}{
		{VOODOO_CC_ZERO, [4]float32{0, 0, 0, 0}},
		{VOODOO_CC_CSUB_CL, [4]float32{0.5, -0.1, -0.1, -0.25}},
		{VOODOO_CC_ALOCAL, [4]float32{0.1, 0.2, 0.3, 0.25}},
		{VOODOO_CC_AOTHER, [4]float32{0.05, 0.1, 0.15, 0.125}},
		{VOODOO_CC_CLOCAL, [4]float32{0.2, 0.4, 0.6, 0.5}},
		{VOODOO_CC_ALOCAL_T, [4]float32{0.35, 0.15, 0.25, 0.125}},
		{VOODOO_CC_CLOC_MUL, [4]float32{0.14, 0.12, 0.3, 0.125}},
		{VOODOO_CC_AOTHER_T, [4]float32{0.175, 0.075, 0.125, 0.0625}},
	}
	for _, tt := range tests {
		b.SetColorPath(VOODOO_CC_COLOR1 | (tt.mode << VOODOO_FCP_CC_MSELECT_SHIFT))
		r, g, bv, a := b.combineColors(vert[0], vert[1], vert[2], vert[3], tex[0], tex[1], tex[2], tex[3])
		got := [4]float32{r, g, bv, a}
		for i := range got {
			if abs32(got[i]-tt.want[i]) > 0.0001 {
				t.Fatalf("mode %d channel %d = %f, want %f", tt.mode, i, got[i], tt.want[i])
			}
		}
	}
}

func TestVoodoo_PerspectiveTex_W1OverDivide(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(4, 4); err != nil {
		t.Fatal(err)
	}
	v0 := &VoodooVertex{S: 0.0, T: 0.0, W: 1.0}
	v1 := &VoodooVertex{S: 1.0, T: 0.0, W: 0.25}
	v2 := &VoodooVertex{S: 0.0, T: 1.0, W: 1.0}

	b.SetTextureMode(0)
	linearS, _ := b.interpolateTextureCoords(0.25, 0.50, 0.25, v0, v1, v2)
	b.SetTextureMode(VOODOO_TEX_PERSPECTIVE)
	perspectiveS, _ := b.interpolateTextureCoords(0.25, 0.50, 0.25, v0, v1, v2)

	if linearS != 0.50 {
		t.Fatalf("linear S = %f, want 0.50", linearS)
	}
	if perspectiveS >= linearS {
		t.Fatalf("perspective S = %f, want less than linear %f after W divide", perspectiveS, linearS)
	}
}

func TestVoodoo_SlopeDeltas_StoredAndUsed(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(8, 8); err != nil {
		t.Fatal(err)
	}
	b.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE, 0)
	b.SetSlopes(VoodooSlopes{DRDX: 0x1000}, true)
	b.FlushTriangles([]VoodooTriangle{{
		Vertices: [3]VoodooVertex{
			{X: 1, Y: 1, R: 0, A: 1, W: 1},
			{X: 6, Y: 1, R: 0, A: 1, W: 1},
			{X: 1, Y: 6, R: 0, A: 1, W: 1},
		},
	}})
	idx := (1*b.width + 2) * 4
	if got := b.colorBuffer[idx]; got < 200 {
		t.Fatalf("slope-derived red at x=2 = %d, want near full red", got)
	}
}

func TestVoodoo_YOrigin_FlipApplied(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(8, 8); err != nil {
		t.Fatal(err)
	}
	b.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_Y_ORIGIN, 0)
	b.FlushTriangles([]VoodooTriangle{{
		Vertices: [3]VoodooVertex{
			{X: 1, Y: 1, R: 1, A: 1, W: 1},
			{X: 4, Y: 1, R: 1, A: 1, W: 1},
			{X: 1, Y: 4, R: 1, A: 1, W: 1},
		},
	}})
	topIdx := (1*b.width + 1) * 4
	bottomIdx := ((b.height-2)*b.width + 1) * 4
	if b.colorBuffer[topIdx] != 0 {
		t.Fatalf("top pixel was written with Y_ORIGIN set")
	}
	if b.colorBuffer[bottomIdx] == 0 {
		t.Fatalf("bottom-flipped pixel was not written with Y_ORIGIN set")
	}
}

func TestVoodoo_DrawFront_DrawBack_AlphaPlanes(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(8, 8); err != nil {
		t.Fatal(err)
	}
	tri := VoodooTriangle{Vertices: [3]VoodooVertex{
		{X: 1, Y: 1, R: 1, A: 0.25, W: 1},
		{X: 4, Y: 1, R: 1, A: 0.25, W: 1},
		{X: 1, Y: 4, R: 1, A: 0.25, W: 1},
	}}

	b.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DRAW_FRONT, 0)
	b.FlushTriangles([]VoodooTriangle{tri})
	idx := (1*b.width + 1) * 4
	if b.colorBuffer[idx] != 0 {
		t.Fatalf("back buffer changed during DRAW_FRONT")
	}
	if b.frontBuffer[idx] == 0 {
		t.Fatalf("front buffer not changed during DRAW_FRONT")
	}
	if b.frontBuffer[idx+3] != 0xFF {
		t.Fatalf("alpha without ALPHA_PLANES = %#02x, want 0xFF", b.frontBuffer[idx+3])
	}

	b.Reset()
	b.UpdatePipelineState(VOODOO_FBZ_RGB_WRITE|VOODOO_FBZ_DRAW_BACK|VOODOO_FBZ_ALPHA_PLANES, 0)
	b.FlushTriangles([]VoodooTriangle{tri})
	if b.colorBuffer[idx] == 0 {
		t.Fatalf("back buffer not changed during DRAW_BACK")
	}
	if b.colorBuffer[idx+3] >= 0x80 {
		t.Fatalf("alpha with ALPHA_PLANES = %#02x, want preserved low alpha", b.colorBuffer[idx+3])
	}
}

func TestVoodoo_MegaDemoPrebuilt_ProducesNonBlackFrame(t *testing.T) {
	program, err := os.ReadFile("sdk/examples/prebuilt/voodoo_mega_demo.iex")
	if err != nil {
		t.Skipf("prebuilt demo unavailable: %v", err)
	}

	bus, v := newMappedTestVoodoo(t)
	cpu := NewCPU(bus)
	cpu.LoadProgramBytes(program)
	cpu.StartExecution()
	time.Sleep(250 * time.Millisecond)
	cpu.Stop()

	frame := v.backend.GetFrame()
	if len(frame) == 0 {
		t.Fatal("Voodoo backend produced no frame")
	}
	for i := 0; i+3 < len(frame); i += 4 {
		if frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0 {
			return
		}
	}
	t.Fatal("prebuilt Voodoo mega demo frame is black")
}

func TestVoodoo_MegaDemoPrebuilt_PublishesNonBlackEngineFrame(t *testing.T) {
	program, err := os.ReadFile("sdk/examples/prebuilt/voodoo_mega_demo.iex")
	if err != nil {
		t.Skipf("prebuilt demo unavailable: %v", err)
	}

	bus, v := newMappedTestVoodoo(t)
	cpu := NewCPU(bus)
	cpu.LoadProgramBytes(program)
	cpu.StartExecution()
	time.Sleep(250 * time.Millisecond)
	cpu.Stop()

	frame := v.GetFrame()
	if len(frame) == 0 {
		t.Fatal("Voodoo engine produced no frame")
	}
	for i := 0; i+3 < len(frame); i += 4 {
		if (frame[i] != 0 || frame[i+1] != 0 || frame[i+2] != 0) && frame[i+3] != 0 {
			return
		}
	}
	t.Fatal("prebuilt Voodoo mega demo engine frame has no visible compositable pixels")
}
