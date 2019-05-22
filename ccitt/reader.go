// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run gen.go

// Package ccitt implements a CCITT (fax) image decoder.
package ccitt

import (
	"encoding/binary"
	"errors"
	"io"
	"math/bits"
)

var (
	errInvalidCode = errors.New("ccitt: invalid code")
)

// Order specifies the bit ordering in a CCITT data stream.
type Order uint32

const (
	// LSB means Least Significant Bits first.
	LSB Order = iota
	// MSB means Most Significant Bits first.
	MSB
)

type bitReader struct {
	r io.Reader

	// readErr is the error returned from the most recent r.Read call. As the
	// io.Reader documentation says, when r.Read returns (n, err), "always
	// process the n > 0 bytes returned before considering the error err".
	readErr error

	order Order

	// The low nBits bits of the bits field hold upcoming bits in LSB order.
	bits  uint64
	nBits uint32

	// bytes[br:bw] holds bytes read from r but not yet loaded into bits.
	br    uint32
	bw    uint32
	bytes [1024]uint8
}

func (b *bitReader) alignToByteBoundary() {
	n := b.nBits & 7
	b.bits >>= n
	b.nBits -= n
}

// nextBitMaxNBits is the maximum possible value of bitReader.nBits after a
// bitReader.nextBit call, provided that bitReader.nBits was not more than this
// value before that call.
//
// Note that the decode function can unread bits, which can temporarily set the
// bitReader.nBits value above nextBitMaxNBits.
const nextBitMaxNBits = 31

func (b *bitReader) nextBit() (uint32, error) {
	for {
		if b.nBits > 0 {
			bit := uint32(b.bits) & 1
			b.bits >>= 1
			b.nBits--
			return bit, nil
		}

		if available := b.bw - b.br; available >= 4 {
			// Read 32 bits, even though b.bits is a uint64, since the decode
			// function may need to unread up to maxCodeLength bits, putting
			// them back in the remaining (64 - 32) bits. TestMaxCodeLength
			// checks that the generated maxCodeLength constant fits.
			//
			// If changing the Uint32 call, also change nextBitMaxNBits.
			b.bits = uint64(binary.LittleEndian.Uint32(b.bytes[b.br:]))
			b.br += 4
			b.nBits = 32
			continue
		} else if available > 0 {
			b.bits = uint64(b.bytes[b.br])
			b.br++
			b.nBits = 8
			continue
		}

		if b.readErr != nil {
			return 0, b.readErr
		}

		n, err := b.r.Read(b.bytes[:])
		b.br = 0
		b.bw = uint32(n)
		b.readErr = err

		if b.order != LSB {
			written := b.bytes[:b.bw]
			for i, x := range written {
				written[i] = bits.Reverse8(x)
			}
		}
	}
}

func decode(b *bitReader, decodeTable [][2]int16) (uint32, error) {
	nBitsRead, bitsRead, state := uint32(0), uint32(0), int32(1)
	for {
		bit, err := b.nextBit()
		if err != nil {
			return 0, err
		}
		bitsRead |= bit << nBitsRead
		nBitsRead++
		// The "&1" is redundant, but can eliminate a bounds check.
		state = int32(decodeTable[state][bit&1])
		if state < 0 {
			return uint32(^state), nil
		} else if state == 0 {
			// Unread the bits we've read, then return errInvalidCode.
			b.bits = (b.bits << nBitsRead) | uint64(bitsRead)
			b.nBits += nBitsRead
			return 0, errInvalidCode
		}
	}
}
