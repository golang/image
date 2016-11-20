// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sfnt

import (
	"bytes"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestParse(t *testing.T) {
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	testFont(t, f)
}

func TestParseReaderAt(t *testing.T) {
	f, err := ParseReaderAt(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatalf("ParseReaderAt: %v", err)
	}
	testFont(t, f)
}

func testFont(t *testing.T, f *Font) {
	if got, want := f.UnitsPerEm(), Units(2048); got != want {
		t.Errorf("UnitsPerEm: got %d, want %d", got, want)
	}
	// The exact number of glyphs in goregular.TTF can vary, and future
	// versions may add more glyphs, but https://blog.golang.org/go-fonts says
	// that "The WGL4 character set... [has] more than 650 characters in all.
	if got, want := f.NumGlyphs(), 650; got <= want {
		t.Errorf("NumGlyphs: got %d, want > %d", got, want)
	}
}
