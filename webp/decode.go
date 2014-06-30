// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package webp implements a decoder for WEBP images.
//
// WEBP is defined at:
// https://developers.google.com/speed/webp/docs/riff_container
package webp

import (
	"errors"
	"image"
	"image/color"
	"io"

	"code.google.com/p/go.image/vp8"
	"code.google.com/p/go.image/vp8l"
)

const (
	formatVP8  = 1
	formatVP8L = 2
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
	}
	if string(b[:4]) != "RIFF" || format == 0 {
		return nil, image.Config{}, errors.New("webp: invalid format")
	}
	riffLen := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24
	dataLen := uint32(b[16]) | uint32(b[17])<<8 | uint32(b[18])<<16 | uint32(b[19])<<24
	if riffLen < dataLen+12 {
		return nil, image.Config{}, errors.New("webp: invalid format")
	}
	if dataLen >= 1<<31 {
		return nil, image.Config{}, errors.New("webp: invalid format")
	}

	if format == formatVP8 {
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
		return m, image.Config{}, nil
	}

	r = &io.LimitedReader{R: r, N: int64(dataLen)}
	if configOnly {
		c, err := vp8l.DecodeConfig(r)
		return nil, c, err
	}
	m, err := vp8l.Decode(r)
	return m, image.Config{}, err
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
