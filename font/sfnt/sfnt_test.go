// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sfnt

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestTrueTypeParse(t *testing.T) {
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	testTrueType(t, f)
}

func TestTrueTypeParseReaderAt(t *testing.T) {
	f, err := ParseReaderAt(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatalf("ParseReaderAt: %v", err)
	}
	testTrueType(t, f)
}

func testTrueType(t *testing.T, f *Font) {
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

func TestPostScript(t *testing.T) {
	data, err := ioutil.ReadFile(filepath.Join("..", "testdata", "CFFTest.otf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: replace this by a higher level test, once we parse Type 2
	// Charstrings.
	//
	// As a sanity check for now, note that each string ends in '\x0e', which
	// 5177.Type2.pdf Appendix A defines as "endchar".
	wants := [...]string{
		"\xf7\x63\x8b\xbd\xf8\x45\xbd\x01\xbd\xbd\xf7\xc0\xbd\x03\xbd\x16\xf8\x24\xf8\xa9\xfc\x24\x06\xbd\xfc\x77\x15\xf8\x45\xf7\xc0\xfc\x45\x07\x0e",
		"\x8b\xef\xf8\xec\xef\x01\xef\xdb\xf7\x84\xdb\x03\xf7\xc0\xf9\x50\x15\xdb\xb3\xfb\x0c\x3b\xfb\x2a\x6d\xfb\x8e\x31\x3b\x63\xf7\x0c\xdb\xf7\x2a\xa9\xf7\x8e\xe5\x1f\xef\x04\x27\x27\xfb\x70\xfb\x48\xfb\x48\xef\xfb\x70\xef\xef\xef\xf7\x70\xf7\x48\xf7\x48\x27\xf7\x70\x27\x1f\x0e",
		"\xf6\xa0\x76\x01\xef\xf7\x5c\x03\xef\x16\xf7\x5c\xf9\xb4\xfb\x5c\x06\x0e",
		"\xf7\x89\xe1\x03\xf7\x21\xf8\x9c\x15\x87\xfb\x38\xf7\x00\xb7\xe1\xfc\x0a\xa3\xf8\x18\xf7\x00\x9f\x81\xf7\x4e\xfb\x04\x6f\x81\xf7\x3a\x33\x85\x83\xfb\x52\x05\x0e",
	}
	if ng := f.NumGlyphs(); ng != len(wants) {
		t.Fatalf("NumGlyphs: got %d, want %d", ng, len(wants))
	}
	for i, want := range wants {
		gd, err := f.viewGlyphData(nil, i)
		if err != nil {
			t.Errorf("i=%d: %v", i, err)
			continue
		}
		if got := string(gd); got != want {
			t.Errorf("i=%d:\ngot  % x\nwant % x", i, got, want)
		}
	}
}
