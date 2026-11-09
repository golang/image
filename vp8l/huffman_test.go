// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vp8l

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

func TestDecodeSimpleCode(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []byte
	}{
		// A 2x1 image whose green code lists 200 before 10.
		{"decreasing", "\x2f\x01\x00\x00\x00\x38\xb2\x42\x44\xff\x43", []byte{0, 10, 0, 255, 0, 200, 0, 255}},
		// A 1x1 image whose green code lists 7 twice, followed by a red
		// code that lists 0 and 255.
		{"repeated", "\x2f\x00\x00\x00\x00\xf8\xc1\xc1\xfc\x47\xff\x23", []byte{255, 7, 0, 255}},
	}
	for _, tt := range tests {
		m, err := Decode(strings.NewReader(tt.data))
		if err != nil {
			t.Errorf("%s: Decode: %v", tt.name, err)
			continue
		}
		if got := m.(*image.NRGBA).Pix; !bytes.Equal(got, tt.want) {
			t.Errorf("%s: got pixels %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBuildSimple(t *testing.T) {
	for _, symbols := range [][]uint32{{0}, {200}, {2, 5}, {5, 2}, {255, 0}, {7, 7}} {
		var s [2]uint32
		copy(s[:], symbols)
		var simple hTree
		if err := simple.buildSimple(uint32(len(symbols)), s, 256); err != nil {
			t.Fatalf("buildSimple(%v): %v", symbols, err)
		}
		// A simple code is the canonical code that gives each symbol a
		// code length of 1.
		codeLengths := make([]uint32, 256)
		for _, sym := range symbols {
			codeLengths[sym] = 1
		}
		var canonical hTree
		if err := canonical.build(codeLengths); err != nil {
			t.Fatalf("build(%v): %v", symbols, err)
		}
		d0 := &decoder{r: strings.NewReader("\x5a\xc3")}
		d1 := &decoder{r: strings.NewReader("\x5a\xc3")}
		for i := 0; i < 16; i++ {
			got, err0 := simple.next(d0)
			want, err1 := canonical.next(d1)
			if err0 != nil || err1 != nil {
				t.Fatalf("%v: next: %v, %v", symbols, err0, err1)
			}
			if got != want || d0.bits != d1.bits || d0.nBits != d1.nBits {
				t.Errorf("%v: symbol %d: got %d with %d bits left, want %d with %d bits left", symbols, i, got, d0.nBits, want, d1.nBits)
				break
			}
		}
	}
}
