// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestAnimationPlayback(t *testing.T) {
	left := image.Rect(0, 0, 2, 2)
	right := image.Rect(2, 0, 4, 2)
	black := color.NRGBA{A: 255}
	data := animationTestRIFF(
		animationTestVP8X(4, 2, 0x12),
		animationTestANIM(black, 1),
		animationTestANMF(left, 10, 3, animationTestLossless(t, "green")),
		animationTestANMF(right, 20, 0, animationTestLossless(t, "red")),
		animationTestANMF(right, 30, 2, animationTestLossless(t, "blue")),
		animationTestANMF(right, 0, 2, animationTestLossless(t, "transparent")),
	)
	a, err := DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]color.NRGBA{
		// Replace the left region with opaque green.
		{{G: 255, A: 255}, black},
		// Dispose the left region, then blend red over the black right region.
		{black, {R: 128, A: 255}},
		// Replace the right region, preserving the new frame's transparency.
		{black, {B: 255, A: 128}},
		// A fully transparent replacement clears the previous frame.
		{black, {}},
	}
	if len(a.Frames) != len(want) {
		t.Fatalf("len(Frames) = %d, want %d", len(a.Frames), len(want))
	}

	// Reconstruct the display frames using only the exported animation data.
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
		for y := 0; y < 2; y++ {
			for x := 0; x < 4; x++ {
				if got := canvas.NRGBAAt(x, y); got != want[i][x/2] {
					t.Errorf("frame %d pixel (%d, %d) = %v, want %v", i, x, y, got, want[i][x/2])
				}
			}
		}
		if frame.DisposeToBackground {
			draw.Draw(canvas, r, background, image.Point{}, draw.Src)
		}
	}
}
