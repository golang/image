// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package webp implements a decoder for WEBP images.
//
// WEBP is defined in the VP8 specification at:
// http://datatracker.ietf.org/doc/rfc6386/
package webp

import (
	"errors"
	"image"
	"image/color"
	"io"

	"code.google.com/p/go.image/vp8"
)

func decode(r io.Reader) (d *vp8.Decoder, fh vp8.FrameHeader, err error) {
	var b [20]byte
	if _, err = io.ReadFull(r, b[:]); err != nil {
		return
	}
	if string(b[0:4]) != "RIFF" || string(b[8:16]) != "WEBPVP8 " {
		err = errors.New("webp: invalid format")
		return
	}
	riffLen := uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24
	dataLen := uint32(b[16]) | uint32(b[17])<<8 | uint32(b[18])<<16 | uint32(b[19])<<24
	if riffLen < dataLen+12 {
		err = errors.New("webp: invalid format")
		return
	}
	if dataLen >= 1<<31 {
		err = errors.New("webp: invalid format")
		return
	}
	d = vp8.NewDecoder()
	d.Init(r, int(dataLen))
	fh, err = d.DecodeFrameHeader()
	if err != nil {
		d, fh = nil, vp8.FrameHeader{}
		return
	}
	return
}

// Decode reads a WEBP image from r and returns it as an image.Image.
func Decode(r io.Reader) (image.Image, error) {
	d, _, err := decode(r)
	if err != nil {
		return nil, err
	}
	return d.DecodeFrame()
}

// DecodeConfig returns the color model and dimensions of a WEBP image without
// decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	_, fh, err := decode(r)
	if err != nil {
		return image.Config{}, err
	}
	c := image.Config{
		ColorModel: color.YCbCrModel,
		Width:      fh.Width,
		Height:     fh.Height,
	}
	return c, nil
}

func init() {
	image.RegisterFormat("webp", "RIFF????WEBPVP8 ", Decode, DecodeConfig)
}
