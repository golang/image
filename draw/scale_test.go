// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package draw

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"reflect"
	"testing"

	_ "image/jpeg"
)

var genScaleFiles = flag.Bool("gen_scale_files", false, "whether to generate the TestScaleXxx golden files.")

// testScale tests that scaling the source image gives the exact destination
// image. This is to ensure that any refactoring or optimization of the scaling
// code doesn't change the scaling behavior. Changing the actual algorithm or
// kernel used by any particular quality setting will obviously change the
// resultant pixels. In such a case, use the gen_scale_files flag to regenerate
// the golden files.
func testScale(t *testing.T, w int, h int, direction, srcFilename string) {
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
		gotFilename := fmt.Sprintf("../testdata/go-turns-two-%s-%s.png", direction, name)

		got := image.NewRGBA(image.Rect(0, 0, w, h))
		Scale(got, got.Bounds(), src, src.Bounds(), q)
		if *genScaleFiles {
			g, err := os.Create(gotFilename)
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

		g, err := os.Open(gotFilename)
		if err != nil {
			t.Errorf("Open: %v", err)
			continue
		}
		defer g.Close()
		want, err := png.Decode(g)
		if err != nil {
			t.Errorf("Decode: %v", err)
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: actual image differs from golden image", gotFilename)
			continue
		}
	}
}

func TestScaleDown(t *testing.T) { testScale(t, 100, 100, "down", "280x360.jpeg") }
func TestScaleUp(t *testing.T)   { testScale(t, 75, 100, "up", "14x18.png") }

// TODO: test that scaling concrete types like *image.RGBA and *image.YCbCr
// give the same results as scaling those images wrapped in another Image or
// image.Image type that would skip the fast-path type switch.

func srcNRGBA() (image.Image, error) {
	return image.NewNRGBA(image.Rect(0, 0, 1024, 768)), nil
}

func srcRGBA() (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 1024, 768)), nil
}

func srcUniform() (image.Image, error) {
	return image.White, nil
}

func srcYCbCr() (image.Image, error) {
	return image.NewYCbCr(image.Rect(0, 0, 1024, 768), image.YCbCrSubsampleRatio420), nil
}

func srcYCbCrLarge() (image.Image, error) {
	// 3072 x 2304 is over 7 million pixels at 4:3, comparable to a
	// 2015 smart-phone camera's output.
	return image.NewYCbCr(image.Rect(0, 0, 3072, 2304), image.YCbCrSubsampleRatio420), nil
}

func srcTux() (image.Image, error) {
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

func benchScale(b *testing.B, srcf func() (image.Image, error), w int, h int, q Interpolator) {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	src, err := srcf()
	if err != nil {
		b.Fatal(err)
	}
	dr, sr := dst.Bounds(), src.Bounds()
	scaler := q.NewScaler(int32(dr.Dx()), int32(dr.Dy()), int32(sr.Dx()), int32(sr.Dy()))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scaler.Scale(dst, dr.Min, src, sr.Min)
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

func BenchmarkScaleSrcNRGBA(b *testing.B)   { benchScale(b, srcNRGBA, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcRGBA(b *testing.B)    { benchScale(b, srcRGBA, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcUniform(b *testing.B) { benchScale(b, srcUniform, 200, 150, ApproxBiLinear) }
func BenchmarkScaleSrcYCbCr(b *testing.B)   { benchScale(b, srcYCbCr, 200, 150, ApproxBiLinear) }
