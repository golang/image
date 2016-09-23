// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vector

// TODO: add tests for NaN and Inf coordinates.

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"testing"

	"golang.org/x/image/math/f32"
)

// encodePNG is useful for manually debugging the tests.
func encodePNG(dstFilename string, src image.Image) error {
	f, err := os.Create(dstFilename)
	if err != nil {
		return err
	}
	encErr := png.Encode(f, src)
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	return closeErr
}

var basicMask = []byte{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xe3, 0xaa, 0x3e, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfa, 0x5f, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfc, 0x24, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xa1, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfc, 0x14, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x4a, 0x00, 0x00,
	0x00, 0x00, 0xcc, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x81, 0x00, 0x00,
	0x00, 0x00, 0x66, 0xff, 0xff, 0xff, 0xff, 0xff, 0xef, 0xe4, 0xff, 0xff, 0xff, 0xb6, 0x00, 0x00,
	0x00, 0x00, 0x0c, 0xf2, 0xff, 0xff, 0xfe, 0x9e, 0x15, 0x00, 0x15, 0x96, 0xff, 0xce, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x88, 0xfc, 0xe3, 0x43, 0x00, 0x00, 0x00, 0x00, 0x06, 0xcd, 0xdc, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x10, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x25, 0xde, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x56, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

func basicRasterizer() *Rasterizer {
	z := NewRasterizer(16, 16)
	z.MoveTo(f32.Vec2{2, 2})
	z.LineTo(f32.Vec2{8, 2})
	z.QuadTo(f32.Vec2{14, 2}, f32.Vec2{14, 14})
	z.CubeTo(f32.Vec2{8, 2}, f32.Vec2{5, 20}, f32.Vec2{2, 8})
	z.ClosePath()
	return z
}

func TestBasicPathDstAlpha(t *testing.T) {
	for _, background := range []uint8{0x00, 0x80} {
		for _, op := range []draw.Op{draw.Over, draw.Src} {
			z := basicRasterizer()
			dst := image.NewAlpha(z.Bounds())
			for i := range dst.Pix {
				dst.Pix[i] = background
			}
			z.DrawOp = op
			z.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
			got := dst.Pix

			want := make([]byte, len(basicMask))
			if op == draw.Over && background == 0x80 {
				for i, ma := range basicMask {
					want[i] = 0xff - (0xff-ma)/2
				}
			} else {
				copy(want, basicMask)
			}

			if len(got) != len(want) {
				t.Errorf("background=%#02x, op=%v: len(got)=%d and len(want)=%d differ",
					background, op, len(got), len(want))
				continue
			}
			for i := range got {
				delta := int(got[i]) - int(want[i])
				// The +/- 2 allows different implementations to give different
				// rounding errors.
				if delta < -2 || +2 < delta {
					t.Errorf("background=%#02x, op=%v: i=%d: got %#02x, want %#02x",
						background, op, i, got[i], want[i])
				}
			}
		}
	}
}

func TestBasicPathDstRGBA(t *testing.T) {
	blue := color.RGBA{0x00, 0x00, 0xff, 0xff}

	for _, op := range []draw.Op{draw.Over, draw.Src} {
		z := basicRasterizer()
		dst := image.NewRGBA(z.Bounds())
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				dst.SetRGBA(x, y, color.RGBA{
					R: uint8(y*0x11) / 2,
					G: uint8(x*0x11) / 2,
					B: 0x00,
					A: 0x80,
				})
			}
		}
		z.DrawOp = op
		z.Draw(dst, dst.Bounds(), image.NewUniform(blue), image.Point{})
		got := dst.Pix

		want := make([]byte, len(basicMask)*4)
		if op == draw.Over {
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					i := 16*y + x
					ma := basicMask[i]
					want[4*i+0] = uint8((uint32(0xff-ma) * uint32(y*0x11/2)) / 0xff)
					want[4*i+1] = uint8((uint32(0xff-ma) * uint32(x*0x11/2)) / 0xff)
					want[4*i+2] = ma
					want[4*i+3] = ma/2 + 0x80
				}
			}
		} else {
			for y := 0; y < 16; y++ {
				for x := 0; x < 16; x++ {
					i := 16*y + x
					ma := basicMask[i]
					want[4*i+0] = 0x00
					want[4*i+1] = 0x00
					want[4*i+2] = ma
					want[4*i+3] = ma
				}
			}
		}

		if len(got) != len(want) {
			t.Errorf("op=%v: len(got)=%d and len(want)=%d differ", op, len(got), len(want))
			continue
		}
		for i := range got {
			delta := int(got[i]) - int(want[i])
			// The +/- 2 allows different implementations to give different
			// rounding errors.
			if delta < -2 || +2 < delta {
				t.Errorf("op=%v: i=%d: got %#02x, want %#02x", op, i, got[i], want[i])
			}
		}
	}
}

const (
	benchmarkGlyphWidth  = 893
	benchmarkGlyphHeight = 1122
)

// benchmarkGlyphData is the 'a' glyph from the Roboto Regular font, translated
// so that its top left corner is (0, 0).
var benchmarkGlyphData = []struct {
	// n being 0, 1 or 2 means moveTo, lineTo or quadTo.
	n uint32
	p f32.Vec2
	q f32.Vec2
}{
	{0, f32.Vec2{699, 1102}, f32.Vec2{0, 0}},
	{2, f32.Vec2{683, 1070}, f32.Vec2{673, 988}},
	{2, f32.Vec2{544, 1122}, f32.Vec2{365, 1122}},
	{2, f32.Vec2{205, 1122}, f32.Vec2{102.5, 1031.5}},
	{2, f32.Vec2{0, 941}, f32.Vec2{0, 802}},
	{2, f32.Vec2{0, 633}, f32.Vec2{128.5, 539.5}},
	{2, f32.Vec2{257, 446}, f32.Vec2{490, 446}},
	{1, f32.Vec2{670, 446}, f32.Vec2{0, 0}},
	{1, f32.Vec2{670, 361}, f32.Vec2{0, 0}},
	{2, f32.Vec2{670, 264}, f32.Vec2{612, 206.5}},
	{2, f32.Vec2{554, 149}, f32.Vec2{441, 149}},
	{2, f32.Vec2{342, 149}, f32.Vec2{275, 199}},
	{2, f32.Vec2{208, 249}, f32.Vec2{208, 320}},
	{1, f32.Vec2{22, 320}, f32.Vec2{0, 0}},
	{2, f32.Vec2{22, 239}, f32.Vec2{79.5, 163.5}},
	{2, f32.Vec2{137, 88}, f32.Vec2{235.5, 44}},
	{2, f32.Vec2{334, 0}, f32.Vec2{452, 0}},
	{2, f32.Vec2{639, 0}, f32.Vec2{745, 93.5}},
	{2, f32.Vec2{851, 187}, f32.Vec2{855, 351}},
	{1, f32.Vec2{855, 849}, f32.Vec2{0, 0}},
	{2, f32.Vec2{855, 998}, f32.Vec2{893, 1086}},
	{1, f32.Vec2{893, 1102}, f32.Vec2{0, 0}},
	{1, f32.Vec2{699, 1102}, f32.Vec2{0, 0}},
	{0, f32.Vec2{392, 961}, f32.Vec2{0, 0}},
	{2, f32.Vec2{479, 961}, f32.Vec2{557, 916}},
	{2, f32.Vec2{635, 871}, f32.Vec2{670, 799}},
	{1, f32.Vec2{670, 577}, f32.Vec2{0, 0}},
	{1, f32.Vec2{525, 577}, f32.Vec2{0, 0}},
	{2, f32.Vec2{185, 577}, f32.Vec2{185, 776}},
	{2, f32.Vec2{185, 863}, f32.Vec2{243, 912}},
	{2, f32.Vec2{301, 961}, f32.Vec2{392, 961}},
}

// benchGlyph benchmarks rasterizing a TrueType glyph.
//
// Note that, compared to the github.com/google/font-go prototype, the height
// here is the height of the bounding box, not the pixels per em used to scale
// a glyph's vectors. A height of 64 corresponds to a ppem greater than 64.
func benchGlyph(b *testing.B, cm color.Model, height int, op draw.Op) {
	scale := float32(height) / benchmarkGlyphHeight

	// Clone the benchmarkGlyphData slice and scale its coordinates.
	data := append(benchmarkGlyphData[:0:0], benchmarkGlyphData...)
	for i := range data {
		data[i].p[0] *= scale
		data[i].p[1] *= scale
		data[i].q[0] *= scale
		data[i].q[1] *= scale
	}

	width := int(math.Ceil(float64(benchmarkGlyphWidth * scale)))
	z := NewRasterizer(width, height)

	dst, src := draw.Image(nil), image.Image(nil)
	switch cm {
	case color.AlphaModel:
		dst = image.NewAlpha(z.Bounds())
		src = image.Opaque
	case color.NRGBAModel:
		dst = image.NewNRGBA(z.Bounds())
		src = image.NewUniform(color.NRGBA{0x40, 0x80, 0xc0, 0xff})
	case color.RGBAModel:
		dst = image.NewRGBA(z.Bounds())
		src = image.NewUniform(color.RGBA{0x40, 0x80, 0xc0, 0xff})
	default:
		b.Fatal("unsupported color model")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		z.Reset(width, height)
		z.DrawOp = op
		for _, d := range data {
			switch d.n {
			case 0:
				z.MoveTo(d.p)
			case 1:
				z.LineTo(d.p)
			case 2:
				z.QuadTo(d.p, d.q)
			}
		}
		z.Draw(dst, dst.Bounds(), src, image.Point{})
	}
}

func BenchmarkGlyphAlpha16Over(b *testing.B)  { benchGlyph(b, color.AlphaModel, 16, draw.Over) }
func BenchmarkGlyphAlpha16Src(b *testing.B)   { benchGlyph(b, color.AlphaModel, 16, draw.Src) }
func BenchmarkGlyphAlpha32Over(b *testing.B)  { benchGlyph(b, color.AlphaModel, 32, draw.Over) }
func BenchmarkGlyphAlpha32Src(b *testing.B)   { benchGlyph(b, color.AlphaModel, 32, draw.Src) }
func BenchmarkGlyphAlpha64Over(b *testing.B)  { benchGlyph(b, color.AlphaModel, 64, draw.Over) }
func BenchmarkGlyphAlpha64Src(b *testing.B)   { benchGlyph(b, color.AlphaModel, 64, draw.Src) }
func BenchmarkGlyphAlpha128Over(b *testing.B) { benchGlyph(b, color.AlphaModel, 128, draw.Over) }
func BenchmarkGlyphAlpha128Src(b *testing.B)  { benchGlyph(b, color.AlphaModel, 128, draw.Src) }
func BenchmarkGlyphAlpha256Over(b *testing.B) { benchGlyph(b, color.AlphaModel, 256, draw.Over) }
func BenchmarkGlyphAlpha256Src(b *testing.B)  { benchGlyph(b, color.AlphaModel, 256, draw.Src) }

func BenchmarkGlyphNRGBA16Over(b *testing.B)  { benchGlyph(b, color.NRGBAModel, 16, draw.Over) }
func BenchmarkGlyphNRGBA16Src(b *testing.B)   { benchGlyph(b, color.NRGBAModel, 16, draw.Src) }
func BenchmarkGlyphNRGBA32Over(b *testing.B)  { benchGlyph(b, color.NRGBAModel, 32, draw.Over) }
func BenchmarkGlyphNRGBA32Src(b *testing.B)   { benchGlyph(b, color.NRGBAModel, 32, draw.Src) }
func BenchmarkGlyphNRGBA64Over(b *testing.B)  { benchGlyph(b, color.NRGBAModel, 64, draw.Over) }
func BenchmarkGlyphNRGBA64Src(b *testing.B)   { benchGlyph(b, color.NRGBAModel, 64, draw.Src) }
func BenchmarkGlyphNRGBA128Over(b *testing.B) { benchGlyph(b, color.NRGBAModel, 128, draw.Over) }
func BenchmarkGlyphNRGBA128Src(b *testing.B)  { benchGlyph(b, color.NRGBAModel, 128, draw.Src) }
func BenchmarkGlyphNRGBA256Over(b *testing.B) { benchGlyph(b, color.NRGBAModel, 256, draw.Over) }
func BenchmarkGlyphNRGBA256Src(b *testing.B)  { benchGlyph(b, color.NRGBAModel, 256, draw.Src) }

func BenchmarkGlyphRGBA16Over(b *testing.B)  { benchGlyph(b, color.RGBAModel, 16, draw.Over) }
func BenchmarkGlyphRGBA16Src(b *testing.B)   { benchGlyph(b, color.RGBAModel, 16, draw.Src) }
func BenchmarkGlyphRGBA32Over(b *testing.B)  { benchGlyph(b, color.RGBAModel, 32, draw.Over) }
func BenchmarkGlyphRGBA32Src(b *testing.B)   { benchGlyph(b, color.RGBAModel, 32, draw.Src) }
func BenchmarkGlyphRGBA64Over(b *testing.B)  { benchGlyph(b, color.RGBAModel, 64, draw.Over) }
func BenchmarkGlyphRGBA64Src(b *testing.B)   { benchGlyph(b, color.RGBAModel, 64, draw.Src) }
func BenchmarkGlyphRGBA128Over(b *testing.B) { benchGlyph(b, color.RGBAModel, 128, draw.Over) }
func BenchmarkGlyphRGBA128Src(b *testing.B)  { benchGlyph(b, color.RGBAModel, 128, draw.Src) }
func BenchmarkGlyphRGBA256Over(b *testing.B) { benchGlyph(b, color.RGBAModel, 256, draw.Over) }
func BenchmarkGlyphRGBA256Src(b *testing.B)  { benchGlyph(b, color.RGBAModel, 256, draw.Src) }
