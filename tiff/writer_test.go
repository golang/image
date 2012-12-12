// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tiff

import (
	"bytes"
	"image"
	"io/ioutil"
	"os"
	"testing"
)

var roundtripTests = []struct {
	filename string
	opts     *Options
}{
	{"video-001.tiff", nil},
	{"bw-packbits.tiff", nil},
	{"video-001.tiff", &Options{Predictor: true}},
	{"video-001.tiff", &Options{Compression: Deflate}},
	{"video-001.tiff", &Options{Predictor: true, Compression: Deflate}},
}

func openImage(filename string) (image.Image, error) {
	f, err := os.Open(testdataDir + filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

func TestRoundtrip(t *testing.T) {
	for _, rt := range roundtripTests {
		img, err := openImage(rt.filename)
		if err != nil {
			t.Fatal(err)
		}
		out := new(bytes.Buffer)
		err = Encode(out, img, rt.opts)
		if err != nil {
			t.Fatal(err)
		}

		img2, err := Decode(&buffer{buf: out.Bytes()})
		if err != nil {
			t.Fatal(err)
		}
		compare(t, img, img2)
	}
}

// TestRoundtrip2 tests that encoding and decoding an image whose
// origin is not (0, 0) gives the same thing.
func TestRoundtrip2(t *testing.T) {
	m0 := image.NewRGBA(image.Rect(3, 4, 9, 8))
	for i := range m0.Pix {
		m0.Pix[i] = byte(i)
	}
	out := new(bytes.Buffer)
	if err := Encode(out, m0, nil); err != nil {
		t.Fatal(err)
	}
	m1, err := Decode(&buffer{buf: out.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	compare(t, m0, m1)
}

// BenchmarkEncode benchmarks the encoding of an image.
func BenchmarkEncode(b *testing.B) {
	img, err := openImage("video-001.tiff")
	if err != nil {
		b.Fatal(err)
	}
	s := img.Bounds().Size()
	b.SetBytes(int64(s.X * s.Y * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Encode(ioutil.Discard, img, nil)
	}
}
