// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package webp

import (
	"bytes"
	"encoding/binary"
	encodinghex "encoding/hex"
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"strconv"
	"testing"
	"testing/iotest"
	"time"
)

func animationTestChunk(id string, payload []byte) []byte {
	b := make([]byte, 8, 8+len(payload)+len(payload)%2)
	copy(b, id)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(payload)))
	b = append(b, payload...)
	if len(payload)%2 != 0 {
		b = append(b, 0)
	}
	return b
}

func animationTestRIFF(chunks ...[]byte) []byte {
	payload := []byte("WEBP")
	for _, c := range chunks {
		payload = append(payload, c...)
	}
	return animationTestChunk("RIFF", payload)
}

func animationTestPut24(b []byte, n int) {
	b[0], b[1], b[2] = byte(n), byte(n>>8), byte(n>>16)
}

func animationTestVP8X(w, h int, flags byte) []byte {
	b := make([]byte, 10)
	b[0] = flags
	animationTestPut24(b[4:], w-1)
	animationTestPut24(b[7:], h-1)
	return animationTestChunk("VP8X", b)
}

func animationTestANIM(bg color.NRGBA, loop int) []byte {
	return animationTestChunk("ANIM", []byte{bg.B, bg.G, bg.R, bg.A, byte(loop), byte(loop >> 8)})
}

func animationTestANMF(r image.Rectangle, ms int, flags byte, payload []byte) []byte {
	b := make([]byte, 16)
	animationTestPut24(b[0:], r.Min.X/2)
	animationTestPut24(b[3:], r.Min.Y/2)
	animationTestPut24(b[6:], r.Dx()-1)
	animationTestPut24(b[9:], r.Dy()-1)
	animationTestPut24(b[12:], ms)
	b[15] = flags
	return animationTestChunk("ANMF", append(b, payload...))
}

// These 2-by-2 lossless images were generated independently using Pillow 13.0.0
// and libwebp 1.6.0:
//
//	Image.new("RGBA", (2, 2), rgba).save(out, format="WEBP", lossless=True, exact=True)
//
// Return the complete VP8L chunk, including its header and padding.
func animationTestLossless(t testing.TB, name string) []byte {
	t.Helper()
	var s string
	switch name {
	case "red": // (255, 0, 0, 128)
		s = "524946461c000000574542505650384c0f0000002f014000100710fd8f020622a2ff0100"
	case "green": // (0, 255, 0, 255)
		s = "524946461c000000574542505650384c0f0000002f0140000007d0ff88fe0722a2ff0100"
	case "blue": // (0, 0, 255, 128)
		s = "524946461c000000574542505650384c0f0000002f014000100710d1ff020622a2ff0100"
	case "transparent": // (0, 0, 0, 0)
		s = "524946461a000000574542505650384c0d0000002f01400010071011118888fe0700"
	default:
		t.Fatalf("unknown lossless image %q", name)
	}
	b, err := encodinghex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b[12:]
}

// This 2-by-2 lossy image was generated using Pillow 13.0.0 and libwebp 1.6.0:
//
//	Image.new("RGB", (2, 2), (128, 64, 32)).save(out, format="WEBP", quality=75)
func animationTestLossy(t testing.TB) []byte {
	t.Helper()
	b, err := encodinghex.DecodeString("524946463600000057454250565038202a000000b001009d012a0200020002003425a00274ba0004668000fef09b43f8b5adc2d2c3fd5788333d64000000")
	if err != nil {
		t.Fatal(err)
	}
	return b[12:]
}

func animationTestPixels(t testing.TB, got, want image.Image) {
	t.Helper()
	if got.Bounds().Size() != want.Bounds().Size() {
		t.Fatalf("image size = %v, want %v", got.Bounds().Size(), want.Bounds().Size())
	}
	for y := 0; y < want.Bounds().Dy(); y++ {
		for x := 0; x < want.Bounds().Dx(); x++ {
			g := color.NRGBAModel.Convert(got.At(got.Bounds().Min.X+x, got.Bounds().Min.Y+y)).(color.NRGBA)
			w := color.NRGBAModel.Convert(want.At(want.Bounds().Min.X+x, want.Bounds().Min.Y+y)).(color.NRGBA)
			if g != w {
				t.Fatalf("pixel (%d, %d) = %v, want %v", x, y, g, w)
			}
		}
	}
}

func TestDecodeAllAnimation(t *testing.T) {
	bg := color.NRGBA{R: 11, G: 22, B: 33, A: 64}
	tests := []struct {
		name  string
		r     image.Rectangle
		ms    int
		flags byte
		color color.NRGBA
	}{
		{"red", image.Rect(2, 0, 4, 2), 0, 3, color.NRGBA{R: 255, A: 128}},
		{"green", image.Rect(0, 2, 2, 4), 0xffffff, 0, color.NRGBA{G: 255, A: 255}},
		{"blue", image.Rect(4, 0, 6, 2), 37, 2, color.NRGBA{B: 255, A: 128}},
		{"transparent", image.Rect(2, 2, 4, 4), 1, 1, color.NRGBA{}},
	}
	chunks := [][]byte{animationTestVP8X(6, 4, 0x12), animationTestANIM(bg, 7)}
	for _, tc := range tests {
		chunks = append(chunks, animationTestANMF(tc.r, tc.ms, tc.flags, animationTestLossless(t, tc.name)))
	}
	a, err := DecodeAll(bytes.NewReader(animationTestRIFF(chunks...)))
	if err != nil {
		t.Fatal(err)
	}
	wantConfig := image.Config{ColorModel: color.NRGBAModel, Width: 6, Height: 4}
	if !a.IsAnimated || a.Config != wantConfig || a.LoopCount != 7 || a.BackgroundColor != bg {
		t.Fatalf("animation metadata = %+v", a)
	}
	if len(a.Frames) != len(tests) {
		t.Fatalf("frame count = %d, want %d", len(a.Frames), len(tests))
	}
	for i, tc := range tests {
		f := a.Frames[i]
		if f.Image.Bounds() != tc.r || f.Duration != time.Duration(tc.ms)*time.Millisecond || f.Blend != (tc.flags&2 == 0) || f.DisposeToBackground != (tc.flags&1 != 0) {
			t.Errorf("frame %d metadata = %+v, bounds = %v", i, f, f.Image.Bounds())
		}
		for y := tc.r.Min.Y; y < tc.r.Max.Y; y++ {
			for x := tc.r.Min.X; x < tc.r.Max.X; x++ {
				if got := color.NRGBAModel.Convert(f.Image.At(x, y)); got != tc.color {
					t.Errorf("frame %d at (%d, %d) = %v, want %v", i, x, y, got, tc.color)
				}
			}
		}
	}
}

func TestDecodeAnimatedFirstFrame(t *testing.T) {
	for _, tc := range []struct {
		name  string
		image string
		bg    color.NRGBA
		flags byte
		want  color.NRGBA
	}{
		{"blend", "red", color.NRGBA{G: 255, A: 255}, 0, color.NRGBA{R: 128, G: 127, A: 255}},
		{"replace", "red", color.NRGBA{G: 255, A: 255}, 2, color.NRGBA{R: 255, A: 128}},
		{"transparent blend", "transparent", color.NRGBA{G: 255, A: 255}, 0, color.NRGBA{G: 255, A: 255}},
		{"transparent replace", "transparent", color.NRGBA{G: 255, A: 255}, 2, color.NRGBA{}},
		{"transparent background", "green", color.NRGBA{R: 25, G: 50, B: 75, A: 64}, 2, color.NRGBA{G: 255, A: 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := animationTestRIFF(animationTestVP8X(6, 6, 0x12), animationTestANIM(tc.bg, 0),
				animationTestANMF(image.Rect(2, 2, 4, 4), 10, tc.flags, animationTestLossless(t, tc.image)))
			m, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := m.(*image.NRGBA); !ok {
				t.Fatalf("image type = %T, want *image.NRGBA", m)
			}
			if m.Bounds() != image.Rect(0, 0, 6, 6) {
				t.Fatalf("bounds = %v, want (0,0)-(6,6)", m.Bounds())
			}
			for y := 0; y < 6; y++ {
				for x := 0; x < 6; x++ {
					want := tc.bg
					if image.Pt(x, y).In(image.Rect(2, 2, 4, 4)) {
						want = tc.want
					}
					if got := color.NRGBAModel.Convert(m.At(x, y)); got != want {
						t.Fatalf("pixel (%d, %d) = %v, want %v", x, y, got, want)
					}
				}
			}
		})
	}
}

// animationTestSource returns real frame chunks from the existing static test
// corpus, and the image those chunks represent.
func animationTestSource(t testing.TB, name string) ([]byte, image.Image) {
	t.Helper()
	data, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var payload []byte
	for b := data[12:]; len(b) != 0; {
		if len(b) < 8 {
			t.Fatalf("short chunk header in %s", name)
		}
		n := uint64(binary.LittleEndian.Uint32(b[4:8]))
		n += n & 1
		if n > uint64(len(b)-8) {
			t.Fatalf("short chunk in %s", name)
		}
		n += 8
		switch string(b[:4]) {
		case "ALPH", "VP8 ", "VP8L":
			payload = append(payload, b[:int(n)]...)
		}
		b = b[int(n):]
	}
	return payload, m
}

func TestDecodeAllAnimationCodecs(t *testing.T) {
	names := []string{"blue-purple-pink.lossy.webp", "yellow_rose.lossy-with-alpha.webp", "gopher-doc.with-alpha.lossless.webp"}
	var frames [][]byte
	var images []image.Image
	w, h := 0, 0
	for _, name := range names {
		payload, m := animationTestSource(t, name)
		r := m.Bounds().Add(image.Pt(2, 2))
		w, h = max(w, r.Max.X), max(h, r.Max.Y)
		frames = append(frames, animationTestANMF(r, 10, 2, payload))
		images = append(images, m)
	}
	chunks := append([][]byte{animationTestVP8X(w, h, 0x12), animationTestANIM(color.NRGBA{}, 0)}, frames...)
	a, err := DecodeAll(bytes.NewReader(animationTestRIFF(chunks...)))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Frames) != len(images) {
		t.Fatalf("frame count = %d, want %d", len(a.Frames), len(images))
	}
	for i, m := range images {
		animationTestPixels(t, a.Frames[i].Image, m)
		if got, want := a.Frames[i].Image.Bounds(), m.Bounds().Add(image.Pt(2, 2)); got != want {
			t.Errorf("frame %d bounds = %v, want %v", i, got, want)
		}
	}
}

func TestDecodeAllStatic(t *testing.T) {
	for _, name := range []string{"blue-purple-pink.lossy.webp", "yellow_rose.lossy-with-alpha.webp", "gopher-doc.with-alpha.lossless.webp"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			want, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			a, err := DecodeAll(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if a.IsAnimated || a.LoopCount != 1 || len(a.Frames) != 1 || a.Config.Width != want.Bounds().Dx() || a.Config.Height != want.Bounds().Dy() {
				t.Fatalf("static image metadata = %+v", a)
			}
			if a.Frames[0].Duration != 0 || a.Frames[0].DisposeToBackground {
				t.Errorf("static frame metadata = %+v", a.Frames[0])
			}
			animationTestPixels(t, a.Frames[0].Image, want)
		})
	}
}

func TestDecodeStaticIgnoresANIM(t *testing.T) {
	for _, flags := range []byte{0, 0x10} {
		data := animationTestRIFF(animationTestVP8X(2, 2, flags),
			// The animation flag is unset, so even an ANIM chunk that would
			// be invalid in an animation must be ignored.
			animationTestChunk("ANIM", []byte{1, 2, 3}), animationTestLossless(t, "green"))
		m, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Decode with flags %#x: %v", flags, err)
		}
		a, err := DecodeAll(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("DecodeAll with flags %#x: %v", flags, err)
		}
		if a.IsAnimated || a.LoopCount != 1 || len(a.Frames) != 1 || a.BackgroundColor != (color.NRGBA{}) {
			t.Fatalf("flags %#x: static image metadata = %+v", flags, a)
		}
		animationTestPixels(t, a.Frames[0].Image, m)
		if got := color.NRGBAModel.Convert(m.At(0, 0)); got != (color.NRGBA{G: 255, A: 255}) {
			t.Errorf("flags %#x: pixel = %v, want opaque green", flags, got)
		}
	}
}

func TestDecodeAnimationFrameIndependence(t *testing.T) {
	frame := animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "green"))
	data := animationTestRIFF(animationTestVP8X(2, 2, 2), animationTestANIM(color.NRGBA{}, 0), frame, frame)
	a, err := DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Frames) != 2 {
		t.Fatalf("frame count = %d, want 2", len(a.Frames))
	}
	first, ok := a.Frames[0].Image.(interface{ Set(int, int, color.Color) })
	if !ok {
		t.Fatalf("frame type %T does not have a Set method", a.Frames[0].Image)
	}
	first.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if got := color.NRGBAModel.Convert(a.Frames[1].Image.At(0, 0)); got != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("modifying the first frame changed the second frame to %v", got)
	}
}

func TestDecodeAnimationRegistered(t *testing.T) {
	data := animationTestRIFF(animationTestVP8X(4, 2, 2), animationTestANIM(color.NRGBA{}, 1),
		animationTestANMF(image.Rect(2, 0, 4, 2), 10, 2, animationTestLossless(t, "green")))
	m, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if format != "webp" || m.Bounds() != image.Rect(0, 0, 4, 2) {
		t.Fatalf("image.Decode format = %q, bounds = %v", format, m.Bounds())
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "webp" || cfg.Width != 4 || cfg.Height != 2 {
		t.Fatalf("image.DecodeConfig = %+v, %q, %v", cfg, format, err)
	}
}

func TestDecodeAnimationUnknownChunksAndReservedBits(t *testing.T) {
	vp8x := animationTestVP8X(2, 2, 0xd3)
	vp8x[9], vp8x[10], vp8x[11] = 0x12, 0x34, 0x56
	unknown := animationTestChunk("test", []byte{1, 2, 3})
	frame := animationTestANMF(image.Rect(0, 0, 2, 2), 5, 0xfe, append(animationTestLossless(t, "green"), unknown...))
	data := animationTestRIFF(vp8x, unknown, animationTestANIM(color.NRGBA{A: 255}, 65535), unknown, frame, unknown)
	a, err := DecodeAll(iotest.OneByteReader(bytes.NewReader(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Frames) != 1 || a.LoopCount != 65535 || a.Frames[0].Blend || a.Frames[0].DisposeToBackground {
		t.Fatalf("animation = %+v", a)
	}
	if got := color.NRGBAModel.Convert(a.Frames[0].Image.At(0, 0)); got != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("pixel = %v, want opaque green", got)
	}
}

func TestDecodeAnimationWithoutGlobalAlpha(t *testing.T) {
	bg := color.NRGBA{R: 10, G: 20, B: 30, A: 40}
	data := animationTestRIFF(animationTestVP8X(4, 2, 2), animationTestANIM(bg, 1),
		animationTestANMF(image.Rect(2, 0, 4, 2), 10, 2, animationTestLossless(t, "green")))
	m, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(m.At(0, 0)); got != bg {
		t.Fatalf("background = %v, want %v", got, bg)
	}
}

func TestDecodeAnimationFirstFrameOnly(t *testing.T) {
	data := animationTestRIFF(animationTestVP8X(2, 2, 0x12), animationTestANIM(color.NRGBA{}, 0),
		animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "red")),
		animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestChunk("VP8L", []byte{0})))
	if _, err := Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if _, err := DecodeAll(bytes.NewReader(data)); err == nil {
		t.Fatal("DecodeAll succeeded with a corrupt second frame")
	}
	config, err := DecodeConfig(bytes.NewReader(data[:30]))
	if err != nil || config.Width != 2 || config.Height != 2 {
		t.Fatalf("DecodeConfig = %+v, %v; want 2 by 2 and no error", config, err)
	}
}

func TestDecodeAnimationConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags byte
		model color.Model
	}{
		{"animated opaque", 0x02, color.NRGBAModel},
		{"animated alpha", 0x12, color.NRGBAModel},
		{"static opaque", 0x00, color.YCbCrModel},
		{"static alpha", 0x10, color.NYCbCrAModel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := animationTestRIFF(animationTestVP8X(4, 6, tc.flags), animationTestANIM(color.NRGBA{}, 0),
				animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "green")))
			// Only the RIFF and VP8X headers are available. DecodeConfig must
			// not read or allocate frame pixels, or require animation data.
			cfg, err := DecodeConfig(bytes.NewReader(data[:30]))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Width != 4 || cfg.Height != 6 || cfg.ColorModel != tc.model {
				t.Fatalf("config = %+v, want 4 by 6 with color model %v", cfg, tc.model)
			}
		})
	}
}

func TestDecodeAnimationLargeCanvas(t *testing.T) {
	// The canvas dimensions fit the existing VP8X pixel-count limit, but a
	// four-byte-per-pixel canvas would overflow int on a 32-bit system.
	// DecodeAll only needs to allocate the small, independent frame.
	data := animationTestRIFF(animationTestVP8X(65536, 16384, 2), animationTestANIM(color.NRGBA{}, 1),
		animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "green")))
	a, err := DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if a.Config.Width != 65536 || a.Config.Height != 16384 || len(a.Frames) != 1 {
		t.Fatalf("animation metadata = %+v", a)
	}
	if got := a.Frames[0].Image.Bounds(); got != image.Rect(0, 0, 2, 2) {
		t.Fatalf("frame bounds = %v, want (0,0)-(2,2)", got)
	}
	if got := color.NRGBAModel.Convert(a.Frames[0].Image.At(0, 0)); got != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("frame pixel = %v, want opaque green", got)
	}
	if strconv.IntSize == 32 {
		if _, err := Decode(bytes.NewReader(data)); err == nil {
			t.Fatal("Decode succeeded with a canvas that overflows int")
		}
	}
}

func TestDecodeAnimationAlphaFilters(t *testing.T) {
	want := []byte{10, 30, 60, 100}
	for _, tc := range []struct {
		name    string
		filter  byte
		encoded []byte
	}{
		{"none", 0, []byte{10, 30, 60, 100}},
		{"horizontal", 1, []byte{10, 20, 50, 40}},
		{"vertical", 2, []byte{10, 20, 50, 70}},
		{"gradient", 3, []byte{10, 20, 50, 20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alpha := animationTestChunk("ALPH", append([]byte{tc.filter << 2}, tc.encoded...))
			payload := append(alpha, animationTestLossy(t)...)
			data := animationTestRIFF(animationTestVP8X(2, 2, 0x12), animationTestANIM(color.NRGBA{}, 1),
				animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, payload))
			a, err := DecodeAll(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if len(a.Frames) != 1 {
				t.Fatalf("frame count = %d, want 1", len(a.Frames))
			}
			m, ok := a.Frames[0].Image.(*image.NYCbCrA)
			if !ok {
				t.Fatalf("frame type = %T, want *image.NYCbCrA", a.Frames[0].Image)
			}
			if m.AStride != 2 || !bytes.Equal(m.A, want) {
				t.Fatalf("alpha = %v, stride = %d; want %v, stride 2", m.A, m.AStride, want)
			}
		})
	}
}

func TestDecodeAllAnimationInvalidAlpha(t *testing.T) {
	alpha := animationTestChunk("ALPH", []byte{0, 10, 30, 60, 100})
	bitstream := animationTestLossy(t)
	for _, tc := range []struct {
		name   string
		width  int
		chunks [][]byte
	}{
		{"duplicate ALPH", 2, [][]byte{alpha, alpha, bitstream}},
		{"ALPH after image", 2, [][]byte{bitstream, alpha}},
		{"invalid compression", 2, [][]byte{animationTestChunk("ALPH", []byte{2, 10, 30, 60, 100}), bitstream}},
		{"short alpha", 2, [][]byte{animationTestChunk("ALPH", []byte{0, 10, 30, 60}), bitstream}},
		{"empty alpha", 2, [][]byte{animationTestChunk("ALPH", nil), bitstream}},
		{"VP8 dimension mismatch", 1, [][]byte{bitstream}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var payload []byte
			for _, c := range tc.chunks {
				payload = append(payload, c...)
			}
			data := animationTestRIFF(animationTestVP8X(2, 2, 0x12), animationTestANIM(color.NRGBA{}, 1),
				animationTestANMF(image.Rect(0, 0, tc.width, 2), 10, 2, payload))
			if _, err := DecodeAll(bytes.NewReader(data)); err == nil {
				t.Fatal("DecodeAll succeeded, want an error")
			}
		})
	}
}

func TestDecodeAllAnimationInvalid(t *testing.T) {
	vp8x := animationTestVP8X(2, 2, 0x12)
	anim := animationTestANIM(color.NRGBA{}, 0)
	imageData := animationTestLossless(t, "red")
	frame := animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, imageData)
	alpha := animationTestChunk("ALPH", []byte{0, 128, 128, 128, 128})
	for _, tc := range []struct {
		name   string
		chunks [][]byte
	}{
		{"no frames", [][]byte{vp8x, anim}},
		{"missing ANIM", [][]byte{vp8x, frame}},
		{"late ANIM", [][]byte{vp8x, frame, anim}},
		{"duplicate ANIM", [][]byte{vp8x, anim, anim, frame}},
		{"short ANIM", [][]byte{vp8x, animationTestChunk("ANIM", make([]byte, 5)), frame}},
		{"duplicate VP8X", [][]byte{vp8x, anim, vp8x, frame}},
		{"short frame header", [][]byte{vp8x, anim, animationTestChunk("ANMF", make([]byte, 15))}},
		{"empty frame", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, nil)}},
		{"frame outside canvas", [][]byte{vp8x, anim, animationTestANMF(image.Rect(2, 0, 4, 2), 10, 0, imageData)}},
		{"frame wider than canvas", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 3, 2), 10, 0, imageData)}},
		{"frame size mismatch", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 1, 2), 10, 0, imageData)}},
		{"huge frame", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 1<<24, 1<<24), 10, 0, imageData)}},
		{"huge offset", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0x1fffffe, 0x1fffffe, 0x2000000, 0x2000000), 10, 0, imageData)}},
		{"missing frame bitstream", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, animationTestChunk("test", nil))}},
		{"two frame bitstreams", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, append(append([]byte{}, imageData...), imageData...))}},
		{"ALPH with VP8L", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, append(alpha, imageData...))}},
		{"ALPH without bitstream", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, alpha)}},
		{"top level bitstream", [][]byte{vp8x, anim, imageData, frame}},
		{"truncated bitstream", [][]byte{vp8x, anim, animationTestANMF(image.Rect(0, 0, 2, 2), 10, 0, animationTestChunk("VP8L", []byte{0x2f, 1, 0x40, 0, 0x10}))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeAll(bytes.NewReader(animationTestRIFF(tc.chunks...))); err == nil {
				t.Fatal("DecodeAll succeeded, want an error")
			}
		})
	}
}

func TestDecodeAllAnimationTruncated(t *testing.T) {
	data := animationTestRIFF(animationTestVP8X(2, 2, 0x12), animationTestANIM(color.NRGBA{}, 0),
		animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "red")),
		animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "green")))
	for n := 0; n < len(data); n++ {
		if _, err := DecodeAll(bytes.NewReader(data[:n])); err == nil {
			t.Errorf("DecodeAll succeeded with %d of %d bytes", n, len(data))
		}
	}
}

func TestDecodeAllAnimationReadError(t *testing.T) {
	data := animationTestRIFF(animationTestVP8X(2, 2, 0x12), animationTestANIM(color.NRGBA{}, 0),
		animationTestANMF(image.Rect(0, 0, 2, 2), 10, 2, animationTestLossless(t, "red")))
	want := errors.New("test read error")
	for _, n := range []int{0, 17, 35, len(data) - 1} {
		r := io.MultiReader(bytes.NewReader(data[:n]), iotest.ErrReader(want))
		if _, err := DecodeAll(r); !errors.Is(err, want) {
			t.Errorf("read error at byte %d = %v, want %v", n, err, want)
		}
	}
}
