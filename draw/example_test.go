// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw_test

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

func ExampleDraw() {
	fSrc, err := os.Open("../testdata/blue-purple-pink.png")
	if err != nil {
		log.Fatal(err)
	}
	defer fSrc.Close()
	src, err := png.Decode(fSrc)
	if err != nil {
		log.Fatal(err)
	}

	sr := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, 400, 300))
	green := image.NewUniform(color.RGBA{0x00, 0x1f, 0x00, 0xff})
	draw.Copy(dst, image.Point{}, green, dst.Bounds(), nil)
	qs := []draw.Interpolator{
		draw.NearestNeighbor,
		draw.ApproxBiLinear,
		draw.CatmullRom,
	}
	const cos60, sin60 = 0.5, 0.866025404
	t := &f64.Aff3{
		+2 * cos60, -2 * sin60, 100,
		+2 * sin60, +2 * cos60, 100,
	}

	draw.Copy(dst, image.Point{20, 30}, src, sr, nil)
	for i, q := range qs {
		q.Scale(dst, image.Rect(200+10*i, 100*i, 600+10*i, 150+100*i), src, sr, nil)
	}
	draw.NearestNeighbor.Transform(dst, t, src, sr, nil)

	red := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			red.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 0x11),
				A: uint8(y * 0x11),
			})
		}
	}
	red.SetNRGBA(0, 0, color.NRGBA{0xff, 0xff, 0x00, 0xff})
	red.SetNRGBA(15, 15, color.NRGBA{0xff, 0xff, 0x00, 0xff})

	ops := []draw.Op{
		draw.Over,
		draw.Src,
	}
	for i, op := range ops {
		q, opts := draw.NearestNeighbor, &draw.Options{Op: op}
		dr := image.Rect(120+10*i, 150+60*i, 170+10*i, 200+60*i)
		q.Scale(dst, dr, red, red.Bounds(), opts)
		t := &f64.Aff3{
			+cos60, -sin60, float64(190 + 10*i),
			+sin60, +cos60, float64(140 + 50*i),
		}
		q.Transform(dst, t, red, red.Bounds(), opts)
	}

	// Change false to true to write the resultant image to disk.
	if false {
		fDst, err := os.Create("out.png")
		if err != nil {
			log.Fatal(err)
		}
		defer fDst.Close()
		err = png.Encode(fDst, dst)
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("dst has bounds %v.\n", dst.Bounds())
	// Output:
	// dst has bounds (0,0)-(400,300).
}
