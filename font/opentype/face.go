// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package opentype

import (
	"image"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// FaceOptions describes the possible options given to NewFace when
// creating a new font.Face from a sfnt.Font.
type FaceOptions struct {
	Size    float64      // Size is the font size in points
	DPI     float64      // DPI is the dots per inch resolution
	Hinting font.Hinting // Hinting selects how to quantize a vector font's glyph nodes
}

func defaultFaceOptions() *FaceOptions {
	return &FaceOptions{
		Size:    12,
		DPI:     72,
		Hinting: font.HintingNone,
	}
}

// Face implements the font.Face interface for sfnt.Font values.
type Face struct {
	f       *sfnt.Font
	hinting font.Hinting
	scale   fixed.Int26_6

	metrics    font.Metrics
	metricsSet bool

	buf  sfnt.Buffer
	rast vector.Rasterizer
	mask image.Alpha
}

// NewFace returns a new font.Face for the given sfnt.Font.
// if opts is nil, sensible defaults will be used.
func NewFace(f *sfnt.Font, opts *FaceOptions) (font.Face, error) {
	if opts == nil {
		opts = defaultFaceOptions()
	}
	face := &Face{
		f:       f,
		hinting: opts.Hinting,
		scale:   fixed.Int26_6(0.5 + (opts.Size * opts.DPI * 64 / 72)),
	}
	return face, nil
}

// Close satisfies the font.Face interface.
func (f *Face) Close() error {
	return nil
}

// Metrics satisfies the font.Face interface.
func (f *Face) Metrics() font.Metrics {
	if !f.metricsSet {
		var err error
		f.metrics, err = f.f.Metrics(&f.buf, f.scale, f.hinting)
		if err != nil {
			f.metrics = font.Metrics{}
		}
		f.metricsSet = true
	}
	return f.metrics
}

// Kern satisfies the font.Face interface.
func (f *Face) Kern(r0, r1 rune) fixed.Int26_6 {
	x0 := f.index(r0)
	x1 := f.index(r1)
	k, err := f.f.Kern(&f.buf, x0, x1, fixed.Int26_6(f.f.UnitsPerEm()), f.hinting)
	if err != nil {
		return 0
	}
	return k
}

// Glyph satisfies the font.Face interface.
func (f *Face) Glyph(dot fixed.Point26_6, r rune) (dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	x, err := f.f.GlyphIndex(&f.buf, r)
	if err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	// Call f.f.GlyphAdvance before f.f.LoadGlyph because the LoadGlyph docs
	// say this about the &f.buf argument: the segments become invalid to use
	// once [the buffer] is re-used.

	advance, err = f.f.GlyphAdvance(&f.buf, x, f.scale, f.hinting)
	if err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	segments, err := f.f.LoadGlyph(&f.buf, x, f.scale, nil)
	if err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	// Numerical notation used below:
	//  - 2    is an integer, "two"
	//  - 2:16 is a 26.6 fixed point number, "two and a quarter"
	//  - 2.5  is a float32 number, "two and a half"
	// Using 26.6 fixed point numbers means that there are 64 sub-pixel units
	// in 1 integer pixel unit.

	// Translate the sub-pixel bounding box from glyph space (where the glyph
	// origin is at (0:00, 0:00)) to dst space (where the glyph origin is at
	// the dot). dst space is the coordinate space that contains both the dot
	// (a sub-pixel position) and dr (an integer-pixel rectangle).
	dBounds := segments.Bounds().Add(dot)

	// Quantize the sub-pixel bounds (dBounds) to integer-pixel bounds (dr).
	dr.Min.X = dBounds.Min.X.Floor()
	dr.Min.Y = dBounds.Min.Y.Floor()
	dr.Max.X = dBounds.Max.X.Ceil()
	dr.Max.Y = dBounds.Max.Y.Ceil()
	width := dr.Dx()
	height := dr.Dy()
	if width < 0 || height < 0 {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	// Calculate the sub-pixel bias to convert from glyph space to rasterizer
	// space. In glyph space, the segments may be to the left or right and
	// above or below the glyph origin. In rasterizer space, the segments
	// should only be right and below (or equal to) the top-left corner (0.0,
	// 0.0). They should also be left and above (or equal to) the bottom-right
	// corner (width, height), as the rasterizer should enclose the glyph
	// bounding box.
	//
	// For example, suppose that dot.X was at the sub-pixel position 25:48,
	// three quarters of the way into the 26th pixel, and that bounds.Min.X was
	// 1:20. We then have dBounds.Min.X = 1:20 + 25:48 = 27:04, dr.Min.X = 27
	// and biasX = 25:48 - 27:00 = -1:16. A vertical stroke at 1:20 in glyph
	// space becomes (1:20 + -1:16) = 0:04 in rasterizer space. 0:04 as a
	// fixed.Int26_6 value is float32(4)/64.0 = 0.0625 as a float32 value.
	biasX := dot.X - fixed.Int26_6(dr.Min.X<<6)
	biasY := dot.Y - fixed.Int26_6(dr.Min.Y<<6)

	// Configure the mask image, re-allocating its buffer if necessary.
	nPixels := width * height
	if cap(f.mask.Pix) < nPixels {
		f.mask.Pix = make([]uint8, 2*nPixels)
	}
	f.mask.Pix = f.mask.Pix[:nPixels]
	f.mask.Stride = width
	f.mask.Rect.Min.X = 0
	f.mask.Rect.Min.Y = 0
	f.mask.Rect.Max.X = width
	f.mask.Rect.Max.Y = height

	// Rasterize the biased segments, converting from fixed.Int26_6 to float32.
	f.rast.Reset(width, height)
	f.rast.DrawOp = draw.Src
	for _, seg := range segments {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			f.rast.MoveTo(
				float32(seg.Args[0].X+biasX)/64,
				float32(seg.Args[0].Y+biasY)/64,
			)
		case sfnt.SegmentOpLineTo:
			f.rast.LineTo(
				float32(seg.Args[0].X+biasX)/64,
				float32(seg.Args[0].Y+biasY)/64,
			)
		case sfnt.SegmentOpQuadTo:
			f.rast.QuadTo(
				float32(seg.Args[0].X+biasX)/64,
				float32(seg.Args[0].Y+biasY)/64,
				float32(seg.Args[1].X+biasX)/64,
				float32(seg.Args[1].Y+biasY)/64,
			)
		case sfnt.SegmentOpCubeTo:
			f.rast.CubeTo(
				float32(seg.Args[0].X+biasX)/64,
				float32(seg.Args[0].Y+biasY)/64,
				float32(seg.Args[1].X+biasX)/64,
				float32(seg.Args[1].Y+biasY)/64,
				float32(seg.Args[2].X+biasX)/64,
				float32(seg.Args[2].Y+biasY)/64,
			)
		}
	}
	f.rast.Draw(&f.mask, f.mask.Bounds(), image.Opaque, image.Point{})

	return dr, &f.mask, f.mask.Rect.Min, advance, true
}

// GlyphBounds satisfies the font.Face interface.
func (f *Face) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	bounds, advance, err := f.f.GlyphBounds(&f.buf, f.index(r), f.scale, f.hinting)
	return bounds, advance, err == nil
}

// GlyphAdvance satisfies the font.Face interface.
func (f *Face) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	advance, err := f.f.GlyphAdvance(&f.buf, f.index(r), f.scale, f.hinting)
	return advance, err == nil
}

func (f *Face) index(r rune) sfnt.GlyphIndex {
	x, _ := f.f.GlyphIndex(&f.buf, r)
	return x
}
