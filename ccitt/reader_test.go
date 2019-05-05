// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccitt

import (
	"bytes"
	"reflect"
	"testing"
)

func testTable(t *testing.T, table [][2]int16, codes []code, values []uint32) {
	// Build a map from values to codes.
	m := map[uint32]string{}
	for _, code := range codes {
		m[code.val] = code.str
	}

	// Build the encoded form of those values.
	enc := []byte(nil)
	bits := uint8(0)
	nBits := uint32(0)
	for _, v := range values {
		code := m[v]
		if code == "" {
			panic("unmapped code")
		}
		for _, c := range code {
			bits |= uint8(c&1) << nBits
			nBits++
			if nBits == 8 {
				enc = append(enc, bits)
				bits = 0
				nBits = 0
			}
		}
	}
	if nBits > 0 {
		enc = append(enc, bits)
	}

	// Decode that encoded form.
	got := []uint32(nil)
	r := &bitReader{
		r: bytes.NewReader(enc),
	}
	finalValue := values[len(values)-1]
	for {
		v, err := decode(r, table)
		if err != nil {
			t.Fatalf("after got=%d: %v", got, err)
		}
		got = append(got, v)
		if v == finalValue {
			break
		}
	}

	// Check that the round-tripped values were unchanged.
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("\ngot:  %v\nwant: %v", got, values)
	}
}

func TestModeTable(t *testing.T) {
	testTable(t, modeTable[:], modeCodes, []uint32{
		modePass,
		modeV0,
		modeV0,
		modeVL1,
		modeVR3,
		modeVL2,
		modeExt,
		modeVL1,
		modeH,
		modeVL1,
		modeVL1,
		modeEOL,
	})
}

func TestWhiteTable(t *testing.T) {
	testTable(t, whiteTable[:], whiteCodes, []uint32{
		0, 1, 256, 7, 128, 3, 2560,
	})
}

func TestBlackTable(t *testing.T) {
	testTable(t, blackTable[:], blackCodes, []uint32{
		63, 64, 63, 64, 64, 63, 22, 1088, 2048, 7, 6, 5, 4, 3, 2, 1, 0,
	})
}

// TODO: more tests.
