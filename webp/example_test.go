// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp_test

import (
	"fmt"
	"image"
	"image/draw"
	"log"
	"os"

	"golang.org/x/image/webp"
)

// This example composites one animation cycle. DecodeAll returns local frames,
// so the caller applies each frame's blending and disposal settings.
func ExampleDecodeAll() {
	f, err := os.Open("../testdata/animated.lossless.webp")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	a, err := webp.DecodeAll(f)
	if err != nil {
		log.Fatal(err)
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, a.Config.Width, a.Config.Height))
	background := image.NewUniform(a.BackgroundColor)
	draw.Draw(canvas, canvas.Bounds(), background, image.Point{}, draw.Src)

	for i, frame := range a.Frames {
		op := draw.Src
		if frame.Blend {
			op = draw.Over
		}
		r := frame.Image.Bounds()
		draw.Draw(canvas, r, frame.Image, r.Min, op)

		// Display or save canvas here, before applying disposal. A player
		// would display it for frame.Duration. Copy canvas if retaining it,
		// since the next frame changes the same image.
		fmt.Printf("frame %d: %v, duration %v\n", i, canvas.Bounds(), frame.Duration)

		if frame.DisposeToBackground {
			draw.Draw(canvas, r, background, image.Point{}, draw.Src)
		}
	}
	// Start each subsequent cycle with the background color again.
	// LoopCount is the total number of cycles; zero means repeat indefinitely.

	// Output:
	// frame 0: (0,0)-(4,4), duration 100ms
	// frame 1: (0,0)-(4,4), duration 200ms
	// frame 2: (0,0)-(4,4), duration 50ms
	// frame 3: (0,0)-(4,4), duration 100ms
}
