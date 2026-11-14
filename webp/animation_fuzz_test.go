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
	"time"
)

func animationFuzzInput(t testing.TB) []byte {
	t.Helper()
	return animationTestRIFF(
		animationTestVP8X(4, 4, 0x12),
		animationTestANIM(color.NRGBA{R: 10, G: 20, B: 30, A: 40}, 3),
		animationTestANMF(image.Rect(0, 0, 2, 2), 0, 0, animationTestLossless(t, "red")),
		animationTestANMF(image.Rect(2, 0, 4, 2), 1, 1, animationTestLossless(t, "green")),
		animationTestANMF(image.Rect(0, 2, 2, 4), 37, 2, animationTestLossless(t, "blue")),
		animationTestANMF(image.Rect(2, 2, 4, 4), 0xffffff, 3, animationTestLossless(t, "transparent")),
	)
}

func FuzzDecodeAnimation(f *testing.F) {
	data := animationFuzzInput(f)
	f.Add(data)
	f.Add(data[:30]) // A complete canvas header, but no animation data.
	f.Add(data[:len(data)-1])
	f.Add([]byte(nil))
	f.Add([]byte("not a WEBP image"))
	f.Add(animationTestRIFF(animationTestLossless(f, "red")))
	f.Add(animationTestRIFF(animationTestVP8X(2, 2, 0x10), animationTestLossless(f, "red")))
	for _, payload := range [][]byte{
		animationTestLossy(f),
		append(animationTestChunk("ALPH", []byte{0, 10, 30, 60, 100}), animationTestLossy(f)...),
		animationTestLossless(f, "red"),
	} {
		f.Add(animationTestRIFF(animationTestVP8X(4, 4, 0x12),
			animationTestANIM(color.NRGBA{}, 0), animationTestANMF(image.Rect(2, 2, 4, 4), 10, 2, payload)))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		c, err := DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return
		}
		// Bound both individual allocations and retained animation pixels.
		// Each ANMF needs at least an 8-byte chunk header and a 16-byte frame
		// header. This intentionally overestimates the possible frame count.
		pixels := int64(c.Width) * int64(c.Height)
		if c.Width <= 0 || c.Height <= 0 || pixels > 64<<10 || pixels*int64(max(1, len(data)/24)) > 1<<20 {
			return
		}

		m, decodeErr := Decode(bytes.NewReader(data))
		a, allErr := DecodeAll(bytes.NewReader(data))
		canvas := image.Rect(0, 0, c.Width, c.Height)
		if decodeErr == nil && (m == nil || m.Bounds() != canvas) {
			t.Fatalf("Decode image = %v, want bounds %v", m, canvas)
		}
		// Decode may succeed when a later frame is corrupt. The converse
		// cannot happen: decoding every frame also decodes the first one.
		if allErr != nil {
			return
		}
		if decodeErr != nil {
			t.Fatalf("DecodeAll succeeded, but Decode failed: %v", decodeErr)
		}
		if a == nil || len(a.Frames) == 0 {
			t.Fatal("DecodeAll succeeded without frames")
		}
		if a.Config.Width != c.Width || a.Config.Height != c.Height {
			t.Fatalf("DecodeAll canvas = %dx%d, DecodeConfig canvas = %dx%d", a.Config.Width, a.Config.Height, c.Width, c.Height)
		}
		if a.LoopCount < 0 || a.LoopCount > 65535 {
			t.Fatalf("LoopCount = %d, want a 16-bit unsigned value", a.LoopCount)
		}
		for i, frame := range a.Frames {
			if frame.Image == nil {
				t.Fatalf("frame %d has no image", i)
			}
			r := frame.Image.Bounds()
			if r.Empty() || !r.In(canvas) {
				t.Fatalf("frame %d bounds = %v, want a non-empty rectangle in %v", i, r, canvas)
			}
			if frame.Duration < 0 || frame.Duration > 0xffffff*time.Millisecond || frame.Duration%time.Millisecond != 0 {
				t.Fatalf("frame %d duration = %v, want a 24-bit number of milliseconds", i, frame.Duration)
			}
			if a.IsAnimated && (r.Min.X&1 != 0 || r.Min.Y&1 != 0) {
				t.Fatalf("frame %d origin = %v, want even coordinates", i, r.Min)
			}
			// Exercise translated image storage at both ends of the frame.
			frame.Image.At(r.Min.X, r.Min.Y)
			frame.Image.At(r.Max.X-1, r.Max.Y-1)
		}

		first := a.Frames[0]
		if !a.IsAnimated {
			if len(a.Frames) != 1 || a.LoopCount != 1 || a.BackgroundColor != (color.NRGBA{}) || first.Duration != 0 || first.Blend || first.DisposeToBackground {
				t.Fatal("static image has animation playback settings")
			}
			animationTestPixels(t, first.Image, m)
			return
		}
		// Reconstruct the first displayed frame from the public raw frame API.
		want := image.NewNRGBA(canvas)
		draw.Draw(want, canvas, image.NewUniform(a.BackgroundColor), image.Point{}, draw.Src)
		op := draw.Src
		if first.Blend {
			op = draw.Over
		}
		draw.Draw(want, first.Image.Bounds(), first.Image, first.Image.Bounds().Min, op)
		animationTestPixels(t, m, want)
	})
}

func BenchmarkDecodeAnimationFirst(b *testing.B) {
	data := animationFuzzInput(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Decode(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeAnimationAll(b *testing.B) {
	data := animationFuzzInput(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := DecodeAll(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}
