// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vp8

import (
	"image"
	"testing"
)

// TestEnsureImgReuseSmallerFrame tests that reusing a Decoder for a smaller
// frame shrinks the image bounds instead of keeping the previous, larger ones.
func TestEnsureImgReuseSmallerFrame(t *testing.T) {
	d := NewDecoder()

	d.frameHeader.Width, d.frameHeader.Height = 100, 100
	d.mbw, d.mbh = (100+0x0f)>>4, (100+0x0f)>>4
	d.ensureImg()
	if got, want := d.img.Bounds(), image.Rect(0, 0, 100, 100); got != want {
		t.Fatalf("first frame bounds = %v, want %v", got, want)
	}

	d.frameHeader.Width, d.frameHeader.Height = 50, 50
	d.mbw, d.mbh = (50+0x0f)>>4, (50+0x0f)>>4
	d.ensureImg()
	if got, want := d.img.Bounds(), image.Rect(0, 0, 50, 50); got != want {
		t.Fatalf("second frame bounds = %v, want %v", got, want)
	}

	d.frameHeader.Width, d.frameHeader.Height = 100, 100
	d.mbw, d.mbh = (100+0x0f)>>4, (100+0x0f)>>4
	d.ensureImg()
	if got, want := d.img.Bounds(), image.Rect(0, 0, 100, 100); got != want {
		t.Fatalf("third frame bounds = %v, want %v", got, want)
	}
}
