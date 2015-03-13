// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"golang.org/x/image/math/f64"

	_ "image/jpeg"
)

var genGoldenFiles = flag.Bool("gen_golden_files", false, "whether to generate the TestXxx golden files.")

var transformMatrix = func() *f64.Aff3 {
	const scale, cos30, sin30 = 3.75, 0.866025404, 0.5
	return &f64.Aff3{
		+scale * cos30, -scale * sin30, 40,
		+scale * sin30, +scale * cos30, 10,
	}
}()

// testInterp tests that interpolating the source image gives the exact
// destination image. This is to ensure that any refactoring or optimization of
// the interpolation code doesn't change the behavior. Changing the actual
// algorithm or kernel used by any particular quality setting will obviously
// change the resultant pixels. In such a case, use the gen_golden_files flag
// to regenerate the golden files.
func testInterp(t *testing.T, w int, h int, direction, srcFilename string) {
	f, err := os.Open("../testdata/go-turns-two-" + srcFilename)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	testCases := map[string]Interpolator{
		"nn": NearestNeighbor,
		"ab": ApproxBiLinear,
		"bl": BiLinear,
		"cr": CatmullRom,
	}
	for name, q := range testCases {
		goldenFilename := fmt.Sprintf("../testdata/go-turns-two-%s-%s.png", direction, name)

		got := image.NewRGBA(image.Rect(0, 0, w, h))
		if direction == "rotate" {
			if name == "bl" || name == "cr" {
				// TODO: implement Kernel.Transform.
				continue
			}
			q.Transform(got, transformMatrix, src, src.Bounds(), nil)
		} else {
			q.Scale(got, got.Bounds(), src, src.Bounds(), nil)
		}

		if *genGoldenFiles {
			g, err := os.Create(goldenFilename)
			if err != nil {
				t.Errorf("Create: %v", err)
				continue
			}
			defer g.Close()
			if err := png.Encode(g, got); err != nil {
				t.Errorf("Encode: %v", err)
				continue
			}
			continue
		}

		g, err := os.Open(goldenFilename)
		if err != nil {
			t.Errorf("Open: %v", err)
			continue
		}
		defer g.Close()
		wantRaw, err := png.Decode(g)
		if err != nil {
			t.Errorf("Decode: %v", err)
			continue
		}
		// convert wantRaw to RGBA.
		want, ok := wantRaw.(*image.RGBA)
		if !ok {
			b := wantRaw.Bounds()
			want = image.NewRGBA(b)
			Draw(want, b, wantRaw, b.Min, Src)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: actual image differs from golden image", goldenFilename)
			continue
		}
	}
}

func TestScaleDown(t *testing.T) { testInterp(t, 100, 100, "down", "280x360.jpeg") }
func TestScaleUp(t *testing.T)   { testInterp(t, 75, 100, "up", "14x18.png") }
func TestTransform(t *testing.T) { testInterp(t, 100, 100, "rotate", "14x18.png") }

func fillPix(r *rand.Rand, pixs ...[]byte) {
	for _, pix := range pixs {
		for i := range pix {
			pix[i] = uint8(r.Intn(256))
		}
	}
}

func TestInterpClipCommute(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	fillPix(rand.New(rand.NewSource(0)), src.Pix)

	outer := image.Rect(1, 1, 8, 5)
	inner := image.Rect(2, 3, 6, 5)
	qs := []Interpolator{
		NearestNeighbor,
		ApproxBiLinear,
		CatmullRom,
	}
	for _, transform := range []bool{false, true} {
		for _, q := range qs {
			if transform && q == CatmullRom {
				// TODO: implement Kernel.Transform.
				continue
			}

			dst0 := image.NewRGBA(image.Rect(1, 1, 10, 10))
			dst1 := image.NewRGBA(image.Rect(1, 1, 10, 10))
			for i := range dst0.Pix {
				dst0.Pix[i] = uint8(i / 4)
				dst1.Pix[i] = uint8(i / 4)
			}

			var interp func(dst *image.RGBA)
			if transform {
				interp = func(dst *image.RGBA) {
					q.Transform(dst, transformMatrix, src, src.Bounds(), nil)
				}
			} else {
				interp = func(dst *image.RGBA) {
					q.Scale(dst, outer, src, src.Bounds(), nil)
				}
			}

			// Interpolate then clip.
			interp(dst0)
			dst0 = dst0.SubImage(inner).(*image.RGBA)

			// Clip then interpolate.
			dst1 = dst1.SubImage(inner).(*image.RGBA)
			interp(dst1)

		loop:
			for y := inner.Min.Y; y < inner.Max.Y; y++ {
				for x := inner.Min.X; x < inner.Max.X; x++ {
					if c0, c1 := dst0.RGBAAt(x, y), dst1.RGBAAt(x, y); c0 != c1 {
						t.Errorf("q=%T: at (%d, %d): c0=%v, c1=%v", q, x, y, c0, c1)
						break loop
					}
				}
			}
		}
	}
}

// translatedImage is an image m translated by t.
type translatedImage struct {
	m image.Image
	t image.Point
}

func (t *translatedImage) At(x, y int) color.Color { return t.m.At(x-t.t.X, y-t.t.Y) }
func (t *translatedImage) Bounds() image.Rectangle { return t.m.Bounds().Add(t.t) }
func (t *translatedImage) ColorModel() color.Model { return t.m.ColorModel() }

// TestSrcTranslationInvariance tests that Scale and Transform are invariant
// under src translations. Specifically, when some source pixels are not in the
// bottom-right quadrant of src coordinate space, we consistently round down,
// not round towards zero.
func TestSrcTranslationInvariance(t *testing.T) {
	f, err := os.Open("../testdata/testpattern.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	qs := []Interpolator{
		NearestNeighbor,
		ApproxBiLinear,
		CatmullRom,
	}
	deltas := []image.Point{
		{+0, +0},
		{+0, +5},
		{+0, -5},
		{+5, +0},
		{-5, +0},
		{+8, +8},
		{+8, -8},
		{-8, +8},
		{-8, -8},
	}

	for _, q := range qs {
		want := image.NewRGBA(image.Rect(0, 0, 200, 200))
		q.Scale(want, want.Bounds(), src, src.Bounds(), nil)
		for _, delta := range deltas {
			tsrc := &translatedImage{src, delta}

			got := image.NewRGBA(image.Rect(0, 0, 200, 200))
			q.Scale(got, got.Bounds(), tsrc, tsrc.Bounds(), nil)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Errorf("pix differ for delta=%v, q=%T", delta, q)
			}

			// TODO: Transform, once Kernel.Transform is implemented.
		}
	}
}

// The fooWrapper types wrap the dst or src image to avoid triggering the
// type-specific fast path implementations.
type (
	dstWrapper struct{ Image }
	srcWrapper struct{ image.Image }
)

// TestFastPaths tests that the fast path implementations produce identical
// results to the generic implementation.
func TestFastPaths(t *testing.T) {
	drs := []image.Rectangle{
		image.Rect(0, 0, 10, 10),   // The dst bounds.
		image.Rect(3, 4, 8, 6),     // A strict subset of the dst bounds.
		image.Rect(-3, -5, 2, 4),   // Partial out-of-bounds #0.
		image.Rect(4, -2, 6, 12),   // Partial out-of-bounds #1.
		image.Rect(12, 14, 23, 45), // Complete out-of-bounds.
		image.Rect(5, 5, 5, 5),     // Empty.
	}
	srs := []image.Rectangle{
		image.Rect(0, 0, 12, 9),    // The src bounds.
		image.Rect(2, 2, 10, 8),    // A strict subset of the src bounds.
		image.Rect(10, 5, 20, 20),  // Partial out-of-bounds #0.
		image.Rect(-40, 0, 40, 8),  // Partial out-of-bounds #1.
		image.Rect(-8, -8, -4, -4), // Complete out-of-bounds.
		image.Rect(5, 5, 5, 5),     // Empty.
	}
	srcfs := []func(image.Rectangle) (image.Image, error){
		srcGray,
		srcNRGBA,
		srcRGBA,
		srcUniform,
		srcYCbCr,
	}
	var srcs []image.Image
	for _, srcf := range srcfs {
		src, err := srcf(srs[0])
		if err != nil {
			t.Fatal(err)
		}
		srcs = append(srcs, src)
	}
	qs := []Interpolator{
		NearestNeighbor,
		ApproxBiLinear,
		CatmullRom,
	}
	blue := image.NewUniform(color.RGBA{0x11, 0x22, 0x44, 0x7f})

	for _, dr := range drs {
		for _, src := range srcs {
			for _, sr := range srs {
				for _, q := range qs {
					dst0 := image.NewRGBA(drs[0])
					dst1 := image.NewRGBA(drs[0])
					Draw(dst0, dst0.Bounds(), blue, image.Point{}, Src)
					Draw(dstWrapper{dst1}, dst1.Bounds(), srcWrapper{blue}, image.Point{}, Src)
					q.Scale(dst0, dr, src, sr, nil)
					q.Scale(dstWrapper{dst1}, dr, srcWrapper{src}, sr, nil)
					if !bytes.Equal(dst0.Pix, dst1.Pix) {
						t.Errorf("pix differ for dr=%v, src=%T, sr=%v, q=%T", dr, src, sr, q)
					}

					// TODO: Transform, once Kernel.Transform is implemented.
				}
			}
		}
	}
}

func srcGray(boundsHint image.Rectangle) (image.Image, error) {
	m := image.NewGray(boundsHint)
	fillPix(rand.New(rand.NewSource(0)), m.Pix)
	return m, nil
}

func srcNRGBA(boundsHint image.Rectangle) (image.Image, error) {
	m := image.NewNRGBA(boundsHint)
	fillPix(rand.New(rand.NewSource(1)), m.Pix)
	return m, nil
}

func srcRGBA(boundsHint image.Rectangle) (image.Image, error) {
	m := image.NewRGBA(boundsHint)
	fillPix(rand.New(rand.NewSource(2)), m.Pix)
	// RGBA is alpha-premultiplied, so the R, G and B values should
	// be <= the A values.
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i+0] = uint8(uint32(m.Pix[i+0]) * uint32(m.Pix[i+3]) / 0xff)
		m.Pix[i+1] = uint8(uint32(m.Pix[i+1]) * uint32(m.Pix[i+3]) / 0xff)
		m.Pix[i+2] = uint8(uint32(m.Pix[i+2]) * uint32(m.Pix[i+3]) / 0xff)
	}
	return m, nil
}

func srcUniform(boundsHint image.Rectangle) (image.Image, error) {
	return image.NewUniform(color.RGBA64{0x1234, 0x5555, 0x9181, 0xbeef}), nil
}

func srcYCbCr(boundsHint image.Rectangle) (image.Image, error) {
	m := image.NewYCbCr(boundsHint, image.YCbCrSubsampleRatio420)
	fillPix(rand.New(rand.NewSource(3)), m.Y, m.Cb, m.Cr)
	return m, nil
}

func srcYCbCrLarge(boundsHint image.Rectangle) (image.Image, error) {
	// 3072 x 2304 is over 7 million pixels at 4:3, comparable to a
	// 2015 smart-phone camera's output.
	return srcYCbCr(image.Rect(0, 0, 3072, 2304))
}

func srcTux(boundsHint image.Rectangle) (image.Image, error) {
	// tux.png is a 386 x 395 image.
	f, err := os.Open("../testdata/tux.png")
	if err != nil {
		return nil, fmt.Errorf("Open: %v", err)
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("Decode: %v", err)
	}
	return src, nil
}

func benchScale(b *testing.B, srcf func(image.Rectangle) (image.Image, error), w int, h int, q Interpolator) {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	src, err := srcf(image.Rect(0, 0, 1024, 768))
	if err != nil {
		b.Fatal(err)
	}
	dr, sr := dst.Bounds(), src.Bounds()
	scaler := Scaler(q)
	if n, ok := q.(interface {
		NewScaler(int, int, int, int) Scaler
	}); ok {
		scaler = n.NewScaler(dr.Dx(), dr.Dy(), sr.Dx(), sr.Dy())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scaler.Scale(dst, dr, src, sr, nil)
	}
}

func benchTform(b *testing.B, srcf func(image.Rectangle) (image.Image, error), w int, h int, q Interpolator) {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	src, err := srcf(image.Rect(0, 0, 1024, 768))
	if err != nil {
		b.Fatal(err)
	}
	sr := src.Bounds()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Transform(dst, transformMatrix, src, sr, nil)
	}
}

func BenchmarkScaleLargeDownNN(b *testing.B) { benchScale(b, srcYCbCrLarge, 200, 150, NearestNeighbor) }
func BenchmarkScaleLargeDownAB(b *testing.B) { benchScale(b, srcYCbCrLarge, 200, 150, ApproxBiLinear) }
func BenchmarkScaleLargeDownBL(b *testing.B) { benchScale(b, srcYCbCrLarge, 200, 150, BiLinear) }
func BenchmarkScaleLargeDownCR(b *testing.B) { benchScale(b, srcYCbCrLarge, 200, 150, CatmullRom) }

func BenchmarkScaleDownNN(b *testing.B) { benchScale(b, srcTux, 120, 80, NearestNeighbor) }
func BenchmarkScaleDownAB(b *testing.B) { benchScale(b, srcTux, 120, 80, ApproxBiLinear) }
func BenchmarkScaleDownBL(b *testing.B) { benchScale(b, srcTux, 120, 80, BiLinear) }
func BenchmarkScaleDownCR(b *testing.B) { benchScale(b, srcTux, 120, 80, CatmullRom) }

func BenchmarkScaleUpNN(b *testing.B) { benchScale(b, srcTux, 800, 600, NearestNeighbor) }
func BenchmarkScaleUpAB(b *testing.B) { benchScale(b, srcTux, 800, 600, ApproxBiLinear) }
func BenchmarkScaleUpBL(b *testing.B) { benchScale(b, srcTux, 800, 600, BiLinear) }
func BenchmarkScaleUpCR(b *testing.B) { benchScale(b, srcTux, 800, 600, CatmullRom) }

func BenchmarkScaleSrcGray(b *testing.B)    { benchScale(b, srcGray, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcNRGBA(b *testing.B)   { benchScale(b, srcNRGBA, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcRGBA(b *testing.B)    { benchScale(b, srcRGBA, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcUniform(b *testing.B) { benchScale(b, srcUniform, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcYCbCr(b *testing.B)   { benchScale(b, srcYCbCr, 200, 150, ApproxBiLinear) }

func BenchmarkTformSrcGray(b *testing.B)    { benchTform(b, srcGray, 200, 150, ApproxBiLinear) }
func BenchmarkTformSrcNRGBA(b *testing.B)   { benchTform(b, srcNRGBA, 200, 150, ApproxBiLinear) }
func BenchmarkTformSrcRGBA(b *testing.B)    { benchTform(b, srcRGBA, 200, 150, ApproxBiLinear) }
func BenchmarkTformSrcUniform(b *testing.B) { benchTform(b, srcUniform, 200, 150, ApproxBiLinear) }
func BenchmarkTformSrcYCbCr(b *testing.B)   { benchTform(b, srcYCbCr, 200, 150, ApproxBiLinear) }
