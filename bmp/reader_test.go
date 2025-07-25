// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmp

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"testing"

	_ "image/png"
)

const testdataDir = "../testdata/"

func compare(img0, img1 image.Image) error {
	b := img1.Bounds()
	if !b.Eq(img0.Bounds()) {
		return fmt.Errorf("wrong image size: want %s, got %s", img0.Bounds(), b)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c0 := img0.At(x, y)
			c1 := img1.At(x, y)
			r0, g0, b0, a0 := c0.RGBA()
			r1, g1, b1, a1 := c1.RGBA()
			if r0 != r1 || g0 != g1 || b0 != b1 || a0 != a1 {
				return fmt.Errorf("pixel at (%d, %d) has wrong color: want %v, got %v", x, y, c0, c1)
			}
		}
	}
	return nil
}

// TestDecode tests that decoding a PNG image and a BMP image result in the
// same pixel data.
func TestDecode(t *testing.T) {
	testCases := []string{
		"colormap",
		"colormap-0",
		"colormap-251",
		"video-001",
		"yellow_rose-small",
		"yellow_rose-small-v5",
		"bmp_1bpp",
		"bmp_4bpp",
		"bmp_8bpp",
	}

	for _, tc := range testCases {
		f0, err := os.Open(testdataDir + tc + ".png")
		if err != nil {
			t.Errorf("%s: Open PNG: %v", tc, err)
			continue
		}
		defer f0.Close()
		img0, _, err := image.Decode(f0)
		if err != nil {
			t.Errorf("%s: Decode PNG: %v", tc, err)
			continue
		}

		f1, err := os.Open(testdataDir + tc + ".bmp")
		if err != nil {
			t.Errorf("%s: Open BMP: %v", tc, err)
			continue
		}
		defer f1.Close()
		img1, _, err := image.Decode(f1)
		if err != nil {
			t.Errorf("%s: Decode BMP: %v", tc, err)
			continue
		}

		if err := compare(img0, img1); err != nil {
			t.Errorf("%s: %v", tc, err)
			continue
		}
	}
}

// TestEOF tests that decoding a BMP image returns io.ErrUnexpectedEOF
// when there are no headers or data is empty
func TestEOF(t *testing.T) {
	_, err := Decode(bytes.NewReader(nil))
	if err != io.ErrUnexpectedEOF {
		t.Errorf("Error should be io.ErrUnexpectedEOF on nil but got %v", err)
	}
}
