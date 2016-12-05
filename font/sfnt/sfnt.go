// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sfnt implements a decoder for SFNT font file formats, including
// TrueType and OpenType.
package sfnt // import "golang.org/x/image/font/sfnt"

// This implementation was written primarily to the
// https://www.microsoft.com/en-us/Typography/OpenTypeSpecification.aspx
// specification. Additional documentation is at
// http://developer.apple.com/fonts/TTRefMan/
//
// The pyftinspect tool from https://github.com/fonttools/fonttools is useful
// for inspecting SFNT fonts.

import (
	"errors"
	"io"

	"golang.org/x/image/math/fixed"
)

// These constants are not part of the specifications, but are limitations used
// by this implementation.
const (
	maxHintBits         = 256
	maxNumTables        = 256
	maxRealNumberStrLen = 64 // Maximum length in bytes of the "-123.456E-7" representation.

	// (maxTableOffset + maxTableLength) will not overflow an int32.
	maxTableLength = 1 << 29
	maxTableOffset = 1 << 29
)

var (
	errGlyphIndexOutOfRange = errors.New("sfnt: glyph index out of range")

	errInvalidBounds        = errors.New("sfnt: invalid bounds")
	errInvalidCFFTable      = errors.New("sfnt: invalid CFF table")
	errInvalidHeadTable     = errors.New("sfnt: invalid head table")
	errInvalidLocationData  = errors.New("sfnt: invalid location data")
	errInvalidMaxpTable     = errors.New("sfnt: invalid maxp table")
	errInvalidSourceData    = errors.New("sfnt: invalid source data")
	errInvalidTableOffset   = errors.New("sfnt: invalid table offset")
	errInvalidTableTagOrder = errors.New("sfnt: invalid table tag order")
	errInvalidVersion       = errors.New("sfnt: invalid version")

	errUnsupportedCFFVersion         = errors.New("sfnt: unsupported CFF version")
	errUnsupportedRealNumberEncoding = errors.New("sfnt: unsupported real number encoding")
	errUnsupportedNumberOfHints      = errors.New("sfnt: unsupported number of hints")
	errUnsupportedNumberOfTables     = errors.New("sfnt: unsupported number of tables")
	errUnsupportedTableOffsetLength  = errors.New("sfnt: unsupported table offset or length")
	errUnsupportedType2Charstring    = errors.New("sfnt: unsupported Type 2 Charstring")
)

// GlyphIndex is a glyph index in a Font.
type GlyphIndex uint16

// Units are an integral number of abstract, scalable "font units". The em
// square is typically 1000 or 2048 "font units". This would map to a certain
// number (e.g. 30 pixels) of physical pixels, depending on things like the
// display resolution (DPI) and font size (e.g. a 12 point font).
type Units int32

func u16(b []byte) uint16 {
	_ = b[1] // Bounds check hint to compiler.
	return uint16(b[0])<<8 | uint16(b[1])<<0
}

func u32(b []byte) uint32 {
	_ = b[3] // Bounds check hint to compiler.
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])<<0
}

// source is a source of byte data. Conceptually, it is like an io.ReaderAt,
// except that a common source of SFNT font data is in-memory instead of
// on-disk: a []byte containing the entire data, either as a global variable
// (e.g. "goregular.TTF") or the result of an ioutil.ReadFile call. In such
// cases, as an optimization, we skip the io.Reader / io.ReaderAt model of
// copying from the source to a caller-supplied buffer, and instead provide
// direct access to the underlying []byte data.
type source struct {
	b []byte
	r io.ReaderAt

	// TODO: add a caching layer, if we're using the io.ReaderAt? Note that
	// this might make a source no longer safe to use concurrently.
}

// valid returns whether exactly one of s.b and s.r is nil.
func (s *source) valid() bool {
	return (s.b == nil) != (s.r == nil)
}

// viewBufferWritable returns whether the []byte returned by source.view can be
// written to by the caller, including by passing it to the same method
// (source.view) on other receivers (i.e. different sources).
//
// In other words, it returns whether the source's underlying data is an
// io.ReaderAt, not a []byte.
func (s *source) viewBufferWritable() bool {
	return s.b == nil
}

// view returns the length bytes at the given offset. buf is an optional
// scratch buffer to reduce allocations when calling view multiple times. A nil
// buf is valid. The []byte returned may be a sub-slice of buf[:cap(buf)], or
// it may be an unrelated slice. In any case, the caller should not modify the
// contents of the returned []byte, other than passing that []byte back to this
// method on the same source s.
func (s *source) view(buf []byte, offset, length int) ([]byte, error) {
	if 0 > offset || offset > offset+length {
		return nil, errInvalidBounds
	}

	// Try reading from the []byte.
	if s.b != nil {
		if offset+length > len(s.b) {
			return nil, errInvalidBounds
		}
		return s.b[offset : offset+length], nil
	}

	// Read from the io.ReaderAt.
	if length <= cap(buf) {
		buf = buf[:length]
	} else {
		// Round length up to the nearest KiB. The slack can lead to fewer
		// allocations if the buffer is re-used for multiple source.view calls.
		n := length
		n += 1023
		n &^= 1023
		buf = make([]byte, length, n)
	}
	if n, err := s.r.ReadAt(buf, int64(offset)); n != length {
		return nil, err
	}
	return buf, nil
}

// u16 returns the uint16 in the table t at the relative offset i.
//
// buf is an optional scratch buffer as per the source.view method.
func (s *source) u16(buf []byte, t table, i int) (uint16, error) {
	if i < 0 || uint(t.length) < uint(i+2) {
		return 0, errInvalidBounds
	}
	buf, err := s.view(buf, int(t.offset)+i, 2)
	if err != nil {
		return 0, err
	}
	return u16(buf), nil
}

// table is a section of the font data.
type table struct {
	offset, length uint32
}

// Parse parses an SFNT font from a []byte data source.
func Parse(src []byte) (*Font, error) {
	f := &Font{src: source{b: src}}
	if err := f.initialize(); err != nil {
		return nil, err
	}
	return f, nil
}

// ParseReaderAt parses an SFNT font from an io.ReaderAt data source.
func ParseReaderAt(src io.ReaderAt) (*Font, error) {
	f := &Font{src: source{r: src}}
	if err := f.initialize(); err != nil {
		return nil, err
	}
	return f, nil
}

// Font is an SFNT font.
//
// All of its methods are safe to call concurrently, although the method
// arguments may have further restrictions. For example, it is valid to have
// multiple concurrent Font.LoadGlyph calls to the same *Font receiver, as long
// as each call has a different Buffer.
type Font struct {
	src source

	// https://www.microsoft.com/typography/otspec/otff.htm#otttables
	// "Required Tables".
	cmap table
	head table
	hhea table
	hmtx table
	maxp table
	name table
	os2  table
	post table

	// https://www.microsoft.com/typography/otspec/otff.htm#otttables
	// "Tables Related to TrueType Outlines".
	//
	// This implementation does not support hinting, so it does not read the
	// cvt, fpgm gasp or prep tables.
	glyf table
	loca table

	// https://www.microsoft.com/typography/otspec/otff.htm#otttables
	// "Tables Related to PostScript Outlines".
	//
	// TODO: cff2, vorg?
	cff table

	// https://www.microsoft.com/typography/otspec/otff.htm#otttables
	// "Advanced Typographic Tables".
	//
	// TODO: base, gdef, gpos, gsub, jstf, math?

	// https://www.microsoft.com/typography/otspec/otff.htm#otttables
	// "Other OpenType Tables".
	//
	// TODO: hdmx, kern, vmtx? Others?

	cached struct {
		isPostScript bool
		unitsPerEm   Units

		// The glyph data for the glyph index i is in
		// src[locations[i+0]:locations[i+1]].
		locations []uint32
	}
}

// NumGlyphs returns the number of glyphs in f.
func (f *Font) NumGlyphs() int { return len(f.cached.locations) - 1 }

// UnitsPerEm returns the number of units per em for f.
func (f *Font) UnitsPerEm() Units { return f.cached.unitsPerEm }

func (f *Font) initialize() error {
	if !f.src.valid() {
		return errInvalidSourceData
	}
	var buf []byte

	// https://www.microsoft.com/typography/otspec/otff.htm "Organization of an
	// OpenType Font" says that "The OpenType font starts with the Offset
	// Table", which is 12 bytes.
	buf, err := f.src.view(buf, 0, 12)
	if err != nil {
		return err
	}
	switch u32(buf) {
	default:
		return errInvalidVersion
	case 0x00010000:
		// No-op.
	case 0x4f54544f: // "OTTO".
		f.cached.isPostScript = true
	}
	numTables := int(u16(buf[4:]))
	if numTables > maxNumTables {
		return errUnsupportedNumberOfTables
	}

	// "The Offset Table is followed immediately by the Table Record entries...
	// sorted in ascending order by tag", 16 bytes each.
	buf, err = f.src.view(buf, 12, 16*numTables)
	if err != nil {
		return err
	}
	for b, first, prevTag := buf, true, uint32(0); len(b) > 0; b = b[16:] {
		tag := u32(b)
		if first {
			first = false
		} else if tag <= prevTag {
			return errInvalidTableTagOrder
		}
		prevTag = tag

		o, n := u32(b[8:12]), u32(b[12:16])
		if o > maxTableOffset || n > maxTableLength {
			return errUnsupportedTableOffsetLength
		}
		// We ignore the checksums, but "all tables must begin on four byte
		// boundries [sic]".
		if o&3 != 0 {
			return errInvalidTableOffset
		}

		// Match the 4-byte tag as a uint32. For example, "OS/2" is 0x4f532f32.
		switch tag {
		case 0x43464620:
			f.cff = table{o, n}
		case 0x4f532f32:
			f.os2 = table{o, n}
		case 0x636d6170:
			f.cmap = table{o, n}
		case 0x676c7966:
			f.glyf = table{o, n}
		case 0x68656164:
			f.head = table{o, n}
		case 0x68686561:
			f.hhea = table{o, n}
		case 0x686d7478:
			f.hmtx = table{o, n}
		case 0x6c6f6361:
			f.loca = table{o, n}
		case 0x6d617870:
			f.maxp = table{o, n}
		case 0x6e616d65:
			f.name = table{o, n}
		case 0x706f7374:
			f.post = table{o, n}
		}
	}

	var u uint16

	// https://www.microsoft.com/typography/otspec/head.htm
	if f.head.length != 54 {
		return errInvalidHeadTable
	}
	u, err = f.src.u16(buf, f.head, 18)
	if err != nil {
		return err
	}
	if u == 0 {
		return errInvalidHeadTable
	}
	f.cached.unitsPerEm = Units(u)

	// https://www.microsoft.com/typography/otspec/maxp.htm
	if f.cached.isPostScript {
		if f.maxp.length != 6 {
			return errInvalidMaxpTable
		}
	} else {
		if f.maxp.length != 32 {
			return errInvalidMaxpTable
		}
	}
	u, err = f.src.u16(buf, f.maxp, 4)
	if err != nil {
		return err
	}
	numGlyphs := int(u)

	if f.cached.isPostScript {
		p := cffParser{
			src:    &f.src,
			base:   int(f.cff.offset),
			offset: int(f.cff.offset),
			end:    int(f.cff.offset + f.cff.length),
		}
		f.cached.locations, err = p.parse()
		if err != nil {
			return err
		}
	} else {
		// TODO: locaParser for TrueType fonts.
		f.cached.locations = make([]uint32, numGlyphs+1)
	}
	if len(f.cached.locations) != numGlyphs+1 {
		return errInvalidLocationData
	}
	return nil
}

// TODO: func (f *Font) GlyphIndex(r rune) (x GlyphIndex, ok bool)
// This will require parsing the cmap table.

func (f *Font) viewGlyphData(buf []byte, x GlyphIndex) ([]byte, error) {
	xx := int(x)
	if f.NumGlyphs() <= xx {
		return nil, errGlyphIndexOutOfRange
	}
	i := f.cached.locations[xx+0]
	j := f.cached.locations[xx+1]
	return f.src.view(buf, int(i), int(j-i))
}

// LoadGlyphOptions are the options to the Font.LoadGlyph method.
type LoadGlyphOptions struct {
	// TODO: scale / transform / hinting.
}

// LoadGlyph loads the glyphs vectors for the x'th glyph into b.Segments.
func (f *Font) LoadGlyph(b *Buffer, x GlyphIndex, opts *LoadGlyphOptions) error {
	buf, err := f.viewGlyphData(b.buf, x)
	if err != nil {
		return err
	}
	// Only update b.buf if it is safe to re-use buf.
	if f.src.viewBufferWritable() {
		b.buf = buf
	}

	b.Segments = b.Segments[:0]
	if f.cached.isPostScript {
		b.psi.type2Charstrings.initialize(b.Segments)
		if err := b.psi.run(psContextType2Charstring, buf); err != nil {
			return err
		}
		b.Segments = b.psi.type2Charstrings.segments
	} else {
		return errors.New("sfnt: TODO: load glyf data")
	}

	// TODO: look at opts to scale / transform / hint the Buffer.Segments.

	return nil
}

// Buffer holds the result of the Font.LoadGlyph method. It is valid to re-use
// a Buffer with multiple Font.LoadGlyph calls, even with different *Font
// receivers, as long as they are not concurrent calls.
//
// It is also valid to have multiple concurrent Font.LoadGlyph calls to the
// same *Font receiver, as long as each call has a different Buffer.
type Buffer struct {
	Segments []Segment
	// buf is a byte buffer for when a Font's source is an io.ReaderAt.
	buf []byte
	// psi is a PostScript interpreter for when the Font is an OpenType/CFF
	// font.
	psi psInterpreter
}

// Segment is a segment of a vector path.
type Segment struct {
	Op   SegmentOp
	Args [6]fixed.Int26_6
}

// SegmentOp is a vector path segment's operator.
type SegmentOp uint32

const (
	SegmentOpMoveTo SegmentOp = iota
	SegmentOpLineTo
	SegmentOpQuadTo
	SegmentOpCubeTo
)
