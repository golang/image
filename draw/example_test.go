// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw_test

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
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
	c, s := math.Cos(math.Pi/3), math.Sin(math.Pi/3)
	t := &f64.Aff3{
		+2 * c, -2 * s, 100,
		+2 * s, +2 * c, 100,
	}

	draw.Copy(dst, image.Point{20, 30}, src, sr, nil)
	for i, q := range qs {
		q.Scale(dst, image.Rect(200+10*i, 100*i, 600+10*i, 150+100*i), src, sr, nil)
	}
	// TODO: delete the "_ = t" and uncomment this when Transform is implemented.
	// draw.NearestNeighbor.Transform(dst, t, src, sr, nil)
	_ = t

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
