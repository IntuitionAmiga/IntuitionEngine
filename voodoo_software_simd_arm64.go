//go:build linux && arm64 && goexperiment.simd

package main

import (
	"simd/archsimd"
	"unsafe"
)

func mask32x4FromBits(bits uint8) archsimd.Mask32x4 {
	var lanes [4]uint32
	for i := range lanes {
		if bits&(1<<i) != 0 {
			lanes[i] = ^uint32(0)
		}
	}
	return archsimd.LoadUint32x4Array(&lanes).NotEqual(archsimd.BroadcastUint32x4(0))
}

func mask32x4Bits(mask archsimd.Mask32x4) uint8 {
	var lanes [4]int32
	mask.ToInt32x4().StoreArray(&lanes)
	var bits uint8
	for i, lane := range lanes {
		if lane != 0 {
			bits |= 1 << i
		}
	}
	return bits
}

// rasterizeRowsSIMD is the 4-wide bit-exact rasteriser for the SIMD-eligible
// setup class (voodooSetupSIMDEligible): slope-register affine interpolation,
// RGB write and optional depth, fog, texture, blend, alpha test, chroma, dither
// and stipple stages. The stages without suitable vector operations use scalar
// lane hybrids. Vector arithmetic mirrors the scalar order, and the
// uint32(ch*255) pack squashes NaN lanes to zero to match Go's scalar
// conversion. Bands own disjoint rows, so the in-row masked writes are safe.
func rasterizeRowsSIMD(b *VoodooSoftwareBackend, s *voodooTriangleSetup, minY, maxY int) {
	v0, v1, v2 := s.v0, s.v1, s.v2

	bc := archsimd.BroadcastFloat32x4
	laneIdx := archsimd.LoadFloat32x4Array(&[4]float32{0, 1, 2, 3})
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

	clamp01 := func(x archsimd.Float32x4) archsimd.Float32x4 {
		x = zero.IfElse(x.Less(zero), x) // where x<0 -> 0
		x = one.IfElse(x.Greater(one), x)
		return x
	}
	packCh := func(ch archsimd.Float32x4, shift uint64) archsimd.Uint32x4 {
		prod := ch.Mul(c255)
		i := prod.ConvertToInt32().ToBits()
		i = i.Masked(prod.Equal(prod)) // NaN lanes -> 0 (Go uint32(NaN)==0)
		if shift != 0 {
			i = i.ShiftAllLeft(shift)
		}
		return i
	}
	depthCompare := func(z, oldZ archsimd.Float32x4) archsimd.Mask32x4 {
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
	alphaTestCompare := func(a archsimd.Float32x4) archsimd.Mask32x4 {
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

		for xc := s.minX; xc < s.maxX; xc += 4 {
			sliceLen := 4
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
				writeMask = writeMask.And(mask32x4FromBits(bits))
			}

			// The Linux ARM64 compiler fuses both additions in
			// `v0 + dx*ddx + dy*ddy`. Match that order with two MulAdd calls so
			// depth values and colour feedback remain bit-exact.
			dxv := pxv.Sub(v0Xv)
			r := dyv.MulAdd(drdyv, dxv.MulAdd(drdxv, v0Rv))
			g := dyv.MulAdd(dgdyv, dxv.MulAdd(dgdxv, v0Gv))
			bb := dyv.MulAdd(dbdyv, dxv.MulAdd(dbdxv, v0Bv))
			a := dyv.MulAdd(dadyv, dxv.MulAdd(dadxv, v0Av))
			z := dyv.MulAdd(dzdyv, dxv.MulAdd(dzdxv, v0Zv))

			wordBase := rowBase + xc
			// Only complete 4-pixel chunks fully inside the row use full 4-lane
			// vector loads/stores. Any partial chunk (row end or buffer end) uses
			// a scalar gather/scatter over the SAME vector-computed values. This
			// avoids SlicePart entirely: a short slice would make SlicePart
			// address a full 4-lane vector that straddles into the next row (a
			// checkptr straddle at the buffer end, and a cross-band read/write
			// under row-band parallelism). Full chunks touch only in-row memory.
			partial := sliceLen < simdF32Lanes

			var oldZ archsimd.Float32x4
			if s.depthEnable {
				if partial {
					var ob [4]float32
					for l := 0; l < sliceLen; l++ {
						ob[l] = b.depthBuffer[wordBase+l]
					}
					oldZ = archsimd.LoadFloat32x4Array(&ob)
				} else {
					oldZ = archsimd.LoadFloat32x4(b.depthBuffer[wordBase : wordBase+simdF32Lanes])
				}
				writeMask = writeMask.And(depthCompare(z, oldZ))
			}

			// No lane survives to this chunk: skip the remaining per-pixel work
			// (texture sample, clamp, fog, dither, pack, store). Bit-identical to
			// writing nothing.
			if mask32x4Bits(writeMask) == 0 {
				continue
			}

			// Texture: hybrid stage. Texel fetch (no suitable SIMD gather in Go 1.27)
			// and combineVoodooColors run scalar per lane on the pre-clamp
			// interpolated r,g,b,a; texture coordinates use the exact scalar slope
			// expression so gc fuses them identically. r,g,b,a are replaced with
			// the combined result, then the vector pipeline resumes.
			if s.texActive {
				var ra, ga, bba, aa [4]float32
				r.StoreArray(&ra)
				g.StoreArray(&ga)
				bb.StoreArray(&bba)
				a.StoreArray(&aa)
				wmBits := mask32x4Bits(writeMask)
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
				r = archsimd.LoadFloat32x4Array(&ra)
				g = archsimd.LoadFloat32x4Array(&ga)
				bb = archsimd.LoadFloat32x4Array(&bba)
				a = archsimd.LoadFloat32x4Array(&aa)
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
				var ra, ga, ba [4]float32
				r.StoreArray(&ra)
				g.StoreArray(&ga)
				bb.StoreArray(&ba)
				var keep uint8
				for l := 0; l < simdF32Lanes; l++ {
					if !voodooChromaTest(s.chromaKey, s.chromaRange, ra[l], ga[l], ba[l]) {
						keep |= 1 << uint(l)
					}
				}
				writeMask = writeMask.And(mask32x4FromBits(keep))
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
				var ra, ga, ba [4]float32
				r.StoreArray(&ra)
				g.StoreArray(&ga)
				bb.StoreArray(&ba)
				for l := 0; l < simdF32Lanes; l++ {
					th := b.getDitherThreshold(xc+l, y, s.dither2x2)
					ra[l] = b.applyDither(ra[l], th)
					ga[l] = b.applyDither(ga[l], th)
					ba[l] = b.applyDither(ba[l], th)
				}
				r = archsimd.LoadFloat32x4Array(&ra)
				g = archsimd.LoadFloat32x4Array(&ga)
				bb = archsimd.LoadFloat32x4Array(&ba)
			}

			if s.forceOpaqueAlpha {
				a = one
			}

			// Alpha blending reads each target's own destination pixel and
			// multiplies through per-target factors, so it is a scalar hybrid
			// over the vector-computed source colour: the surviving lanes run
			// the exact scalar blend, including getBlendFactor selection, the
			// inv255 unpack and the clampf/truncate pack. Everything before
			// this point is still vectorised, which is where the win is.
			if s.alphaBlendEnable {
				var ra, ga, ba, aa [4]float32
				var zz [4]float32
				r.StoreArray(&ra)
				g.StoreArray(&ga)
				bb.StoreArray(&ba)
				a.StoreArray(&aa)
				z.StoreArray(&zz)
				wmBits := mask32x4Bits(writeMask)
				for l := 0; l < sliceLen; l++ {
					if wmBits&(1<<uint(l)) == 0 {
						continue
					}
					word := wordBase + l
					for ti, target := range s.targets {
						targetsU[ti][word] = blendVoodooPixel(b, s.srcBlendFactor, s.dstBlendFactor,
							target, word*4, ra[l], ga[l], ba[l], aa[l])
					}
					if s.depthEnable && s.depthWrite {
						b.depthBuffer[word] = zz[l]
					}
				}
				continue
			}

			packed := packCh(r, 0).Or(packCh(g, 8)).Or(packCh(bb, 16)).Or(packCh(a, 24))

			if partial {
				var pk [4]uint32
				var wm [4]int32
				var zz [4]float32
				packed.StoreArray(&pk)
				writeMask.ToInt32x4().StoreArray(&wm)
				z.StoreArray(&zz)
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
				d := archsimd.LoadUint32x4(du)
				packed.IfElse(writeMask, d).Store(du)
			}

			if s.depthEnable && s.depthWrite {
				dslice := b.depthBuffer[wordBase : wordBase+simdF32Lanes]
				z.IfElse(writeMask, oldZ).Store(dslice)
			}
		}
	}
}
