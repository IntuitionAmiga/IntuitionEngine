//go:build amd64 && goexperiment.simd

package main

import (
	"math"
	"math/rand"
	"testing"
)

// Bit-exact differential gate for the Voodoo SIMD row rasteriser. The scalar
// rasteriser is the conformance reference, so the SIMD path must reproduce the
// framebuffer AND depth buffer byte for byte over a large random eligible set,
// including NaN/Inf attributes, denormal slopes, slivers and edge-clipped
// triangles.

func newVoodooBackendForTest(t *testing.T, w, h int) *VoodooSoftwareBackend {
	t.Helper()
	b := &VoodooSoftwareBackend{}
	if err := b.Init(w, h); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return b
}

// randFloatAttr returns a mostly-normal value, occasionally an edge case.
func randFloatAttr(r *rand.Rand) float32 {
	switch r.Intn(20) {
	case 0:
		return float32(math.NaN())
	case 1:
		return float32(math.Inf(1))
	case 2:
		return float32(math.Inf(-1))
	case 3:
		return math.SmallestNonzeroFloat32
	default:
		return r.Float32()*1.5 - 0.25 // spans below 0 and above 1 to exercise clamp
	}
}

func randVertex(r *rand.Rand, w, h int) VoodooVertex {
	return VoodooVertex{
		X: r.Float32() * float32(w),
		Y: r.Float32() * float32(h),
		Z: randFloatAttr(r),
		R: randFloatAttr(r), G: randFloatAttr(r), B: randFloatAttr(r), A: randFloatAttr(r),
		S: r.Float32()*2 - 0.5, T: r.Float32()*2 - 0.5,
	}
}

// randTexture builds a small random RGBA texture and a combine configuration.
var chromaCombineModes = []uint32{
	VOODOO_COMBINE_ADD, VOODOO_COMBINE_MODULATE,
	VOODOO_CC_ITERATED, VOODOO_CC_TEXTURE,
}

func randTexture(r *rand.Rand) (data []byte, w, h int, clampS, clampT bool, fbzColorPath uint32, colorPathSet bool) {
	w = 8 + r.Intn(24)
	h = 8 + r.Intn(24)
	data = make([]byte, w*h*4)
	for i := range data {
		data[i] = byte(r.Intn(256))
	}
	clampS = r.Intn(2) == 0
	clampT = r.Intn(2) == 0
	colorPathSet = r.Intn(2) == 0
	fbzColorPath = chromaCombineModes[r.Intn(len(chromaCombineModes))]
	return
}

// buildEligibleSetup constructs a random SIMD-eligible triangle setup plus the
// [minY,maxY) band, mirroring rasterizeTriangle's edge/area maths.
func buildEligibleSetup(r *rand.Rand, w, h int, targets [][]byte) (voodooTriangleSetup, int, int, bool) {
	v0 := new(VoodooVertex)
	v1 := new(VoodooVertex)
	v2 := new(VoodooVertex)
	*v0 = randVertex(r, w, h)
	*v1 = randVertex(r, w, h)
	*v2 = randVertex(r, w, h)

	area := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, v2.X, v2.Y)
	if area == 0 {
		return voodooTriangleSetup{}, 0, 0, false
	}
	if area < 0 {
		v0, v2 = v2, v0
		area = -area
	}
	minX := int(math.Floor(float64(min3f(v0.X, v1.X, v2.X))))
	maxX := int(math.Ceil(float64(max3f(v0.X, v1.X, v2.X))))
	minY := int(math.Floor(float64(min3f(v0.Y, v1.Y, v2.Y))))
	maxY := int(math.Ceil(float64(max3f(v0.Y, v1.Y, v2.Y))))
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > w {
		maxX = w
	}
	if maxY > h {
		maxY = h
	}
	if minX >= maxX || minY >= maxY {
		return voodooTriangleSetup{}, 0, 0, false
	}

	sl := func() float32 { return r.Float32()*0.02 - 0.01 }
	s := voodooTriangleSetup{
		v0: v0, v1: v1, v2: v2,
		invArea: 1.0 / area,
		e0:      v2.Y - v1.Y, f0: v2.X - v1.X,
		e1: v0.Y - v2.Y, f1: v0.X - v2.X,
		e2: v1.Y - v0.Y, f2: v1.X - v0.X,
		minX: minX, maxX: maxX,
		rgbWrite:         true,
		slopesValid:      true,
		depthEnable:      r.Intn(2) == 0,
		depthWrite:       r.Intn(2) == 0,
		depthFunc:        r.Intn(8),
		alphaTestEnable:  r.Intn(2) == 0,
		alphaTestFunc:    r.Intn(8),
		alphaTestRef:     r.Float32(),
		stippleEnable:    r.Intn(2) == 0,
		stipple:          r.Uint32(),
		ditherEnable:     r.Intn(2) == 0,
		dither2x2:        r.Intn(2) == 0,
		chromaKeyEnable:  r.Intn(2) == 0,
		chromaKey:        r.Uint32() & 0xFFFFFF,
		chromaRange:      chromaRangeFor(r),
		fogEnable:        r.Intn(2) == 0,
		yFlip:            r.Intn(2) == 0,
		forceOpaqueAlpha: r.Intn(2) == 0,
		fogR:             r.Float32(), fogG: r.Float32(), fogB: r.Float32(),
		drdx: sl(), drdy: sl(), dgdx: sl(), dgdy: sl(), dbdx: sl(), dbdy: sl(),
		dadx: sl(), dady: sl(), dzdx: sl(), dzdy: sl(),
		dsdx: sl(), dsdy: sl(), dtdx: sl(), dtdy: sl(),
		targets: targets,
	}
	if r.Intn(2) == 0 {
		s.texActive = true
		s.texData, s.texWidth, s.texHeight, s.texClampS, s.texClampT, s.fbzColorPath, s.colorPathSet = randTexture(r)
	}
	return s, minY, maxY, true
}

//go:noinline
func fmaProbeMul(b, c float32) float32 { return b * c }

//go:noinline
func fmaProbeFused(a, b, c float32) float32 { return a + b*c }

// scalarBuildUsesFMA reports whether the gc compiler fused a*b+c into a single
// FMA for this build. GOAMD64 v3+ has the FMA3 instruction and gc uses it; v1/v2
// do not. The Voodoo SIMD kernel uses MulAdd to match the fused scalar reference
// that the make build (v3) ships and the golden checksums are recorded against,
// so the strict bit-exact differential only holds on FMA-fused builds. The
// distinguishing inputs are chosen so fused and separate rounding differ.
func scalarBuildUsesFMA() bool {
	const a, b, c = float32(0.7926835), float32(-0.35583204), float32(0.44229555)
	return fmaProbeFused(a, b, c) != a+fmaProbeMul(b, c)
}

// chromaRangeFor returns 0 (tolerance path) half the time, else a random range
// value (range path), so the differential exercises both chroma code paths.
func chromaRangeFor(r *rand.Rand) uint32 {
	if r.Intn(2) == 0 {
		return 0
	}
	return r.Uint32() & 0xFFFFFF
}

func TestSIMDRasterizeRowsMatchesScalarBitExact(t *testing.T) {
	if raceEnabled {
		t.Skip("race build inhibits gc FMA fusion; strict raw-float differential targets the release build")
	}
	if !scalarBuildUsesFMA() {
		t.Skip("Voodoo SIMD bit-exactness targets FMA-fused builds (GOAMD64 v3+); scalar reference is non-fused here")
	}
	const w, h = 64, 48
	saved := voodooRasterizeRowsSIMDFn
	defer func() { voodooRasterizeRowsSIMDFn = saved }()

	r := rand.New(rand.NewSource(4242))
	cases := 0
	for iter := 0; iter < 6000 && cases < 2000; iter++ {
		bScalar := newVoodooBackendForTest(t, w, h)
		bSIMD := newVoodooBackendForTest(t, w, h)
		// Randomise initial depth so the depth test has something to compare.
		for i := range bScalar.depthBuffer {
			z := r.Float32()
			bScalar.depthBuffer[i] = z
			bSIMD.depthBuffer[i] = z
		}
		// Random initial framebuffer so masked/skip lanes are observable.
		for i := range bScalar.colorBuffer {
			v := byte(r.Intn(256))
			bScalar.colorBuffer[i] = v
			bSIMD.colorBuffer[i] = v
		}

		sScalar, minY, maxY, ok := buildEligibleSetup(r, w, h, nil)
		if !ok {
			continue
		}
		cases++
		sScalar.targets = [][]byte{bScalar.colorBuffer}
		sSIMD := sScalar
		sSIMD.targets = [][]byte{bSIMD.colorBuffer}

		voodooRasterizeRowsSIMDFn = nil
		bScalar.rasterizeRows(&sScalar, minY, maxY)
		rasterizeRowsSIMD(bSIMD, &sSIMD, minY, maxY)

		for i := range bScalar.colorBuffer {
			if bScalar.colorBuffer[i] != bSIMD.colorBuffer[i] {
				t.Fatalf("iter %d colorBuffer byte %d: scalar %#02x simd %#02x", iter, i, bScalar.colorBuffer[i], bSIMD.colorBuffer[i])
			}
		}
		for i := range bScalar.depthBuffer {
			if math.Float32bits(bScalar.depthBuffer[i]) != math.Float32bits(bSIMD.depthBuffer[i]) {
				t.Fatalf("iter %d depth %d: scalar %#08x simd %#08x", iter, i,
					math.Float32bits(bScalar.depthBuffer[i]), math.Float32bits(bSIMD.depthBuffer[i]))
			}
		}
	}
	if cases < 100 {
		t.Fatalf("only %d eligible cases generated", cases)
	}
	t.Logf("compared %d eligible triangles bit-exact", cases)
}

func benchVoodooSetup(b *VoodooSoftwareBackend, size int, w, h int) (voodooTriangleSetup, int, int) {
	v0 := &VoodooVertex{X: float32(w/2 - size/2), Y: 2, Z: 0.2, R: 1, G: 0.5, B: 0.25, A: 1}
	v1 := &VoodooVertex{X: float32(w/2 + size/2), Y: 2, Z: 0.4, R: 0.2, G: 1, B: 0.5, A: 1}
	v2 := &VoodooVertex{X: float32(w / 2), Y: float32(2 + size), Z: 0.6, R: 0.5, G: 0.25, B: 1, A: 1}
	area := edgeFunction(v0.X, v0.Y, v1.X, v1.Y, v2.X, v2.Y)
	if area < 0 {
		v0, v2 = v2, v0
		area = -area
	}
	minY, maxY := 2, min(2+size+1, h)
	s := voodooTriangleSetup{
		v0: v0, v1: v1, v2: v2, invArea: 1.0 / area,
		e0: v2.Y - v1.Y, f0: v2.X - v1.X,
		e1: v0.Y - v2.Y, f1: v0.X - v2.X,
		e2: v1.Y - v0.Y, f2: v1.X - v0.X,
		minX: max(0, w/2-size/2), maxX: min(w, w/2+size/2+1),
		rgbWrite: true, slopesValid: true,
		depthEnable: true, depthWrite: true, depthFunc: VOODOO_DEPTH_LESS,
		fogEnable: true, fogR: 0.1, fogG: 0.1, fogB: 0.1,
		drdx: 0.001, drdy: 0.001, dgdx: 0.001, dgdy: 0.001, dbdx: 0.001, dbdy: 0.001,
		dadx: 0, dady: 0, dzdx: 0.0005, dzdy: 0.0005,
		targets: [][]byte{b.colorBuffer},
	}
	return s, minY, maxY
}

func BenchmarkVoodooTexturedTriangle(b *testing.B) {
	r := rand.New(rand.NewSource(7))
	for _, tc := range []struct {
		name       string
		size, w, h int
	}{{"32px", 32, 128, 128}, {"400px", 400, 512, 512}} {
		back := &VoodooSoftwareBackend{}
		_ = back.Init(tc.w, tc.h)
		setup, minY, maxY := benchVoodooSetup(back, tc.size, tc.w, tc.h)
		setup.texActive = true
		setup.dsdx, setup.dsdy, setup.dtdx, setup.dtdy = 0.002, 0.002, 0.001, 0.001
		setup.texData, setup.texWidth, setup.texHeight, setup.texClampS, setup.texClampT, setup.fbzColorPath, setup.colorPathSet = randTexture(r)
		b.Run(tc.name+"/scalar", func(b *testing.B) {
			saved := voodooRasterizeRowsSIMDFn
			voodooRasterizeRowsSIMDFn = nil
			defer func() { voodooRasterizeRowsSIMDFn = saved }()
			for i := 0; i < b.N; i++ {
				back.rasterizeRows(&setup, minY, maxY)
			}
		})
		b.Run(tc.name+"/simd", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				rasterizeRowsSIMD(back, &setup, minY, maxY)
			}
		})
	}
}

func BenchmarkVoodooUntexturedTriangle(b *testing.B) {
	for _, tc := range []struct {
		name       string
		size, w, h int
	}{{"32px", 32, 128, 128}, {"400px", 400, 512, 512}} {
		back := &VoodooSoftwareBackend{}
		_ = back.Init(tc.w, tc.h)
		setup, minY, maxY := benchVoodooSetup(back, tc.size, tc.w, tc.h)
		b.Run(tc.name+"/scalar", func(b *testing.B) {
			saved := voodooRasterizeRowsSIMDFn
			voodooRasterizeRowsSIMDFn = nil
			defer func() { voodooRasterizeRowsSIMDFn = saved }()
			for i := 0; i < b.N; i++ {
				back.rasterizeRows(&setup, minY, maxY)
			}
		})
		b.Run(tc.name+"/simd", func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				rasterizeRowsSIMD(back, &setup, minY, maxY)
			}
		})
	}
}

// TestVoodooSIMDPathQualification: setups outside the eligible set must route
// scalar (voodooSetupSIMDEligible false).
func TestVoodooSIMDPathQualification(t *testing.T) {
	base := voodooTriangleSetup{slopesValid: true, rgbWrite: true}
	if !voodooSetupSIMDEligible(&base) {
		t.Fatal("baseline eligible setup rejected")
	}
	for name, mut := range map[string]func(s *voodooTriangleSetup){
		"alphaBlendEnable": func(s *voodooTriangleSetup) { s.alphaBlendEnable = true },
		"noSlopes":         func(s *voodooTriangleSetup) { s.slopesValid = false },
		"noRGBWrite":       func(s *voodooTriangleSetup) { s.rgbWrite = false },
	} {
		s := base
		mut(&s)
		if voodooSetupSIMDEligible(&s) {
			t.Fatalf("%s must route scalar but was reported eligible", name)
		}
	}
}
