// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"
)

// hex is like fmt.Sprintf("% x", x) but also inserts dots every 16 bytes, to
// delineate VP8 macroblock boundaries.
func hex(x []byte) string {
	buf := new(bytes.Buffer)
	for len(x) > 0 {
		n := len(x)
		if n > 16 {
			n = 16
		}
		fmt.Fprintf(buf, " . % x", x[:n])
		x = x[n:]
	}
	return buf.String()
}

func TestDecodeVP8(t *testing.T) {
	// The original video-001.png image is 150x103.
	const w, h = 150, 103
	// w2 and h2 are the half-width and half-height, rounded up.
	const w2, h2 = int((w + 1) / 2), int((h + 1) / 2)

	f0, err := os.Open("../testdata/video-001.webp.ycbcr.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f0.Close()
	img0, err := png.Decode(f0)
	if err != nil {
		t.Fatal(err)
	}

	// The split-into-YCbCr-planes golden image is a 2*w2 wide and h+h2 high
	// gray image arranged in IMC4 format:
	//   YYYY
	//   YYYY
	//   BBRR
	// See http://www.fourcc.org/yuv.php#IMC4
	if got, want := img0.Bounds(), image.Rect(0, 0, 2*w2, h+h2); got != want {
		t.Fatalf("bounds0: got %v, want %v", got, want)
	}
	m0, ok := img0.(*image.Gray)
	if !ok {
		t.Fatal("decoded PNG image is not a Gray")
	}

	f1, err := os.Open("../testdata/video-001.webp")
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()
	img1, err := Decode(f1)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := img1.Bounds(), image.Rect(0, 0, w, h); got != want {
		t.Fatalf("bounds1: got %v, want %v", got, want)
	}
	m1, ok := img1.(*image.YCbCr)
	if !ok || m1.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		t.Fatal("decoded WEBP image is not a 4:2:0 YCbCr")
	}

	planes := []struct {
		name     string
		m1Pix    []uint8
		m1Stride int
		m0Rect   image.Rectangle
	}{
		{"Y", m1.Y, m1.YStride, image.Rect(0, 0, w, h)},
		{"Cb", m1.Cb, m1.CStride, image.Rect(0*w2, h, 1*w2, h+h2)},
		{"Cr", m1.Cr, m1.CStride, image.Rect(1*w2, h, 2*w2, h+h2)},
	}
	for _, plane := range planes {
		dx := plane.m0Rect.Dx()
		nDiff, diff := 0, make([]byte, dx)
		for j, y := 0, plane.m0Rect.Min.Y; y < plane.m0Rect.Max.Y; j, y = j+1, y+1 {
			got := plane.m1Pix[j*plane.m1Stride:][:dx]
			want := m0.Pix[y*m0.Stride+plane.m0Rect.Min.X:][:dx]
			if bytes.Equal(got, want) {
				continue
			}
			nDiff++
			if nDiff > 10 {
				t.Errorf("%s plane: more rows differ", plane.name)
				break
			}
			for i := range got {
				diff[i] = got[i] - want[i]
			}
			t.Errorf("%s plane: m0 row %d, m1 row %d\ngot %s\nwant%s\ndiff%s",
				plane.name, y, j, hex(got), hex(want), hex(diff))
		}
	}
}
