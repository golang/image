// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestScaleUniformBounds(t *testing.T) {
	for _, kernel := range []*Kernel{CatmullRom, BiLinear} {
		for _, op := range []Op{Src, Over} {
			for _, opts := range []*Options{nil, {}} {
				src := image.NewUniform(color.NRGBA{R: 200, G: 100, B: 50, A: 128})
				bounds := image.Rect(3, 4, 13, 14)
				got := image.NewRGBA(bounds)
				want := image.NewRGBA(bounds)
				background := image.NewUniform(color.RGBA{B: 255, A: 255})
				Draw(got, bounds, background, image.Point{}, Src)
				Draw(want, bounds, background, image.Point{}, Src)
				dr := image.Rect(1, 2, 12, 13)
				Draw(want, dr, src, image.Point{}, op)
				kernel.Scale(got, dr, src, src.Bounds(), op, opts)
				if !bytes.Equal(got.Pix, want.Pix) {
					t.Errorf("kernel %p, op %v, opts %v: scaling uniform differs from drawing", kernel, op, opts)
				}
			}
		}
	}
}
