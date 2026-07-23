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
