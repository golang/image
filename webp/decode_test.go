// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"compress/bzip2"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"os"
	"strings"
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

func testDecodeLossy(t *testing.T, tc string, withAlpha bool) {
	webpFilename := "../testdata/" + tc + ".lossy.webp"
	pngFilename := webpFilename + ".ycbcr.png"
	if withAlpha {
		webpFilename = "../testdata/" + tc + ".lossy-with-alpha.webp"
		pngFilename = webpFilename + ".nycbcra.png"
	}

	f0, err := os.Open(webpFilename)
	if err != nil {
		t.Errorf("%s: Open WEBP: %v", tc, err)
		return
	}
	defer f0.Close()
	img0, err := Decode(f0)
	if err != nil {
		t.Errorf("%s: Decode WEBP: %v", tc, err)
		return
	}

	var (
		m0 *image.YCbCr
		a0 *image.NYCbCrA
		ok bool
	)
	if withAlpha {
		a0, ok = img0.(*image.NYCbCrA)
		if ok {
			m0 = &a0.YCbCr
		}
	} else {
		m0, ok = img0.(*image.YCbCr)
	}
	if !ok || m0.SubsampleRatio != image.YCbCrSubsampleRatio420 {
		t.Errorf("%s: decoded WEBP image is not a 4:2:0 YCbCr or 4:2:0 NYCbCrA", tc)
		return
	}
	// w2 and h2 are the half-width and half-height, rounded up.
	w, h := m0.Bounds().Dx(), m0.Bounds().Dy()
	w2, h2 := int((w+1)/2), int((h+1)/2)

	f1, err := os.Open(pngFilename)
	if err != nil {
		t.Errorf("%s: Open PNG: %v", tc, err)
		return
	}
	defer f1.Close()
	img1, err := png.Decode(f1)
	if err != nil {
		t.Errorf("%s: Open PNG: %v", tc, err)
		return
	}

	// The split-into-YCbCr-planes golden image is a 2*w2 wide and h+h2 high
	// (or 2*h+h2 high, if with Alpha) gray image arranged in IMC4 format:
	//   YYYY
	//   YYYY
	//   BBRR
	//   AAAA
	// See http://www.fourcc.org/yuv.php#IMC4
	pngW, pngH := 2*w2, h+h2
	if withAlpha {
		pngH += h
	}
	if got, want := img1.Bounds(), image.Rect(0, 0, pngW, pngH); got != want {
		t.Errorf("%s: bounds0: got %v, want %v", tc, got, want)
		return
	}
	m1, ok := img1.(*image.Gray)
	if !ok {
		t.Errorf("%s: decoded PNG image is not a Gray", tc)
		return
	}

	type plane struct {
		name     string
		m0Pix    []uint8
		m0Stride int
		m1Rect   image.Rectangle
	}
	planes := []plane{
		{"Y", m0.Y, m0.YStride, image.Rect(0, 0, w, h)},
		{"Cb", m0.Cb, m0.CStride, image.Rect(0*w2, h, 1*w2, h+h2)},
		{"Cr", m0.Cr, m0.CStride, image.Rect(1*w2, h, 2*w2, h+h2)},
	}
	if withAlpha {
		planes = append(planes, plane{
			"A", a0.A, a0.AStride, image.Rect(0, h+h2, w, 2*h+h2),
		})
	}

	for _, plane := range planes {
		dx := plane.m1Rect.Dx()
		nDiff, diff := 0, make([]byte, dx)
		for j, y := 0, plane.m1Rect.Min.Y; y < plane.m1Rect.Max.Y; j, y = j+1, y+1 {
			got := plane.m0Pix[j*plane.m0Stride:][:dx]
			want := m1.Pix[y*m1.Stride+plane.m1Rect.Min.X:][:dx]
			if bytes.Equal(got, want) {
				continue
			}
			nDiff++
			if nDiff > 10 {
				t.Errorf("%s: %s plane: more rows differ", tc, plane.name)
				break
			}
			for i := range got {
				diff[i] = got[i] - want[i]
			}
			t.Errorf("%s: %s plane: m0 row %d, m1 row %d\ngot %s\nwant%s\ndiff%s",
				tc, plane.name, j, y, hex(got), hex(want), hex(diff))
		}
	}
}

func TestDecodeVP8(t *testing.T) {
	testCases := []string{
		"blue-purple-pink",
		"blue-purple-pink-large.no-filter",
		"blue-purple-pink-large.simple-filter",
		"blue-purple-pink-large.normal-filter",
		"video-001",
		"yellow_rose",
	}

	for _, tc := range testCases {
		testDecodeLossy(t, tc, false)
	}
}

func TestDecodeVP8XAlpha(t *testing.T) {
	testCases := []string{
		"yellow_rose",
	}

	for _, tc := range testCases {
		testDecodeLossy(t, tc, true)
	}
}

func TestDecodeVP8L(t *testing.T) {
	testCases := []struct {
		name    string
		f0      string
		f1      string
		wantErr string
	}{
		{name: "blue-purple-pink"},
		{name: "blue-purple-pink-large"},
		{name: "gopher-doc.1bpp"},
		{name: "gopher-doc.2bpp"},
		{name: "gopher-doc.4bpp"},
		{name: "gopher-doc.8bpp"},
		{name: "gopher-doc.with-alpha"},
		{name: "tux"},
		{name: "yellow_rose"},
		{
			// VP8L image with unreferenced Huffman tree groups.
			name: "remapped hgroups",
			f0:   "gopher-doc.skip-hgroup.lossless.webp",
			f1:   "gopher-doc.8bpp.png",
		},
		{
			// This file contains an image referencing Huffman tree group 65535,
			// and trivial entries for all the preceding groups.
			//
			// When we allocated all groups (inculding unused ones), decoding this
			// image allocated ~170MiB.
			//
			// We now reject this image for using more than 2600 hGroups.
			name:    "large VP8L huffman index",
			f0:      "large-huffman-index.lossless.webp.bz2",
			wantErr: "vp8l: too many Huffman trees",
		},
	}

	openFile := func(t *testing.T, test, name, suffix string) io.Reader {
		t.Helper()
		if name == "" {
			name = test + suffix
		}
		f, err := os.Open("../testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			f.Close()
		})
		if strings.HasSuffix(name, ".bz2") {
			return bzip2.NewReader(f)
		}
		return f
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f0 := openFile(t, tc.name, tc.f0, ".lossless.webp")
			img0, err := Decode(f0)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Decode WEBP: succeded, want error %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Decode WEBP: error %q, want %q", err, tc.wantErr)
				}
				return
			} else if err != nil {
				t.Fatalf("Decode WEBP: error %q, want success", err)
			}
			m0, ok := img0.(*image.NRGBA)
			if !ok {
				t.Fatalf("WEBP image is %T, want *image.NRGBA", img0)
			}

			name1 := tc.f1
			if name1 == "" {
				name1 = tc.name + ".png"
			}
			f1 := openFile(t, tc.name, tc.f1, ".png")
			img1, err := png.Decode(f1)
			if err != nil {
				t.Fatalf("Decode PNG: %v", err)
			}
			m1, ok := img1.(*image.NRGBA)
			if !ok {
				rgba1, ok := img1.(*image.RGBA)
				if !ok {
					t.Fatalf("PNG image is %T, want *image.NRGBA", img1)
				}
				if !rgba1.Opaque() {
					t.Fatalf("PNG image is non-opaque *image.RGBA, want *image.NRGBA")
				}
				// The image is fully opaque, so we can re-interpret the RGBA pixels
				// as NRGBA pixels.
				m1 = &image.NRGBA{
					Pix:    rgba1.Pix,
					Stride: rgba1.Stride,
					Rect:   rgba1.Rect,
				}
			}

			b0, b1 := m0.Bounds(), m1.Bounds()
			if b0 != b1 {
				t.Fatalf("bounds: got %v, want %v", b0, b1)
			}
			for i := range m0.Pix {
				if m0.Pix[i] != m1.Pix[i] {
					y := i / m0.Stride
					x := (i - y*m0.Stride) / 4
					i = 4 * (y*m0.Stride + x)
					t.Fatalf("at (%d, %d):\ngot  %02x %02x %02x %02x\nwant %02x %02x %02x %02x",
						x, y,
						m0.Pix[i+0], m0.Pix[i+1], m0.Pix[i+2], m0.Pix[i+3],
						m1.Pix[i+0], m1.Pix[i+1], m1.Pix[i+2], m1.Pix[i+3],
					)
				}
			}
		})
	}
}

// TestDecodePartitionTooLarge tests that decoding a malformed WEBP image
// doesn't try to allocate an unreasonable amount of memory. This WEBP image
// claims a RIFF chunk length of 0x12345678 bytes (291 MiB) compressed,
// independent of the actual image size (0 pixels wide * 0 pixels high).
//
// This is based on golang.org/issue/10790.
func TestDecodePartitionTooLarge(t *testing.T) {
	data := "RIFF\xff\xff\xff\x7fWEBPVP8 " +
		"\x78\x56\x34\x12" + // RIFF chunk length.
		"\xbd\x01\x00\x14\x00\x00\xb2\x34\x0a\x9d\x01\x2a\x96\x00\x67\x00"
	_, err := Decode(strings.NewReader(data))
	if err == nil {
		t.Fatal("got nil error, want non-nil")
	}
	if got, want := err.Error(), "too much data"; !strings.Contains(got, want) {
		t.Fatalf("got error %q, want something containing %q", got, want)
	}
}

func TestDuplicateVP8X(t *testing.T) {
	data := []byte{'R', 'I', 'F', 'F', 49, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', 'X', 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'V', 'P', '8', 'X', 10, 0, 0, 0, 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	_, err := Decode(bytes.NewReader(data))
	if err != errInvalidFormat {
		t.Fatalf("unexpected error: want %q, got %q", errInvalidFormat, err)
	}
}

func TestVP8XImageTooLarge(t *testing.T) {
	data := []byte{
		// WebP file header
		'R', 'I', 'F', 'F',
		22, 0, 0, 0, // file size
		'W', 'E', 'B', 'P',
		// ChunkHeader('VP8X')
		'V', 'P', '8', 'X',
		10, 0, 0, 0, // chunk size
		// bits + Reserved
		1 << 4, 0, 0, 0, // alpha bit set
		// Canvas Width Minus One
		0xff, 0xff, 0x00,
		// Canvas Height Minus One
		0xff, 0x7f, 0x00,
	}
	_, err := DecodeConfig(bytes.NewReader(data))
	if err != errInvalidFormat {
		t.Fatalf("unexpected error: want %q, got %q", errInvalidFormat, err)
	}
}

func TestVP8XImageNotQuiteTooLarge(t *testing.T) {
	data := []byte{
		// WebP file header
		'R', 'I', 'F', 'F',
		22, 0, 0, 0, // file size
		'W', 'E', 'B', 'P',
		// ChunkHeader('VP8X')
		'V', 'P', '8', 'X',
		10, 0, 0, 0, // chunk size
		// bits + Reserved
		1 << 4, 0, 0, 0, // alpha bit set
		// Canvas Width Minus One
		0xff, 0xff, 0x00,
		// Canvas Height Minus One
		0xfe, 0x7f, 0x00,
	}
	cfg, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error: want nil, got %q", err)
	}
	wantWidth := 0x10000
	wantHeight := 0x7fff
	if cfg.Width != wantWidth || cfg.Height != wantHeight {
		t.Fatalf("width x height: got %v x %v, want %v x %v", cfg.Width, cfg.Height, wantWidth, wantHeight)
	}
}

func TestVP8XAndVP8LDimensionMismatch(t *testing.T) {
	for _, test := range []struct {
		name string
		file string
	}{{
		name: "vp8",
		file: "../testdata/blue-purple-pink.lossless.webp",
	}, {
		name: "vp8l",
		file: "../testdata/blue-purple-pink.lossy.webp",
	}} {
		t.Run(test.name, func(t *testing.T) {
			vp8Chunk, err := os.ReadFile(test.file)
			if err != nil {
				t.Fatal(err)
			}
			vp8Chunk = vp8Chunk[12:]

			vp8xChunk := []byte{
				'V', 'P', '8', 'X',
				10, 0, 0, 0, // chunk size
				0, 0, 0, 0, // flags
				0, 0, 0, // Canvas Width Minus One: 0 (width = 1) (mismatch!)
				0, 0, 0, // Canvas Height Minus One: 0 (height = 1) (mismatch!)
			}

			fileSize := uint32(12 + len(vp8xChunk) + len(vp8Chunk) - 8)
			data := append([]byte("RIFF"), byte(fileSize), byte(fileSize>>8), byte(fileSize>>16), byte(fileSize>>24))
			data = append(data, []byte("WEBP")...)
			data = append(data, vp8xChunk...)
			data = append(data, vp8Chunk...)

			_, err = Decode(bytes.NewReader(data))
			if err != errInvalidFormat {
				t.Fatalf("unexpected error: want %v, got %v", errInvalidFormat, err)
			}
		})
	}
}

func benchmarkDecode(b *testing.B, filename string) {
	data, err := ioutil.ReadFile("../testdata/blue-purple-pink-large." + filename + ".webp")
	if err != nil {
		b.Fatal(err)
	}
	s := string(data)
	cfg, err := DecodeConfig(strings.NewReader(s))
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(cfg.Width * cfg.Height * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(strings.NewReader(s))
	}
}

func BenchmarkDecodeVP8NoFilter(b *testing.B)     { benchmarkDecode(b, "no-filter.lossy") }
func BenchmarkDecodeVP8SimpleFilter(b *testing.B) { benchmarkDecode(b, "simple-filter.lossy") }
func BenchmarkDecodeVP8NormalFilter(b *testing.B) { benchmarkDecode(b, "normal-filter.lossy") }
func BenchmarkDecodeVP8L(b *testing.B)            { benchmarkDecode(b, "lossless") }
