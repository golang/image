// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ccitt

import (
	"bytes"
	"io"
	"reflect"
	"testing"
	"unsafe"
)

func TestMaxCodeLength(t *testing.T) {
	br := bitReader{}
	size := unsafe.Sizeof(br.bits)
	size *= 8 // Convert from bytes to bits.

	// Check that the size of the bitReader.bits field is large enough to hold
	// nextBitMaxNBits bits.
	if size < nextBitMaxNBits {
		t.Fatalf("size: got %d, want >= %d", size, nextBitMaxNBits)
	}

	// Check that bitReader.nextBit will always leave enough spare bits in the
	// bitReader.bits field such that the decode function can unread up to
	// maxCodeLength bits.
	if want := size - nextBitMaxNBits; maxCodeLength > want {
		t.Fatalf("maxCodeLength: got %d, want <= %d", maxCodeLength, want)
	}

	// The decode function also assumes that, when saving bits to possibly
	// unread later, those bits fit inside a uint32.
	if maxCodeLength > 32 {
		t.Fatalf("maxCodeLength: got %d, want <= %d", maxCodeLength, 32)
	}
}

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

func TestInvalidCode(t *testing.T) {
	// The bit stream is:
	// 1 010 000000011011
	// Packing that LSB-first gives:
	// 0b_1101_1000_0000_0101
	src := []byte{0x05, 0xD8}

	table := modeTable[:]
	r := &bitReader{
		r: bytes.NewReader(src),
	}

	// "1" decodes to the value 2.
	if v, err := decode(r, table); v != 2 || err != nil {
		t.Fatalf("decode #0: got (%v, %v), want (2, nil)", v, err)
	}

	// "010" decodes to the value 6.
	if v, err := decode(r, table); v != 6 || err != nil {
		t.Fatalf("decode #0: got (%v, %v), want (6, nil)", v, err)
	}

	// "00000001" is an invalid code.
	if v, err := decode(r, table); v != 0 || err != errInvalidCode {
		t.Fatalf("decode #0: got (%v, %v), want (0, %v)", v, err, errInvalidCode)
	}

	// The bitReader should not have advanced after encountering an invalid
	// code. The remaining bits should be "000000011011".
	remaining := []byte(nil)
	for {
		bit, err := r.nextBit()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("nextBit: %v", err)
		}
		remaining = append(remaining, uint8('0'+bit))
	}
	if got, want := string(remaining), "000000011011"; got != want {
		t.Fatalf("remaining bits: got %q, want %q", got, want)
	}
}

// TODO: more tests.
