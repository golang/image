// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run gen.go

package draw

// TODO: add an Options type a la
// https://groups.google.com/forum/#!topic/golang-dev/fgn_xM0aeq4

import (
	"image"
	"math"
)

// Scaler scales the part of the source image defined by src and sr and writes
// to the part of the destination image defined by dst and dr.
//
// A Scaler is safe to use concurrently.
type Scaler interface {
	Scale(dst Image, dr image.Rectangle, src image.Image, sr image.Rectangle)
}

// Interpolator is an interpolation algorithm, when dst and src pixels don't
// have a 1:1 correspondance.
//
// Of the interpolators provided by this package:
//	- NearestNeighbor is fast but usually looks worst.
//	- CatmullRom is slow but usually looks best.
//	- ApproxBiLinear has reasonable speed and quality.
//
// The time taken depends on the size of dr. For kernel interpolators, the
// speed also depends on the size of sr, and so are often slower than
// non-kernel interpolators, especially when scaling down.
type Interpolator interface {
	Scaler
	// TODO: Transformer
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

// Scale implements the Scaler interface.
func (k *Kernel) Scale(dst Image, dr image.Rectangle, src image.Image, sr image.Rectangle) {
	k.NewScaler(dr.Dx(), dr.Dy(), sr.Dx(), sr.Dy()).Scale(dst, dr, src, sr)
}

// NewScaler returns a Scaler that is optimized for scaling multiple times with
// the same fixed destination and source width and height.
func (k *Kernel) NewScaler(dw, dh, sw, sh int) Scaler {
	return &kernelScaler{
		kernel:     k,
		dw:         int32(dw),
		dh:         int32(dh),
		sw:         int32(sw),
		sh:         int32(sh),
		horizontal: newDistrib(k, int32(dw), int32(sw)),
		vertical:   newDistrib(k, int32(dh), int32(sh)),
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

type ablInterpolator struct{}

type kernelScaler struct {
	kernel               *Kernel
	dw, dh, sw, sh       int32
	horizontal, vertical distrib
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

func ftou(f float64) uint16 {
	i := int32(0xffff*f + 0.5)
	if i > 0xffff {
		return 0xffff
	} else if i > 0 {
		return uint16(i)
	}
	return 0
}
