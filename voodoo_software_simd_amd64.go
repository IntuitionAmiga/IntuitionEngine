//go:build amd64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

// rasterizeRowsSIMD is the 8-wide bit-exact rasteriser for the SIMD-eligible
// setup class (voodooSetupSIMDEligible): slope-register affine interpolation,
// RGB write, optional depth test/write and fog, no texture / blend / alpha test
// / chroma / dither / stipple. Every float op mirrors the scalar order in
// rasterizeRows (no FMA); clamp uses compare-blend; the uint32(ch*255) pack
// truncates via CVTTPS2DQ and squashes NaN lanes to zero to match Go's scalar
// conversion. Bands own disjoint rows so the in-row RMW masked writes are safe.
func rasterizeRowsSIMD(b *VoodooSoftwareBackend, s *voodooTriangleSetup, minY, maxY int) {
	v0, v1, v2 := s.v0, s.v1, s.v2

	bc := archsimd.BroadcastFloat32x8
	laneIdx := archsimd.LoadFloat32x8(&[8]float32{0, 1, 2, 3, 4, 5, 6, 7})
	zero := bc(0)
	one := bc(1)
	half := bc(0.5)
	c255 := bc(255)
	trueMask := zero.GreaterEqual(zero) // all lanes true (0 >= 0)

	e0v, e1v, e2v := bc(s.e0), bc(s.e1), bc(s.e2)
	v0Xv, v1Xv, v2Xv := bc(v0.X), bc(v1.X), bc(v2.X)
	drdxv, drdyv := bc(s.drdx), bc(s.drdy)
	dgdxv, dgdyv := bc(s.dgdx), bc(s.dgdy)
	dbdxv, dbdyv := bc(s.dbdx), bc(s.dbdy)
	dadxv, dadyv := bc(s.dadx), bc(s.dady)
	dzdxv, dzdyv := bc(s.dzdx), bc(s.dzdy)
	v0Rv, v0Gv, v0Bv := bc(v0.R), bc(v0.G), bc(v0.B)
	v0Av, v0Zv := bc(v0.A), bc(v0.Z)
	fogRv, fogGv, fogBv := bc(s.fogR), bc(s.fogG), bc(s.fogB)
	maxXf := bc(float32(s.maxX))

	// Reinterpret each target once at its base as a []uint32 (checkptr-safe:
	// no interior byte pointer conversions). Inner loop slices these views.
	targetsU := make([][]uint32, len(s.targets))
	for i, t := range s.targets {
		targetsU[i] = unsafe.Slice((*uint32)(unsafe.Pointer(&t[0])), len(t)/4)
	}

	clamp01 := func(x archsimd.Float32x8) archsimd.Float32x8 {
		x = zero.Merge(x, x.Less(zero)) // where x<0 -> 0
		x = one.Merge(x, x.Greater(one))
		return x
	}
	packCh := func(ch archsimd.Float32x8, shift uint64) archsimd.Uint32x8 {
		prod := ch.Mul(c255)
		i := prod.ConvertToInt32().AsUint32x8()
		i = i.Masked(prod.Equal(prod)) // NaN lanes -> 0 (Go uint32(NaN)==0)
		if shift != 0 {
			i = i.ShiftAllLeft(shift)
		}
		return i
	}
	depthCompare := func(z, oldZ archsimd.Float32x8) archsimd.Mask32x8 {
		switch s.depthFunc {
		case VOODOO_DEPTH_NEVER:
			return zero.Greater(zero) // all false
		case VOODOO_DEPTH_LESS:
			return z.Less(oldZ)
		case VOODOO_DEPTH_EQUAL:
			return z.Equal(oldZ)
		case VOODOO_DEPTH_LESSEQUAL:
			return z.LessEqual(oldZ)
		case VOODOO_DEPTH_GREATER:
			return z.Greater(oldZ)
		case VOODOO_DEPTH_NOTEQUAL:
			return z.NotEqual(oldZ)
		case VOODOO_DEPTH_GREATEREQUAL:
			return z.GreaterEqual(oldZ)
		default: // VOODOO_DEPTH_ALWAYS and fallthrough
			return trueMask
		}
	}
	alphaRefV := bc(s.alphaTestRef)
	// alphaTestCompare mirrors b.alphaTest on the clamped alpha lane, returning
	// the pass mask (lanes that survive the test).
	alphaTestCompare := func(a archsimd.Float32x8) archsimd.Mask32x8 {
		switch s.alphaTestFunc {
		case VOODOO_ALPHA_NEVER:
			return zero.Greater(zero)
		case VOODOO_ALPHA_LESS:
			return a.Less(alphaRefV)
		case VOODOO_ALPHA_EQUAL:
			return a.Equal(alphaRefV)
		case VOODOO_ALPHA_LESSEQUAL:
			return a.LessEqual(alphaRefV)
		case VOODOO_ALPHA_GREATER:
			return a.Greater(alphaRefV)
		case VOODOO_ALPHA_NOTEQUAL:
			return a.NotEqual(alphaRefV)
		case VOODOO_ALPHA_GREATEREQUAL:
			return a.GreaterEqual(alphaRefV)
		default: // VOODOO_ALPHA_ALWAYS and fallthrough
			return trueMask
		}
	}

	for y := minY; y < maxY; y++ {
		py := float32(y) + 0.5
		t0v := bc((py - v1.Y) * s.f0)
		t1v := bc((py - v2.Y) * s.f1)
		t2v := bc((py - v0.Y) * s.f2)
		dyScalar := py - v0.Y
		dyv := bc(dyScalar)

		dstY := y
		if s.yFlip {
			dstY = b.height - 1 - y
		}
		rowBase := dstY * b.width

		for xc := s.minX; xc < s.maxX; xc += 8 {
			sliceLen := 8
			if rem := b.width - xc; rem < sliceLen {
				sliceLen = rem
			}
			if sliceLen <= 0 {
				break
			}
			xcv := bc(float32(xc))
			laneX := xcv.Add(laneIdx)
			pxv := laneX.Add(half)

			w0 := pxv.Sub(v1Xv).Mul(e0v).Sub(t0v)
			w1 := pxv.Sub(v2Xv).Mul(e1v).Sub(t1v)
			w2 := pxv.Sub(v0Xv).Mul(e2v).Sub(t2v)
			inside := w0.GreaterEqual(zero).And(w1.GreaterEqual(zero)).And(w2.GreaterEqual(zero))
			writeMask := inside.And(laneX.Less(maxXf))

			// Stipple discards lanes whose pattern bit is clear (a stipple of 0
			// allows all, matching stippleAllowsVoodooPixel). Bit index per lane:
			// (y&3)*8 + (x&7). Only gates the write, like every other discard.
			if s.stippleEnable && s.stipple != 0 {
				rowBits := (y & 3) * 8
				var bits uint8
				for l := 0; l < simdF32Lanes; l++ {
					bit := uint(rowBits + ((xc + l) & 7))
					if (s.stipple>>bit)&1 != 0 {
						bits |= 1 << uint(l)
					}
				}
				writeMask = writeMask.And(archsimd.Mask32x8FromBits(bits))
			}

			// The scalar reference `v0 + dx*ddx + dy*ddy` is fused by the gc
			// compiler into nested FMA under GOAMD64 v3 (the make build). Match
			// that exactly with MulAdd so the SIMD depth and colour are bit-
			// identical to the shipped scalar reference.
			dxv := pxv.Sub(v0Xv)
			r := dyv.MulAdd(drdyv, v0Rv.Add(dxv.Mul(drdxv)))
			g := dyv.MulAdd(dgdyv, v0Gv.Add(dxv.Mul(dgdxv)))
			bb := dyv.MulAdd(dbdyv, v0Bv.Add(dxv.Mul(dbdxv)))
			a := dyv.MulAdd(dadyv, v0Av.Add(dxv.Mul(dadxv)))
			z := dyv.MulAdd(dzdyv, v0Zv.Add(dxv.Mul(dzdxv)))

			wordBase := rowBase + xc
			// Only complete 8-pixel chunks fully inside the row use full 8-lane
			// vector loads/stores. Any partial chunk (row end or buffer end) uses
			// a scalar gather/scatter over the SAME vector-computed values. This
			// avoids SlicePart entirely: a short slice would make SlicePart
			// address a full 8-lane vector that straddles into the next row (a
			// checkptr straddle at the buffer end, and a cross-band read/write
			// under row-band parallelism). Full chunks touch only in-row memory.
			partial := sliceLen < simdF32Lanes

			var oldZ archsimd.Float32x8
			if s.depthEnable {
				if partial {
					var ob [8]float32
					for l := 0; l < sliceLen; l++ {
						ob[l] = b.depthBuffer[wordBase+l]
					}
					oldZ = archsimd.LoadFloat32x8(&ob)
				} else {
					oldZ = archsimd.LoadFloat32x8Slice(b.depthBuffer[wordBase : wordBase+simdF32Lanes])
				}
				writeMask = writeMask.And(depthCompare(z, oldZ))
			}

			// No lane survives to this chunk: skip the remaining per-pixel work
			// (texture sample, clamp, fog, dither, pack, store). Bit-identical to
			// writing nothing.
			if writeMask.ToBits() == 0 {
				continue
			}

			// Texture: hybrid stage. Texel fetch (no SIMD gather in archsimd 1.26)
			// and combineVoodooColors run scalar per lane on the pre-clamp
			// interpolated r,g,b,a; texture coordinates use the exact scalar slope
			// expression so gc fuses them identically. r,g,b,a are replaced with
			// the combined result, then the vector pipeline resumes.
			if s.texActive {
				var ra, ga, bba, aa [8]float32
				r.Store(&ra)
				g.Store(&ga)
				bb.Store(&bba)
				a.Store(&aa)
				wmBits := writeMask.ToBits()
				for l := 0; l < simdF32Lanes; l++ {
					if wmBits&(1<<uint(l)) == 0 {
						continue // skip texel sample for lanes that will not be written
					}
					px := float32(xc+l) + 0.5
					dx := px - v0.X
					sTex := v0.S + dx*s.dsdx + dyScalar*s.dsdy
					tTex := v0.T + dx*s.dtdx + dyScalar*s.dtdy
					texR, texG, texB, texA := sampleVoodooTexel(s.texData, s.texWidth, s.texHeight, s.texClampS, s.texClampT, sTex, tTex)
					ra[l], ga[l], bba[l], aa[l] = combineVoodooColors(s.fbzColorPath, s.colorPathSet, ra[l], ga[l], bba[l], aa[l], texR, texG, texB, texA)
				}
				r = archsimd.LoadFloat32x8(&ra)
				g = archsimd.LoadFloat32x8(&ga)
				bb = archsimd.LoadFloat32x8(&bba)
				a = archsimd.LoadFloat32x8(&aa)
			}

			r = clamp01(r)
			g = clamp01(g)
			bb = clamp01(bb)
			a = clamp01(a)

			// Alpha test discards failing lanes (on the clamped alpha, before
			// forceOpaqueAlpha), matching the scalar discard order.
			if s.alphaTestEnable {
				writeMask = writeMask.And(alphaTestCompare(a))
			}

			// Chroma key discards lanes matching the key. It quantises through
			// int(clampf*255+0.5) and abs32 with two code paths, so it is a scalar
			// hybrid over the clamped pre-fog r,g,b: bit-exact by construction.
			if s.chromaKeyEnable {
				var ra, ga, ba [8]float32
				r.Store(&ra)
				g.Store(&ga)
				bb.Store(&ba)
				var keep uint8
				for l := 0; l < simdF32Lanes; l++ {
					if !voodooChromaTest(s.chromaKey, s.chromaRange, ra[l], ga[l], ba[l]) {
						keep |= 1 << uint(l)
					}
				}
				writeMask = writeMask.And(archsimd.Mask32x8FromBits(keep))
			}

			if s.fogEnable {
				ff := clamp01(z)
				om := one.Sub(ff)
				// Scalar `r*(1-ff) + fogR*ff` is fused by gc; the addend r*(1-ff)
				// is computed first, so the outer add fuses with fogR*ff.
				r = clamp01(r.MulAdd(om, fogRv.Mul(ff)))
				g = clamp01(g.MulAdd(om, fogGv.Mul(ff)))
				bb = clamp01(bb.MulAdd(om, fogBv.Mul(ff)))
			}
			// Dither quantises r,g,b (not a) after fog. applyDither has an int
			// round-trip whose gc FMA fusion is not reliably matchable in lanes,
			// so this stage is a scalar hybrid: it calls the exact scalar
			// applyDither per lane and reloads, guaranteeing bit-exactness.
			if s.ditherEnable {
				var ra, ga, ba [8]float32
				r.Store(&ra)
				g.Store(&ga)
				bb.Store(&ba)
				for l := 0; l < simdF32Lanes; l++ {
					th := b.getDitherThreshold(xc+l, y, s.dither2x2)
					ra[l] = b.applyDither(ra[l], th)
					ga[l] = b.applyDither(ga[l], th)
					ba[l] = b.applyDither(ba[l], th)
				}
				r = archsimd.LoadFloat32x8(&ra)
				g = archsimd.LoadFloat32x8(&ga)
				bb = archsimd.LoadFloat32x8(&ba)
			}

			if s.forceOpaqueAlpha {
				a = one
			}

			packed := packCh(r, 0).Or(packCh(g, 8)).Or(packCh(bb, 16)).Or(packCh(a, 24))

			if partial {
				var pk [8]uint32
				var wm [8]int32
				var zz [8]float32
				packed.Store(&pk)
				writeMask.ToInt32x8().Store(&wm)
				z.Store(&zz)
				for l := 0; l < sliceLen; l++ {
					if wm[l] == 0 {
						continue
					}
					for _, tu := range targetsU {
						tu[wordBase+l] = pk[l]
					}
					if s.depthEnable && s.depthWrite {
						b.depthBuffer[wordBase+l] = zz[l]
					}
				}
				continue
			}

			for _, tu := range targetsU {
				du := tu[wordBase : wordBase+simdF32Lanes]
				d := archsimd.LoadUint32x8Slice(du)
				packed.Merge(d, writeMask).StoreSlice(du)
			}

			if s.depthEnable && s.depthWrite {
				dslice := b.depthBuffer[wordBase : wordBase+simdF32Lanes]
				z.Merge(oldZ, writeMask).StoreSlice(dslice)
			}
		}
	}
}
