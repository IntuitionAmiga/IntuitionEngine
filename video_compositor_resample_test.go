// video_compositor_resample_test.go - scaled resample gather tests.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"
)

func resampleTestRow(w int, rng *rand.Rand) []byte {
	row := make([]byte, w*BYTES_PER_PIXEL)
	for i := range row {
		row[i] = byte(rng.Intn(256))
	}
	return row
}

// resampleNaiveRow is the definition the plan must reproduce: destination
// column dx samples source column dx*srcW/rectW.
func resampleNaiveRow(dst, srcRow []byte, srcW, rectW, colBase, cols int) {
	for i := range cols {
		dx := colBase + i
		src := (dx * srcW / rectW) * BYTES_PER_PIXEL
		copy(dst[i*BYTES_PER_PIXEL:(i+1)*BYTES_PER_PIXEL], srcRow[src:src+BYTES_PER_PIXEL])
	}
}

func TestResamplePlan_MatchesNaiveMapping(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	widths := []int{1, 3, 7, 8, 9, 16, 31, 64, 100, 320, 640}
	for _, srcW := range widths {
		for _, rectW := range widths {
			plan := newResamplePlan(srcW, rectW)
			srcRow := resampleTestRow(srcW, rng)
			got := make([]byte, rectW*BYTES_PER_PIXEL)
			want := make([]byte, rectW*BYTES_PER_PIXEL)
			compositorResampleRowScalar(got, srcRow, plan, 0, rectW)
			resampleNaiveRow(want, srcRow, srcW, rectW, 0, rectW)
			if !bytes.Equal(got, want) {
				t.Fatalf("srcW=%d rectW=%d: plan gather differs from the naive mapping", srcW, rectW)
			}
		}
	}
}

func TestResampleGatherMatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	widths := []int{1, 5, 8, 15, 16, 33, 64, 65, 128, 256, 321, 640, 800}
	for _, srcW := range widths {
		for _, rectW := range widths {
			plan := newResamplePlan(srcW, rectW)
			srcRow := resampleTestRow(srcW, rng)
			// Whole row, then every aligned partial window a tile can ask
			// for, which is how the tile path calls the kernel.
			spans := [][2]int{{0, rectW}}
			for base := 0; base < rectW; base += compositorTileSize {
				cols := min(compositorTileSize, rectW-base)
				spans = append(spans, [2]int{base, cols})
			}
			for _, span := range spans {
				colBase, cols := span[0], span[1]
				got := make([]byte, cols*BYTES_PER_PIXEL)
				want := make([]byte, cols*BYTES_PER_PIXEL)
				compositorResampleRowImpl(got, srcRow, plan, colBase, cols)
				compositorResampleRowScalar(want, srcRow, plan, colBase, cols)
				if !bytes.Equal(got, want) {
					t.Fatalf("srcW=%d rectW=%d colBase=%d cols=%d: dispatched gather differs from the scalar leaf",
						srcW, rectW, colBase, cols)
				}
			}
		}
	}
}

func TestResamplePlan_VectorPlanRejectsNarrowingScales(t *testing.T) {
	// Narrowing by more than one source column per destination column cannot
	// be served by a single eight-pixel load, so the vector plan must decline
	// and the scalar leaf must still be correct.
	plan := newResamplePlan(640, 64)
	if plan.vectorOK {
		t.Fatal("vector plan accepted a 10:1 narrowing scale")
	}
	rng := rand.New(rand.NewSource(13))
	srcRow := resampleTestRow(640, rng)
	got := make([]byte, 64*BYTES_PER_PIXEL)
	want := make([]byte, 64*BYTES_PER_PIXEL)
	compositorResampleRowImpl(got, srcRow, plan, 0, 64)
	resampleNaiveRow(want, srcRow, 640, 64, 0, 64)
	if !bytes.Equal(got, want) {
		t.Fatal("narrowing gather differs from the naive mapping")
	}
}

func TestResamplePlan_UpscaleUsesVectorPlan(t *testing.T) {
	plan := newResamplePlan(320, 640)
	if !plan.vectorOK {
		t.Fatal("vector plan declined a 1:2 upscale")
	}
	for b := range len(plan.blockBase) {
		if plan.blockBase[b] < 0 {
			t.Fatalf("block %d of a 1:2 upscale was not vectorised", b)
		}
	}
}

func TestResamplePlan_FirstAndLastPixelsExact(t *testing.T) {
	// Edge columns are where an off-by-one in the block base would show.
	rng := rand.New(rand.NewSource(17))
	for _, pair := range [][2]int{{320, 640}, {640, 641}, {7, 8}, {8, 7}, {800, 1024}} {
		srcW, rectW := pair[0], pair[1]
		plan := newResamplePlan(srcW, rectW)
		srcRow := resampleTestRow(srcW, rng)
		got := make([]byte, rectW*BYTES_PER_PIXEL)
		compositorResampleRowImpl(got, srcRow, plan, 0, rectW)
		first := binary.LittleEndian.Uint32(got)
		last := binary.LittleEndian.Uint32(got[(rectW-1)*BYTES_PER_PIXEL:])
		wantFirst := binary.LittleEndian.Uint32(srcRow)
		wantLast := binary.LittleEndian.Uint32(srcRow[((rectW-1)*srcW/rectW)*BYTES_PER_PIXEL:])
		if first != wantFirst || last != wantLast {
			t.Fatalf("srcW=%d rectW=%d: edge pixels wrong", srcW, rectW)
		}
	}
}

func BenchmarkCompositeScaled_Resample(b *testing.B) {
	const srcW, rectW = 320, 640
	rng := rand.New(rand.NewSource(23))
	plan := newResamplePlan(srcW, rectW)
	srcRow := resampleTestRow(srcW, rng)
	dst := make([]byte, rectW*BYTES_PER_PIXEL)
	b.SetBytes(int64(rectW * BYTES_PER_PIXEL))
	b.ResetTimer()
	for range b.N {
		compositorResampleRowImpl(dst, srcRow, plan, 0, rectW)
	}
}

func BenchmarkCompositeScaled_ResampleScalar(b *testing.B) {
	const srcW, rectW = 320, 640
	rng := rand.New(rand.NewSource(23))
	plan := newResamplePlan(srcW, rectW)
	srcRow := resampleTestRow(srcW, rng)
	dst := make([]byte, rectW*BYTES_PER_PIXEL)
	b.SetBytes(int64(rectW * BYTES_PER_PIXEL))
	b.ResetTimer()
	for range b.N {
		compositorResampleRowScalar(dst, srcRow, plan, 0, rectW)
	}
}

func BenchmarkCompositeScaled_ResampleWide(b *testing.B) {
	const srcW, rectW = 640, 1920
	rng := rand.New(rand.NewSource(29))
	plan := newResamplePlan(srcW, rectW)
	srcRow := resampleTestRow(srcW, rng)
	dst := make([]byte, rectW*BYTES_PER_PIXEL)
	b.SetBytes(int64(rectW * BYTES_PER_PIXEL))
	b.ResetTimer()
	for range b.N {
		compositorResampleRowImpl(dst, srcRow, plan, 0, rectW)
	}
}

func BenchmarkCompositeScaled_ResampleWideScalar(b *testing.B) {
	const srcW, rectW = 640, 1920
	rng := rand.New(rand.NewSource(29))
	plan := newResamplePlan(srcW, rectW)
	srcRow := resampleTestRow(srcW, rng)
	dst := make([]byte, rectW*BYTES_PER_PIXEL)
	b.SetBytes(int64(rectW * BYTES_PER_PIXEL))
	b.ResetTimer()
	for range b.N {
		compositorResampleRowScalar(dst, srcRow, plan, 0, rectW)
	}
}
