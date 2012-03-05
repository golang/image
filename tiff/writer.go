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

type ifd []ifdEntry

func (d ifd) Len() int {
	return len(d)
}

func (d ifd) Less(i, j int) bool {
	return d[i].tag < d[j].tag
}

func (d ifd) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}

type encoder struct {
	ifd      ifd
	img      image.Image
	imageLen int // Length of the image in bytes.
}

func newEncoder(m image.Image) *encoder {
	width := m.Bounds().Dx()
	height := m.Bounds().Dy()
	imageLen := width * height * 4
	return &encoder{
		img: m,
		// For uncompressed images, imageLen is known in advance.
		// For compressed images, we would need to write the image
		// data in a buffer here to get its length.
		imageLen: imageLen,
		ifd: ifd{
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
		},
	}
}

func (e *encoder) writeImgData(w io.Writer) error {
	b := e.img.Bounds()
	buf := make([]byte, 4*b.Dx())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := 0
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, a := e.img.At(x, y).RGBA()
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

func (e *encoder) writeIFD(w io.Writer) error {
	var buf [ifdLen]byte
	// Make space for "pointer area" containing IFD entry data
	// longer than 4 bytes.
	parea := make([]byte, 1024)
	pstart := int(e.imageLen) + 8 + (ifdLen * len(e.ifd)) + 6
	var o int // Current offset in parea.

	// The IFD has to be written with the tags in ascending order.
	sort.Sort(e.ifd)

	// Write the number of entries in this IFD.
	if err := binary.Write(w, enc, uint16(len(e.ifd))); err != nil {
		return err
	}
	for _, ent := range e.ifd {
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

func (e *encoder) encode(w io.Writer) error {
	_, err := io.WriteString(w, leHeader)
	if err != nil {
		return err
	}

	ifdOffset := e.imageLen + 8 // 8 bytes for TIFF header.
	err = binary.Write(w, enc, uint32(ifdOffset))
	if err != nil {
		return err
	}
	err = e.writeImgData(w)
	if err != nil {
		return err
	}
	return e.writeIFD(w)
}

// Encode writes the image m to w in uncompressed RGBA format.
func Encode(w io.Writer, m image.Image) error {
	return newEncoder(m).encode(w)
}
