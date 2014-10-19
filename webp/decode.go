// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package webp implements a decoder for WEBP images.
//
// WEBP is defined at:
// https://developers.google.com/speed/webp/docs/riff_container
package webp

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"io"

	"code.google.com/p/go.image/vp8"
	"code.google.com/p/go.image/vp8l"
	"code.google.com/p/go.image/webp/nycbcra"
)

// roundUp2 rounds u up to an even number.
// https://developers.google.com/speed/webp/docs/riff_container#riff_file_format
// says that "If Chunk Size is odd, a single padding byte... is added."
func roundUp2(u uint32) uint32 {
	return u + u&1
}

const (
	formatVP8  = 1
	formatVP8L = 2
	formatVP8X = 3
)

func decode(r io.Reader, configOnly bool) (image.Image, image.Config, error) {
	var b [20]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return nil, image.Config{}, err
	}
	format := 0
	switch string(b[8:16]) {
	case "WEBPVP8 ":
		format = formatVP8
	case "WEBPVP8L":
		format = formatVP8L
	case "WEBPVP8X":
		format = formatVP8X
	}
	if string(b[:4]) != "RIFF" || format == 0 {
		return nil, image.Config{}, errors.New("webp: invalid format")
	}
	riffLen := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24
	dataLen := roundUp2(uint32(b[16]) | uint32(b[17])<<8 | uint32(b[18])<<16 | uint32(b[19])<<24)
	if riffLen < dataLen+12 {
		return nil, image.Config{}, errors.New("webp: invalid format")
	}
	if dataLen == 0 || dataLen >= 1<<31 {
		return nil, image.Config{}, errors.New("webp: invalid format")
	}

	if format == formatVP8L {
		r = &io.LimitedReader{R: r, N: int64(dataLen)}
		if configOnly {
			c, err := vp8l.DecodeConfig(r)
			return nil, c, err
		}
		m, err := vp8l.Decode(r)
		return m, image.Config{}, err
	}

	var (
		alpha       []byte
		alphaStride int
	)
	if format == formatVP8X {
		if dataLen != 10 {
			return nil, image.Config{}, errors.New("webp: invalid format")
		}
		if _, err := io.ReadFull(r, b[:10]); err != nil {
			return nil, image.Config{}, err
		}
		const (
			animationBit    = 1 << 1
			xmpMetadataBit  = 1 << 2
			exifMetadataBit = 1 << 3
			alphaBit        = 1 << 4
			iccProfileBit   = 1 << 5
		)
		if b[0] != alphaBit {
			return nil, image.Config{}, errors.New("webp: non-Alpha VP8X is not implemented")
		}
		widthMinusOne := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16
		heightMinusOne := uint32(b[7]) | uint32(b[8])<<8 | uint32(b[9])<<16
		if configOnly {
			return nil, image.Config{
				ColorModel: nycbcra.ColorModel,
				Width:      int(widthMinusOne) + 1,
				Height:     int(heightMinusOne) + 1,
			}, nil
		}

		// Read the 8-byte chunk header plus the mandatory PFC (Pre-processing,
		// Filter, Compression) byte.
		if _, err := io.ReadFull(r, b[:9]); err != nil {
			return nil, image.Config{}, err
		}
		if b[0] != 'A' || b[1] != 'L' || b[2] != 'P' || b[3] != 'H' {
			return nil, image.Config{}, errors.New("webp: invalid format")
		}
		chunkLen := roundUp2(uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24)
		// Subtract one byte from chunkLen, since we've already read the PFC byte.
		if chunkLen == 0 {
			return nil, image.Config{}, errors.New("webp: invalid format")
		}
		chunkLen--
		filter := (b[8] >> 2) & 0x03
		if filter != 0 {
			return nil, image.Config{}, errors.New("webp: VP8X Alpha filtering != 0 is not implemented")
		}
		compression := b[8] & 0x03
		if compression != 1 {
			return nil, image.Config{}, errors.New("webp: VP8X Alpha compression != 1 is not implemented")
		}

		// Read the VP8L-compressed alpha values. First, synthesize a 5-byte VP8L header:
		// a 1-byte magic number, a 14-bit widthMinusOne, a 14-bit heightMinusOne,
		// a 1-bit (ignored, zero) alphaIsUsed and a 3-bit (zero) version.
		// TODO(nigeltao): be more efficient than decoding an *image.NRGBA just to
		// extract the green values to a separately allocated []byte. Fixing this
		// will require changes to the vp8l package's API.
		if widthMinusOne > 0x3fff || heightMinusOne > 0x3fff {
			return nil, image.Config{}, errors.New("webp: invalid format")
		}
		b[0] = 0x2f // VP8L magic number.
		b[1] = uint8(widthMinusOne)
		b[2] = uint8(widthMinusOne>>8) | uint8(heightMinusOne<<6)
		b[3] = uint8(heightMinusOne >> 2)
		b[4] = uint8(heightMinusOne >> 10)
		alphaImage, err := vp8l.Decode(io.MultiReader(
			bytes.NewReader(b[:5]),
			&io.LimitedReader{R: r, N: int64(chunkLen)},
		))
		if err != nil {
			return nil, image.Config{}, err
		}
		// The green values of the inner NRGBA image are the alpha values of the outer NYCbCrA image.
		pix := alphaImage.(*image.NRGBA).Pix
		alpha = make([]byte, len(pix)/4)
		for i := range alpha {
			alpha[i] = pix[4*i+1]
		}
		alphaStride = int(widthMinusOne) + 1

		// The rest of the image should be in the lossy format. Check the "VP8 "
		// header and fall through.
		if _, err := io.ReadFull(r, b[:8]); err != nil {
			return nil, image.Config{}, err
		}
		if b[0] != 'V' || b[1] != 'P' || b[2] != '8' || b[3] != ' ' {
			return nil, image.Config{}, errors.New("webp: invalid format")
		}
		dataLen = roundUp2(uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24)
		if dataLen == 0 || dataLen >= 1<<31 {
			return nil, image.Config{}, errors.New("webp: invalid format")
		}
	}

	d := vp8.NewDecoder()
	d.Init(r, int(dataLen))
	fh, err := d.DecodeFrameHeader()
	if err != nil {
		return nil, image.Config{}, err
	}
	if configOnly {
		return nil, image.Config{
			ColorModel: color.YCbCrModel,
			Width:      fh.Width,
			Height:     fh.Height,
		}, nil
	}
	m, err := d.DecodeFrame()
	if err != nil {
		return nil, image.Config{}, err
	}
	if alpha != nil {
		return &nycbcra.Image{
			YCbCr:   *m,
			A:       alpha,
			AStride: alphaStride,
		}, image.Config{}, nil
	}
	return m, image.Config{}, nil
}

// Decode reads a WEBP image from r and returns it as an image.Image.
func Decode(r io.Reader) (image.Image, error) {
	m, _, err := decode(r, false)
	if err != nil {
		return nil, err
	}
	return m, err
}

// DecodeConfig returns the color model and dimensions of a WEBP image without
// decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	_, c, err := decode(r, true)
	return c, err
}

func init() {
	image.RegisterFormat("webp", "RIFF????WEBPVP8", Decode, DecodeConfig)
}
