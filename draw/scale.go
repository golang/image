// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw

// TODO: should Scale and NewScaler also take an Op argument?

import (
	"image"
	"image/color"
	"math"
)

// Scale scales the part of the source image defined by src and sr and writes
// to the part of the destination image defined by dst and dr.
//
// Of the interpolators provided by this package:
//	- NearestNeighbor is fast but usually looks worst.
//	- CatmullRom is slow but usually looks best.
//	- ApproxBiLinear has reasonable speed and quality.
//
// The time taken depends on the size of dr. For kernel interpolators, the
// speed also depends on the size of sr, and so are often slower than
// non-kernel interpolators, especially when scaling down.
func Scale(dst Image, dr image.Rectangle, src image.Image, sr image.Rectangle, q Interpolator) {
	q.NewScaler(int32(dr.Dx()), int32(dr.Dy()), int32(sr.Dx()), int32(sr.Dy())).Scale(dst, dr.Min, src, sr.Min)
}

// Scaler scales part of a source image, starting from sp, and writes to a
// destination image, starting from dp. The destination and source width and
// heights are pre-determined, as part of the Scaler.
//
// A Scaler is safe to use concurrently.
type Scaler interface {
	Scale(dst Image, dp image.Point, src image.Image, sp image.Point)
}

// Interpolator creates scalers for a given destination and source width and
// heights.
type Interpolator interface {
	NewScaler(dw, dh, sw, sh int32) Scaler
}

// Kernel is an interpolator that blends source pixels weighted by a symmetric
// kernel function.
type Kernel struct {
	// Support is the kernel support and must be >= 0. At(t) is assumed to be
	// zero when t >= Support.
	Support float64
	// At is the kernel function. It will only be called with t in the
	// range [0, Support).
	At func(t float64) float64
}

// NewScaler implements the Interpolator interface.
func (k *Kernel) NewScaler(dw, dh, sw, sh int32) Scaler {
	return &kernelScaler{
		dw:         dw,
		dh:         dh,
		sw:         sw,
		sh:         sh,
		horizontal: newDistrib(k, dw, sw),
		vertical:   newDistrib(k, dh, sh),
	}
}

var (
	// NearestNeighbor is the nearest neighbor interpolator. It is very fast,
	// but usually gives very low quality results. When scaling up, the result
	// will look 'blocky'.
	NearestNeighbor = Interpolator(nnInterpolator{})

	// ApproxBiLinear is a mixture of the nearest neighbor and bi-linear
	// interpolators. It is fast, but usually gives medium quality results.
	//
	// It implements bi-linear interpolation when upscaling and a bi-linear
	// blend of the 4 nearest neighbor pixels when downscaling. This yields
	// nicer quality than nearest neighbor interpolation when upscaling, but
	// the time taken is independent of the number of source pixels, unlike the
	// bi-linear interpolator. When downscaling a large image, the performance
	// difference can be significant.
	ApproxBiLinear = Interpolator(ablInterpolator{})

	// BiLinear is the tent kernel. It is slow, but usually gives high quality
	// results.
	BiLinear = &Kernel{1, func(t float64) float64 {
		return 1 - t
	}}

	// CatmullRom is the Catmull-Rom kernel. It is very slow, but usually gives
	// very high quality results.
	//
	// It is an instance of the more general cubic BC-spline kernel with parameters
	// B=0 and C=0.5. See Mitchell and Netravali, "Reconstruction Filters in
	// Computer Graphics", Computer Graphics, Vol. 22, No. 4, pp. 221-228.
	CatmullRom = &Kernel{2, func(t float64) float64 {
		if t < 1 {
			return (1.5*t-2.5)*t*t + 1
		}
		return ((-0.5*t+2.5)*t-4)*t + 2
	}}

	// TODO: a Kaiser-Bessel kernel?
)

type nnInterpolator struct{}

func (nnInterpolator) NewScaler(dw, dh, sw, sh int32) Scaler { return &nnScaler{dw, dh, sw, sh} }

type nnScaler struct {
	dw, dh, sw, sh int32
}

func (z *nnScaler) Scale(dst Image, dp image.Point, src image.Image, sp image.Point) {
	if z.dw <= 0 || z.dh <= 0 || z.sw <= 0 || z.sh <= 0 {
		return
	}
	dstColorRGBA64 := &color.RGBA64{}
	dstColor := color.Color(dstColorRGBA64)
	for dy := int32(0); dy < z.dh; dy++ {
		sy := (2*uint64(dy) + 1) * uint64(z.sh) / (2 * uint64(z.dh))
		for dx := int32(0); dx < z.dw; dx++ {
			sx := (2*uint64(dx) + 1) * uint64(z.sw) / (2 * uint64(z.dw))
			pr, pg, pb, pa := src.At(sp.X+int(sx), sp.Y+int(sy)).RGBA()
			dstColorRGBA64.R = uint16(pr)
			dstColorRGBA64.G = uint16(pg)
			dstColorRGBA64.B = uint16(pb)
			dstColorRGBA64.A = uint16(pa)
			dst.Set(dp.X+int(dx), dp.Y+int(dy), dstColor)
		}
	}
}

type ablInterpolator struct{}

func (ablInterpolator) NewScaler(dw, dh, sw, sh int32) Scaler { return &ablScaler{dw, dh, sw, sh} }

type ablScaler struct {
	dw, dh, sw, sh int32
}

func (z *ablScaler) Scale(dst Image, dp image.Point, src image.Image, sp image.Point) {
	if z.dw <= 0 || z.dh <= 0 || z.sw <= 0 || z.sh <= 0 {
		return
	}
	yscale := float64(z.sh) / float64(z.dh)
	xscale := float64(z.sw) / float64(z.dw)
	dstColorRGBA64 := &color.RGBA64{}
	dstColor := color.Color(dstColorRGBA64)
	for dy := int32(0); dy < z.dh; dy++ {
		sy := (float64(dy)+0.5)*yscale - 0.5
		sy0 := int32(sy)
		yFrac0 := sy - float64(sy0)
		yFrac1 := 1 - yFrac0
		sy1 := sy0 + 1
		if sy < 0 {
			sy0, sy1 = 0, 0
			yFrac0, yFrac1 = 0, 1
		} else if sy1 >= z.sh {
			sy1 = sy0
			yFrac0, yFrac1 = 1, 0
		}
		for dx := int32(0); dx < z.dw; dx++ {
			sx := (float64(dx)+0.5)*xscale - 0.5
			sx0 := int32(sx)
			xFrac0 := sx - float64(sx0)
			xFrac1 := 1 - xFrac0
			sx1 := sx0 + 1
			if sx < 0 {
				sx0, sx1 = 0, 0
				xFrac0, xFrac1 = 0, 1
			} else if sx1 >= z.sw {
				sx1 = sx0
				xFrac0, xFrac1 = 1, 0
			}
			s00ru, s00gu, s00bu, s00au := src.At(sp.X+int(sx0), sp.Y+int(sy0)).RGBA()
			s00r := float64(s00ru)
			s00g := float64(s00gu)
			s00b := float64(s00bu)
			s00a := float64(s00au)
			s10ru, s10gu, s10bu, s10au := src.At(sp.X+int(sx1), sp.Y+int(sy0)).RGBA()
			s10r := float64(s10ru)
			s10g := float64(s10gu)
			s10b := float64(s10bu)
			s10a := float64(s10au)
			s10r = xFrac1*s00r + xFrac0*s10r
			s10g = xFrac1*s00g + xFrac0*s10g
			s10b = xFrac1*s00b + xFrac0*s10b
			s10a = xFrac1*s00a + xFrac0*s10a
			s01ru, s01gu, s01bu, s01au := src.At(sp.X+int(sx0), sp.Y+int(sy1)).RGBA()
			s01r := float64(s01ru)
			s01g := float64(s01gu)
			s01b := float64(s01bu)
			s01a := float64(s01au)
			s11ru, s11gu, s11bu, s11au := src.At(sp.X+int(sx1), sp.Y+int(sy1)).RGBA()
			s11r := float64(s11ru)
			s11g := float64(s11gu)
			s11b := float64(s11bu)
			s11a := float64(s11au)
			s11r = xFrac1*s01r + xFrac0*s11r
			s11g = xFrac1*s01g + xFrac0*s11g
			s11b = xFrac1*s01b + xFrac0*s11b
			s11a = xFrac1*s01a + xFrac0*s11a
			s11r = yFrac1*s10r + yFrac0*s11r
			s11g = yFrac1*s10g + yFrac0*s11g
			s11b = yFrac1*s10b + yFrac0*s11b
			s11a = yFrac1*s10a + yFrac0*s11a
			dstColorRGBA64.R = uint16(s11r)
			dstColorRGBA64.G = uint16(s11g)
			dstColorRGBA64.B = uint16(s11b)
			dstColorRGBA64.A = uint16(s11a)
			dst.Set(dp.X+int(dx), dp.Y+int(dy), dstColor)
		}
	}
}

type kernelScaler struct {
	dw, dh, sw, sh       int32
	horizontal, vertical distrib
}

func (z *kernelScaler) Scale(dst Image, dp image.Point, src image.Image, sp image.Point) {
	if z.dw <= 0 || z.dh <= 0 || z.sw <= 0 || z.sh <= 0 {
		return
	}
	// TODO: is it worth having a sync.Pool for this temporary buffer?
	tmp := make([][4]float64, z.dw*z.sh)
	z.scaleX(tmp, src, sp)
	z.scaleY(dst, dp, tmp)
}

// source is a range of contribs, their inverse total weight, and that ITW
// divided by 0xffff.
type source struct {
	i, j               int32
	invTotalWeight     float64
	invTotalWeightFFFF float64
}

// contrib is the weight of a column or row.
type contrib struct {
	coord  int32
	weight float64
}

// distrib measures how source pixels are distributed over destination pixels.
type distrib struct {
	// sources are what contribs each column or row in the source image owns,
	// and the total weight of those contribs.
	sources []source
	// contribs are the contributions indexed by sources[s].i and sources[s].j.
	contribs []contrib
}

// newDistrib returns a distrib that distributes sw source columns (or rows)
// over dw destination columns (or rows).
func newDistrib(q *Kernel, dw, sw int32) distrib {
	scale := float64(sw) / float64(dw)
	halfWidth, kernelArgScale := q.Support, 1.0
	if scale > 1 {
		halfWidth *= scale
		kernelArgScale = 1 / scale
	}

	// Make the sources slice, one source for each column or row, and temporarily
	// appropriate its elements' fields so that invTotalWeight is the scaled
	// co-ordinate of the source column or row, and i and j are the lower and
	// upper bounds of the range of destination columns or rows affected by the
	// source column or row.
	n, sources := int32(0), make([]source, dw)
	for x := range sources {
		center := (float64(x)+0.5)*scale - 0.5
		i := int32(math.Floor(center - halfWidth))
		if i < 0 {
			i = 0
		}
		j := int32(math.Ceil(center + halfWidth))
		if j >= sw {
			j = sw - 1
			if j < i {
				j = i
			}
		}
		sources[x] = source{i: i, j: j, invTotalWeight: center}
		n += j - i + 1
	}

	contribs := make([]contrib, 0, n)
	for k, b := range sources {
		totalWeight := 0.0
		l := int32(len(contribs))
		for coord := b.i; coord <= b.j; coord++ {
			t := (b.invTotalWeight - float64(coord)) * kernelArgScale
			if t < 0 {
				t = -t
			}
			if t >= q.Support {
				continue
			}
			weight := q.At(t)
			if weight == 0 {
				continue
			}
			totalWeight += weight
			contribs = append(contribs, contrib{coord, weight})
		}
		totalWeight = 1 / totalWeight
		sources[k] = source{
			i:                  l,
			j:                  int32(len(contribs)),
			invTotalWeight:     totalWeight,
			invTotalWeightFFFF: totalWeight / 0xffff,
		}
	}

	return distrib{sources, contribs}
}

// scaleX distributes the source image's columns over the temporary image.
func (z *kernelScaler) scaleX(tmp [][4]float64, src image.Image, sp image.Point) {
	t := 0
	for y := int32(0); y < z.sh; y++ {
		for _, s := range z.horizontal.sources {
			var r, g, b, a float64
			for _, c := range z.horizontal.contribs[s.i:s.j] {
				rr, gg, bb, aa := src.At(sp.X+int(c.coord), sp.Y+int(y)).RGBA()
				r += float64(rr) * c.weight
				g += float64(gg) * c.weight
				b += float64(bb) * c.weight
				a += float64(aa) * c.weight
			}
			tmp[t] = [4]float64{
				r * s.invTotalWeightFFFF,
				g * s.invTotalWeightFFFF,
				b * s.invTotalWeightFFFF,
				a * s.invTotalWeightFFFF,
			}
			t++
		}
	}
}

// scaleY distributes the temporary image's rows over the destination image.
func (z *kernelScaler) scaleY(dst Image, dp image.Point, tmp [][4]float64) {
	dstColorRGBA64 := &color.RGBA64{}
	dstColor := color.Color(dstColorRGBA64)
	for x := int32(0); x < z.dw; x++ {
		for y, s := range z.vertical.sources {
			var r, g, b, a float64
			for _, c := range z.vertical.contribs[s.i:s.j] {
				p := &tmp[c.coord*z.dw+x]
				r += p[0] * c.weight
				g += p[1] * c.weight
				b += p[2] * c.weight
				a += p[3] * c.weight
			}
			dstColorRGBA64.R = ftou(r * s.invTotalWeight)
			dstColorRGBA64.G = ftou(g * s.invTotalWeight)
			dstColorRGBA64.B = ftou(b * s.invTotalWeight)
			dstColorRGBA64.A = ftou(a * s.invTotalWeight)
			dst.Set(dp.X+int(x), dp.Y+y, dstColor)
		}
	}
}

func ftou(f float64) uint16 {
	i := int32(0xffff*f + 0.5)
	if i > 0xffff {
		return 0xffff
	} else if i > 0 {
		return uint16(i)
	}
	return 0
}
