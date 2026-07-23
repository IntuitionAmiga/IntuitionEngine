// video_gpu_convert_wiring_test.go - indexed layer selection and expansion.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"math/rand"
	"sync"
	"testing"
)

// indexedTestSource publishes a CLUT8 frame both ways: as indices for a backend
// that expands on the GPU, and as expanded RGBA for everything else.
type indexedTestSource struct {
	mockOpaqueSource
	w, h    int
	indices []byte
	palette [256]uint32
	offer   bool

	mu    sync.Mutex
	calls int
}

func newIndexedTestSource(w, h int, offer bool, seed int64) *indexedTestSource {
	r := rand.New(rand.NewSource(seed))
	s := &indexedTestSource{w: w, h: h, offer: offer}
	s.indices = make([]byte, w*h)
	for i := range s.indices {
		s.indices[i] = byte(r.Intn(256))
	}
	for i := range s.palette {
		s.palette[i] = uint32(r.Intn(1<<24)) | 0xFF000000
	}
	s.enabled.Store(true)
	return s
}

func (s *indexedTestSource) GetDimensions() (int, int) { return s.w, s.h }
func (s *indexedTestSource) GetFrame() []byte          { return nil }

func (s *indexedTestSource) CopyFrameForCompositor(dst []byte) ([]byte, bool) {
	need := s.w * s.h * BYTES_PER_PIXEL
	if len(dst) < need {
		return nil, false
	}
	clut8ExpandSpanScalar(dst[:need], s.indices, &s.palette)
	return dst[:need], true
}

func (s *indexedTestSource) IndexedFrameForCompositor(dst []byte) ([256]uint32, bool) {
	if !s.offer {
		return [256]uint32{}, false
	}
	if len(dst) < len(s.indices) {
		return [256]uint32{}, false
	}
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	copy(dst, s.indices)
	return s.palette, true
}

func (s *indexedTestSource) indexedCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// indexedMockOutput is a hardware compositing output that advertises whether it
// can expand palette indices itself.
type indexedMockOutput struct {
	*mockVideoOutput
	accepts bool

	mu     sync.Mutex
	layers []CompositorFrameLayer
}

func (o *indexedMockOutput) AcceptsIndexedLayers() bool { return o.accepts }

func (o *indexedMockOutput) UpdateHardwareCompositorFrame(update CompositorFrameUpdate) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.layers = append([]CompositorFrameLayer(nil), update.Layers...)
	return nil
}

func (o *indexedMockOutput) HardwareCompositorSnapshot(uint64) ([]byte, bool) { return nil, false }

func (o *indexedMockOutput) takeLayers() []CompositorFrameLayer {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.layers
}

func newIndexedMockOutput(t *testing.T, accepts bool) *indexedMockOutput {
	t.Helper()
	base := newMockVideoOutput()
	if err := base.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return &indexedMockOutput{mockVideoOutput: base, accepts: accepts}
}

// TestIndexedLayer_ReachesBackendWhenGPUConversionSelected pins the wiring: an
// indexed source and an index-capable output must produce a layer that carries
// indices and no RGBA buffer, which is what skips the CPU expansion.
func TestIndexedLayer_ReachesBackendWhenGPUConversionSelected(t *testing.T) {
	t.Setenv("IE_VIDEO_GPU_CONVERT", "1")
	out := newIndexedMockOutput(t, true)
	comp := NewVideoCompositor(out)
	comp.LockResolution(64, 64)
	src := newIndexedTestSource(64, 64, true, 5)
	comp.RegisterSource(src)

	comp.composite()

	layers := out.takeLayers()
	if len(layers) != 1 {
		t.Fatalf("output received %d layers, want 1", len(layers))
	}
	if layers[0].Indexed == nil {
		t.Fatal("layer reached an index-capable backend already expanded to RGBA")
	}
	if layers[0].Buffer != nil {
		t.Fatal("indexed layer also carried an RGBA buffer, so the CPU expansion still ran")
	}
	if !bytes.Equal(layers[0].Indexed.Indices, src.indices) {
		t.Fatal("layer indices do not match the source frame")
	}
	if layers[0].Indexed.Palette != src.palette {
		t.Fatal("layer palette does not match the source palette")
	}
	if src.indexedCalls() == 0 {
		t.Fatal("the source was never asked for indexed data")
	}
}

// TestIndexedLayer_FallsBackToRGBA covers every reason the indexed path must
// not engage: the kill switch, an output that cannot expand indices, and a
// source that is not in an indexed mode.
func TestIndexedLayer_FallsBackToRGBA(t *testing.T) {
	cases := []struct {
		name    string
		switchV string
		accepts bool
		offers  bool
	}{
		{"kill switch", "0", true, true},
		{"output cannot expand", "1", false, true},
		{"source not indexed", "1", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IE_VIDEO_GPU_CONVERT", tc.switchV)
			out := newIndexedMockOutput(t, tc.accepts)
			comp := NewVideoCompositor(out)
			comp.LockResolution(32, 32)
			src := newIndexedTestSource(32, 32, tc.offers, 9)
			comp.RegisterSource(src)

			comp.composite()

			layers := out.takeLayers()
			if len(layers) != 1 {
				t.Fatalf("output received %d layers, want 1", len(layers))
			}
			if layers[0].Indexed != nil {
				t.Fatal("an indexed layer was produced where the RGBA path was required")
			}
			want := make([]byte, 32*32*BYTES_PER_PIXEL)
			clut8ExpandSpanScalar(want, src.indices, &src.palette)
			if !bytes.Equal(layers[0].Buffer[:len(want)], want) {
				t.Fatal("RGBA fallback layer does not match the CPU expansion")
			}
		})
	}
}

// TestIndexedLayer_ExpansionMatchesCPUConverter pins that anything on the CPU
// side of the compositor sees exactly what the CPU converter would have
// produced, which is what makes the hardware fallback safe.
func TestIndexedLayer_ExpansionMatchesCPUConverter(t *testing.T) {
	src := newIndexedTestSource(37, 11, true, 21)
	layers := []CompositorFrameLayer{{
		SourceWidth:  37,
		SourceHeight: 11,
		Indexed:      &IndexedLayerData{Indices: src.indices, Palette: src.palette},
	}}
	comp := NewVideoCompositor(nil)
	comp.materialiseIndexedLayers(layers)

	if layers[0].Buffer == nil {
		t.Fatal("indexed layer was not expanded")
	}
	want := make([]byte, 37*11*BYTES_PER_PIXEL)
	clut8ExpandSpanScalar(want, src.indices, &src.palette)
	if !bytes.Equal(layers[0].Buffer, want) {
		t.Fatal("expansion differs from the CPU converter")
	}

	// Expanding again must not overwrite an existing buffer.
	first := &layers[0].Buffer[0]
	comp.materialiseIndexedLayers(layers)
	if &layers[0].Buffer[0] != first {
		t.Fatal("a second expansion replaced the buffer")
	}
}

// TestIndexedLayer_SoftwareRenderExpandsFirst pins the choke point: a software
// render of indexed layers has to produce the same frame as the RGBA path,
// because that is the hardware fallback.
func TestIndexedLayer_SoftwareRenderExpandsFirst(t *testing.T) {
	const w, h = 40, 24
	src := newIndexedTestSource(w, h, true, 33)
	rgba := make([]byte, w*h*BYTES_PER_PIXEL)
	clut8ExpandSpanScalar(rgba, src.indices, &src.palette)

	fromIndexed := renderOneLayer(t, CompositorFrameLayer{
		SourceWidth:  w,
		SourceHeight: h,
		DestWidth:    w,
		DestHeight:   h,
		Opaque:       true,
		Indexed:      &IndexedLayerData{Indices: src.indices, Palette: src.palette},
	}, w, h)
	fromRGBA := renderOneLayer(t, CompositorFrameLayer{
		SourceWidth:  w,
		SourceHeight: h,
		DestWidth:    w,
		DestHeight:   h,
		Opaque:       true,
		Buffer:       rgba,
	}, w, h)

	if !bytes.Equal(fromIndexed, fromRGBA) {
		t.Fatal("software render of an indexed layer differs from the RGBA layer")
	}
}

func renderOneLayer(t *testing.T, layer CompositorFrameLayer, w, h int) []byte {
	t.Helper()
	comp := NewVideoCompositor(nil)
	comp.LockResolution(w, h)
	comp.renderLayersSoftwareLocked([]CompositorFrameLayer{layer}, 1)
	return append([]byte(nil), comp.finalFrame...)
}

// opaqueZeroAlphaSource publishes a frame whose pixels carry no alpha at all,
// which is what a guest framebuffer normally looks like, and declares itself
// opaque.
type opaqueZeroAlphaSource struct {
	mockOpaqueSource
	w, h          int
	frame         []byte
	declareOpaque bool
}

func newOpaqueZeroAlphaSource(w, h int, seed int64) *opaqueZeroAlphaSource {
	r := rand.New(rand.NewSource(seed))
	s := &opaqueZeroAlphaSource{w: w, h: h, declareOpaque: true, frame: make([]byte, w*h*BYTES_PER_PIXEL)}
	for i := 0; i < len(s.frame); i += BYTES_PER_PIXEL {
		s.frame[i+0] = byte(r.Intn(256))
		s.frame[i+1] = byte(r.Intn(256))
		s.frame[i+2] = byte(r.Intn(256))
		s.frame[i+3] = 0 // no alpha, as guest framebuffers write
	}
	// A run of fully zero pixels, the case normalisation deliberately left alone.
	for i := range 16 * BYTES_PER_PIXEL {
		s.frame[i] = 0
	}
	s.enabled.Store(true)
	return s
}

func (s *opaqueZeroAlphaSource) GetDimensions() (int, int) { return s.w, s.h }
func (s *opaqueZeroAlphaSource) GetFrame() []byte          { return nil }
func (s *opaqueZeroAlphaSource) IsOpaqueFrame() bool       { return s.declareOpaque }
func (s *opaqueZeroAlphaSource) CopyFrameForCompositor(dst []byte) ([]byte, bool) {
	if len(dst) < len(s.frame) {
		return nil, false
	}
	copy(dst, s.frame)
	return dst[:len(s.frame)], true
}

// TestOpaqueLayer_SkipsNormalisationWithSameOutput is the gate on dropping the
// whole-frame alpha pass for opaque layers. The composited frame must be
// byte-identical to the same frame normalised first, because every consumer of
// an opaque layer forces alpha itself.
func TestOpaqueLayer_SkipsNormalisationWithSameOutput(t *testing.T) {
	const w, h = 64, 48
	src := newOpaqueZeroAlphaSource(w, h, 4242)

	// Collect through the compositor, so the layer really did skip
	// normalisation, then render those layers.
	comp := NewVideoCompositor(nil)
	comp.LockResolution(w, h)
	comp.RegisterSource(src)
	layers, ok := comp.collectCompositeLayers(true)
	if !ok || len(layers) != 1 {
		t.Fatalf("collect produced %d layers, hasContent=%v", len(layers), ok)
	}
	defer releaseFrameLayerLeases(layers)
	if !layers[0].Opaque {
		t.Fatal("the source declares itself opaque but the layer does not")
	}
	if layers[0].Buffer[3] != 0 {
		t.Fatal("the layer buffer was normalised, so the pass was not skipped")
	}
	comp.renderLayersSoftwareLocked(layers, 1)
	got := append([]byte(nil), comp.finalFrame...)

	// The reference: normalise exactly as the compositor used to, then
	// composite the same frame as a plain buffer layer.
	normalised := append([]byte(nil), src.frame...)
	normaliseFrameLeaseAlphaRGBA(normalised)
	ref := NewVideoCompositor(nil)
	ref.LockResolution(w, h)
	ref.renderLayersSoftwareLocked([]CompositorFrameLayer{{
		SourceWidth: w, SourceHeight: h, DestWidth: w, DestHeight: h,
		Opaque: true, Buffer: normalised,
	}}, 1)
	want := append([]byte(nil), ref.finalFrame...)

	if !bytes.Equal(got, want) {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("byte %d (pixel %d): unnormalised %#02x, normalised %#02x",
					i, i/BYTES_PER_PIXEL, got[i], want[i])
			}
		}
	}
	// Every pixel must be opaque in the composed frame, including the zero run.
	for i := 3; i < len(got); i += BYTES_PER_PIXEL {
		if got[i] != 0xFF {
			t.Fatalf("composed pixel %d has alpha %#02x, want opaque", i/BYTES_PER_PIXEL, got[i])
		}
	}
}

// TestOpaqueLayer_NonOpaqueStillNormalised pins that the saving is scoped: a
// source that does not declare itself opaque still gets its alpha promoted,
// since the blend path relies on it.
func TestOpaqueLayer_NonOpaqueStillNormalised(t *testing.T) {
	const w, h = 32, 16
	src := newOpaqueZeroAlphaSource(w, h, 77)
	src.declareOpaque = false

	comp := NewVideoCompositor(nil)
	comp.LockResolution(w, h)
	comp.RegisterSource(src)
	layers, ok := comp.collectCompositeLayers(true)
	if !ok || len(layers) != 1 {
		t.Fatalf("collect produced %d layers, hasContent=%v", len(layers), ok)
	}
	defer releaseFrameLayerLeases(layers)
	if layers[0].Opaque {
		t.Fatal("the source declared itself non-opaque but the layer says otherwise")
	}
	// A nonzero pixel with no alpha must have been promoted.
	buf := layers[0].Buffer
	for i := 0; i < len(buf); i += BYTES_PER_PIXEL {
		if buf[i] != 0 || buf[i+1] != 0 || buf[i+2] != 0 {
			if buf[i+3] != 0xFF {
				t.Fatalf("non-opaque layer pixel %d kept alpha %#02x, so normalisation was skipped",
					i/BYTES_PER_PIXEL, buf[i+3])
			}
			return
		}
	}
}
