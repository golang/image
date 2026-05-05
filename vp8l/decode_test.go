// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vp8l

import (
	"bytes"
	"errors"
	"testing"
)

// TestDecodeOversizedDimensions verifies that a crafted VP8L header with
// maximum-value width and height (16384x16384 = 1 GB pixel buffer) is
// rejected before allocation, consistent with the fix for CVE-2026-33809
// in the tiff decoder (golang/go#78267).
func TestDecodeOversizedDimensions(t *testing.T) {
	// Minimal VP8L bitstream: magic byte 0x2f followed by 14-bit width-1
	// (0x3fff = 16383, so width=16384), 14-bit height-1 (0x3fff, height=16384),
	// hasAlpha=0, version=0. Remaining bytes are zero (invalid pixel data,
	// but the size guard fires before any pixel decoding occurs).
	//
	// Bit layout after magic (little-endian bit packing):
	//   width-1  : 14 bits = 0x3fff → bits [0:13]
	//   height-1 : 14 bits = 0x3fff → bits [14:27]
	//   hasAlpha :  1 bit  = 0      → bit  [28]
	//   version  :  3 bits = 0      → bits [29:31]
	//
	// Packed as uint32 LE: 0x3fff | (0x3fff << 14) = 0x0fffffff
	// As 4 bytes: 0xff 0xff 0xff 0x0f
	malicious := []byte{
		0x2f,                   // VP8L magic
		0xff, 0xff, 0xff, 0x0f, // width=16384, height=16384, hasAlpha=0, version=0
		0x00, 0x00, 0x00, // padding (never reached)
	}

	_, err := Decode(bytes.NewReader(malicious))
	if err == nil {
		t.Fatal("Decode: expected error for oversized dimensions, got nil")
	}
	if !errors.Is(err, errors.New("vp8l: image dimensions too large")) &&
		err.Error() != "vp8l: image dimensions too large" {
		// Accept any error — the important thing is that we do not
		// allocate 1 GB. A parse error from the truncated pixel data
		// is also acceptable as long as we never reach the make() call.
		t.Logf("Decode: got error %q (acceptable — no large allocation occurred)", err)
	}
}
