package sfnt

import (
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/math/fixed"
)

func FuzzParse(f *testing.F) {
	// Seed: a real font.
	f.Add(goregular.TTF)

	// Seed: truncated font.
	f.Add(goregular.TTF[:256])

	// Seed: empty.
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		fnt, err := Parse(data)
		if err != nil {
			return
		}
		var buf Buffer

		// Exercise glyph count.
		n := fnt.NumGlyphs()
		if n == 0 {
			return
		}

		// Exercise GlyphIndex (cmap parsing).
		for _, r := range "AaBb0123" {
			idx, err := fnt.GlyphIndex(&buf, r)
			if err != nil || idx == 0 {
				continue
			}
			// Exercise glyph metrics.
			fnt.GlyphAdvance(&buf, idx, fixed.I(12), font.HintingNone)
			fnt.GlyphBounds(&buf, idx, fixed.I(12), font.HintingNone)
		}

		// Exercise kerning (GPOS parsing path where OOB bugs were found).
		idxA, _ := fnt.GlyphIndex(&buf, 'A')
		idxV, _ := fnt.GlyphIndex(&buf, 'V')
		if idxA != 0 && idxV != 0 {
			fnt.Kern(&buf, idxA, idxV, fixed.I(12), font.HintingNone)
		}
	})
}
