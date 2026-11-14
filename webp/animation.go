// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"io"
	"time"

	"golang.org/x/image/riff"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
)

// Frame is a decoded frame of a WEBP image.
type Frame struct {
	// Image contains the frame's pixels, without compositing them onto earlier
	// frames. Its bounds give its position within the animation's canvas.
	Image image.Image

	// Duration is the time to display the frame. It is a multiple of a
	// millisecond, and may be zero. No minimum playback delay is imposed.
	Duration time.Duration

	// Blend reports whether the frame is alpha-blended onto the canvas.
	// If false, its pixels replace the corresponding canvas rectangle.
	Blend bool

	// DisposeToBackground reports whether the frame's rectangle is filled
	// with the animation's background color after displaying this frame,
	// before drawing the next frame.
	DisposeToBackground bool
}

// Animation contains the decoded frames and playback information of a WEBP
// image. A static image has one frame and IsAnimated set to false.
type Animation struct {
	// Frames holds the frames in display order. Frames are independent images;
	// they are not composited onto the background or onto each other.
	Frames []Frame

	// Config describes the canvas. For an animation, ColorModel is
	// color.NRGBAModel; individual frames may have different color models.
	Config image.Config

	// LoopCount is the total number of times to play the animation.
	// Zero means to loop forever. A static image has a LoopCount of 1.
	LoopCount int

	// BackgroundColor is used to initialize the canvas and to dispose of
	// frames whose DisposeToBackground field is true. It may be transparent.
	// A static image has a transparent black background.
	BackgroundColor color.NRGBA

	// IsAnimated reports whether the file is an animation, including an
	// animation containing only one frame.
	IsAnimated bool
}

// DecodeAll reads all the frames of a WEBP image from r.
// It returns the frames and the information needed to composite and play them.
// Unlike Decode, it does not composite frames onto the animation's background.
// For a static image, it returns one frame with zero Duration, Blend false,
// and DisposeToBackground false.
//
// DecodeAll retains every decoded frame in memory. DecodeConfig can be used
// to check the canvas dimensions before decoding, but not the frame count.
func DecodeAll(r io.Reader) (*Animation, error) {
	a := new(Animation)
	m, _, err := decode(r, false, a)
	if err != nil {
		return nil, err
	}
	if !a.IsAnimated {
		a.Frames = []Frame{{Image: m}}
		a.Config = image.Config{
			ColorModel: m.ColorModel(),
			Width:      m.Bounds().Dx(),
			Height:     m.Bounds().Dy(),
		}
		a.LoopCount = 1
	}
	return a, nil
}

// decodeAnimation reads the chunks following an animated VP8X header.
// If allFrames is false, it stops after decoding the first frame.
func decodeAnimation(r *riff.Reader, width, height int, allFrames bool) (*Animation, error) {
	a := &Animation{
		Config: image.Config{
			ColorModel: color.NRGBAModel,
			Width:      width,
			Height:     height,
		},
		IsAnimated: true,
	}
	seenANIM := false
	for {
		id, n, data, err := r.Next()
		if err == io.EOF && len(a.Frames) != 0 {
			return a, nil
		}
		if err != nil {
			if err == io.EOF {
				err = errInvalidFormat
			}
			return nil, err
		}
		switch id {
		case fccANIM:
			if seenANIM || n != 6 {
				return nil, errInvalidFormat
			}
			var buf [6]byte
			if _, err := io.ReadFull(data, buf[:]); err != nil {
				return nil, err
			}
			a.BackgroundColor = color.NRGBA{R: buf[2], G: buf[1], B: buf[0], A: buf[3]}
			a.LoopCount = int(buf[4]) | int(buf[5])<<8
			seenANIM = true

		case fccANMF:
			if !seenANIM {
				return nil, errInvalidFormat
			}
			f, err := decodeAnimationFrame(data, n, width, height)
			if err != nil {
				return nil, err
			}
			a.Frames = append(a.Frames, f)
			if !allFrames {
				return a, nil
			}

		case fccVP8X, fccVP8, fccVP8L, fccALPH:
			return nil, errInvalidFormat
		}
	}
}

func uint24(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}

// decodeAnimationFrame reads one ANMF payload. Its subchunk reader limits
// access to this frame, including when a bitstream decoder reads ahead.
func decodeAnimationFrame(data io.Reader, n uint32, canvasWidth, canvasHeight int) (Frame, error) {
	if n < 16 {
		return Frame{}, errInvalidFormat
	}
	var buf [16]byte
	if _, err := io.ReadFull(data, buf[:]); err != nil {
		return Frame{}, err
	}
	x, y := 2*uint24(buf[:3]), 2*uint24(buf[3:6])
	w, h := 1+uint24(buf[6:9]), 1+uint24(buf[9:12])
	if w > canvasWidth || h > canvasHeight || x > canvasWidth-w || y > canvasHeight-h {
		return Frame{}, errInvalidFormat
	}
	// Neither VP8 nor VP8L can encode a dimension larger than 16384. Check
	// before decoding ALPH, whose uncompressed form allocates w*h bytes.
	if w > 1<<14 || h > 1<<14 {
		return Frame{}, errInvalidFormat
	}
	f := Frame{
		Duration:            time.Duration(uint24(buf[12:15])) * time.Millisecond,
		Blend:               buf[15]&2 == 0,
		DisposeToBackground: buf[15]&1 != 0,
	}
	// ANMF contains RIFF subchunks but has no LIST type. Supply one without
	// copying the compressed frame. The Reader enforces the ANMF boundary.
	_, chunks, err := riff.NewListReader(n-12, io.MultiReader(bytes.NewReader(fccWEBP[:]), data))
	if err != nil {
		return Frame{}, err
	}
	var (
		m           image.Image
		alpha       []byte
		alphaStride int
	)
	for {
		id, size, payload, err := chunks.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Frame{}, err
		}
		switch id {
		case fccALPH:
			if alpha != nil || m != nil {
				return Frame{}, errInvalidFormat
			}
			if _, err := io.ReadFull(payload, buf[:1]); err != nil {
				return Frame{}, err
			}
			if buf[0]&3 == 0 && uint64(size-1) < uint64(w)*uint64(h) {
				return Frame{}, errInvalidFormat
			}
			alpha, alphaStride, err = readAlpha(payload, uint32(w-1), uint32(h-1), buf[0]&3)
			if err != nil {
				return Frame{}, err
			}
			unfilterAlpha(alpha, alphaStride, (buf[0]>>2)&3)

		case fccVP8:
			if m != nil || int32(size) < 0 {
				return Frame{}, errInvalidFormat
			}
			d := vp8.NewDecoder()
			d.Init(payload, int(size))
			fh, err := d.DecodeFrameHeader()
			if err != nil {
				return Frame{}, err
			}
			if fh.Width != w || fh.Height != h {
				return Frame{}, errInvalidFormat
			}
			img, err := d.DecodeFrame()
			if err != nil {
				return Frame{}, err
			}
			img.Rect = img.Rect.Add(image.Pt(x, y))
			m = img
			if alpha != nil {
				m = &image.NYCbCrA{YCbCr: *img, A: alpha, AStride: alphaStride}
			}

		case fccVP8L:
			if m != nil || alpha != nil {
				return Frame{}, errInvalidFormat
			}
			// Check the bitstream dimensions before allocating its pixels.
			if _, err := io.ReadFull(payload, buf[:5]); err != nil {
				return Frame{}, err
			}
			c, err := vp8l.DecodeConfig(bytes.NewReader(buf[:5]))
			if err != nil {
				return Frame{}, err
			}
			if c.Width != w || c.Height != h {
				return Frame{}, errInvalidFormat
			}
			m, err = vp8l.Decode(io.MultiReader(bytes.NewReader(buf[:5]), payload))
			if err != nil {
				return Frame{}, err
			}
			img := m.(*image.NRGBA)
			img.Rect = img.Rect.Add(image.Pt(x, y))

		case fccANIM, fccANMF, fccVP8X:
			return Frame{}, errInvalidFormat
		}
	}
	if m == nil {
		return Frame{}, errInvalidFormat
	}
	f.Image = m
	return f, nil
}

// firstFrame composites the first decoded frame onto the initial canvas.
func firstFrame(a *Animation) (image.Image, error) {
	w, h := a.Config.Width, a.Config.Height
	if uint64(w)*uint64(h) > uint64(int(^uint(0)>>1)/4) {
		return nil, errInvalidFormat
	}
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(m, m.Bounds(), image.NewUniform(a.BackgroundColor), image.Point{}, draw.Src)
	f := a.Frames[0]
	op := draw.Src
	if f.Blend {
		op = draw.Over
	}
	draw.Draw(m, f.Image.Bounds(), f.Image, f.Image.Bounds().Min, op)
	return m, nil
}
