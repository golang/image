// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tiff

import (
	"encoding/binary"
	"image"
	"io"
	"sort"
)

// The TIFF format allows to choose the order of the different elements freely.
// The basic structure of a TIFF file written by this package is:
//
//   1. Header (8 bytes).
//   2. Image data.
//   3. Image File Directory (IFD).
//   4. "Pointer area" for larger entries in the IFD.

// We only write little-endian TIFF files.
var enc = binary.LittleEndian

// An ifdEntry is a single entry in an Image File Directory.
// A value of type dtRational is composed of two 32-bit values,
// thus data contains two uints (numerator and denominator) for a single number.
type ifdEntry struct {
	tag      int
	datatype int
	data     []uint32
}

func (e ifdEntry) putData(p []byte) {
	for _, d := range e.data {
		switch e.datatype {
		case dtByte, dtASCII:
			p[0] = byte(d)
			p = p[1:]
		case dtShort:
			enc.PutUint16(p, uint16(d))
			p = p[2:]
		case dtLong, dtRational:
			enc.PutUint32(p, uint32(d))
			p = p[4:]
		}
	}
}

type byTag []ifdEntry

func (d byTag) Len() int           { return len(d) }
func (d byTag) Less(i, j int) bool { return d[i].tag < d[j].tag }
func (d byTag) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

func writeImgData(w io.Writer, m image.Image) error {
	bounds := m.Bounds()
	buf := make([]byte, 4*bounds.Dx())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		i := 0
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := m.At(x, y).RGBA()
			buf[i+0] = uint8(r >> 8)
			buf[i+1] = uint8(g >> 8)
			buf[i+2] = uint8(b >> 8)
			buf[i+3] = uint8(a >> 8)
			i += 4
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func writeIFD(w io.Writer, ifdOffset int, d []ifdEntry) error {
	var buf [ifdLen]byte
	// Make space for "pointer area" containing IFD entry data
	// longer than 4 bytes.
	parea := make([]byte, 1024)
	pstart := ifdOffset + ifdLen*len(d) + 6
	var o int // Current offset in parea.

	// The IFD has to be written with the tags in ascending order.
	sort.Sort(byTag(d))

	// Write the number of entries in this IFD.
	if err := binary.Write(w, enc, uint16(len(d))); err != nil {
		return err
	}
	for _, ent := range d {
		enc.PutUint16(buf[0:2], uint16(ent.tag))
		enc.PutUint16(buf[2:4], uint16(ent.datatype))
		count := uint32(len(ent.data))
		if ent.datatype == dtRational {
			count /= 2
		}
		enc.PutUint32(buf[4:8], count)
		datalen := int(count * lengths[ent.datatype])
		if datalen <= 4 {
			ent.putData(buf[8:12])
		} else {
			if (o + datalen) > len(parea) {
				newlen := len(parea) + 1024
				for (o + datalen) > newlen {
					newlen += 1024
				}
				newarea := make([]byte, newlen)
				copy(newarea, parea)
				parea = newarea
			}
			ent.putData(parea[o : o+datalen])
			enc.PutUint32(buf[8:12], uint32(pstart+o))
			o += datalen
		}
		if _, err := w.Write(buf[:]); err != nil {
			return err
		}
	}
	// The IFD ends with the offset of the next IFD in the file,
	// or zero if it is the last one (page 14).
	if err := binary.Write(w, enc, uint32(0)); err != nil {
		return err
	}
	_, err := w.Write(parea[:o])
	return err
}

// Encode writes the image m to w in uncompressed RGBA format.
func Encode(w io.Writer, m image.Image) error {
	_, err := io.WriteString(w, leHeader)
	if err != nil {
		return err
	}

	bounds := m.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	// imageLen is the length of the image data in bytes.
	imageLen := width * height * 4
	ifdOffset := imageLen + 8 // 8 bytes for TIFF header.
	err = binary.Write(w, enc, uint32(ifdOffset))
	if err != nil {
		return err
	}
	err = writeImgData(w, m)
	if err != nil {
		return err
	}
	return writeIFD(w, ifdOffset, []ifdEntry{
		{tImageWidth, dtShort, []uint32{uint32(width)}},
		{tImageLength, dtShort, []uint32{uint32(height)}},
		{tBitsPerSample, dtShort, []uint32{8, 8, 8, 8}},
		{tCompression, dtShort, []uint32{cNone}},
		{tPhotometricInterpretation, dtShort, []uint32{pRGB}},
		{tStripOffsets, dtLong, []uint32{8}},
		{tSamplesPerPixel, dtShort, []uint32{4}},
		{tRowsPerStrip, dtShort, []uint32{uint32(height)}},
		{tStripByteCounts, dtLong, []uint32{uint32(imageLen)}},
		// There is currently no support for storing the image
		// resolution, so give a bogus value of 72x72 dpi.
		{tXResolution, dtRational, []uint32{72, 1}},
		{tYResolution, dtRational, []uint32{72, 1}},
		{tResolutionUnit, dtShort, []uint32{resPerInch}},
		{tExtraSamples, dtShort, []uint32{1}}, // RGBA.
	})
}
