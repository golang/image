// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw

// TODO: autogenerate this file.

import (
	"image"
	"image/color"
)

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

func (z *kernelScaler) Scale(dst Image, dp image.Point, src image.Image, sp image.Point) {
	if z.dw <= 0 || z.dh <= 0 || z.sw <= 0 || z.sh <= 0 {
		return
	}
	// Create a temporary buffer:
	// scaleX distributes the source image's columns over the temporary image.
	// scaleY distributes the temporary image's rows over the destination image.
	// TODO: is it worth having a sync.Pool for this temporary buffer?
	tmp := make([][4]float64, z.dw*z.sh)
	z.scaleX(tmp, src, sp)
	z.scaleY(dst, dp, tmp)
}

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
