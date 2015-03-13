// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw_test

import (
	"fmt"
	"image"
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
