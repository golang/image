// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tiff

import (
	"bytes"
	"os"
	"testing"
)

var roundtripTests = []string{
	"video-001.tiff",
	"bw-packbits.tiff",
}

func TestRoundtrip(t *testing.T) {
	for _, filename := range roundtripTests {
		f, err := os.Open(testdataDir + filename)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := Decode(f)
		if err != nil {
			t.Fatal(err)
		}

		out := new(bytes.Buffer)
		err = Encode(out, img)
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
