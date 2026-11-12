// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sfnt

import (
	"encoding/binary"
	"reflect"
	"testing"

	"golang.org/x/image/math/fixed"
)

type glyfTestPoint struct {
	x, y int16
	on   bool
}

// glyfTestData returns the glyf data of a simple glyph of the given contours.
func glyfTestData(contours ...[]glyfTestPoint) []byte {
	u16 := binary.BigEndian.AppendUint16
	b := u16(nil, uint16(len(contours)))
	b = append(b, make([]byte, 8)...) // xMin, yMin, xMax, yMax
	var points []glyfTestPoint
	for _, c := range contours {
		points = append(points, c...)
		b = u16(b, uint16(len(points)-1))
	}
	b = u16(b, 0) // instructionLength
	for _, p := range points {
		flag := byte(0)
		if p.on {
			flag = flagOnCurve
		}
		b = append(b, flag)
	}
	var x, y int16
	for _, p := range points {
		b = u16(b, uint16(p.x-x))
		x = p.x
	}
	for _, p := range points {
		b = u16(b, uint16(p.y-y))
		y = p.y
	}
	return b
}

func TestLoadGlyfSingleOffCurvePoint(t *testing.T) {
	square := []glyfTestPoint{
		{100, 0, true},
		{400, 0, true},
		{400, 300, true},
		{100, 300, true},
	}
	lone := []glyfTestPoint{{250, 600, false}}

	pt := func(x, y fixed.Int26_6) fixed.Point26_6 { return fixed.Point26_6{X: x, Y: y} }
	squareSegs := []Segment{
		{Op: SegmentOpMoveTo, Args: [3]fixed.Point26_6{pt(100, 0)}},
		{Op: SegmentOpLineTo, Args: [3]fixed.Point26_6{pt(400, 0)}},
		{Op: SegmentOpLineTo, Args: [3]fixed.Point26_6{pt(400, 300)}},
		{Op: SegmentOpLineTo, Args: [3]fixed.Point26_6{pt(100, 300)}},
		{Op: SegmentOpLineTo, Args: [3]fixed.Point26_6{pt(100, 0)}},
	}
	loneSegs := []Segment{
		{Op: SegmentOpMoveTo, Args: [3]fixed.Point26_6{pt(250, 600)}},
		{Op: SegmentOpQuadTo, Args: [3]fixed.Point26_6{pt(250, 600), pt(250, 600)}},
	}

	testCases := []struct {
		name     string
		contours [][]glyfTestPoint
		want     Segments
	}{
		{"lone", [][]glyfTestPoint{lone}, loneSegs},
		{"square then lone", [][]glyfTestPoint{square, lone}, append(squareSegs[:len(squareSegs):len(squareSegs)], loneSegs...)},
		{"lone then square", [][]glyfTestPoint{lone, square}, append(loneSegs[:len(loneSegs):len(loneSegs)], squareSegs...)},
	}
	for _, tc := range testCases {
		data := glyfTestData(tc.contours...)
		f := &Font{src: source{b: data}}
		f.cached.glyphData.locations = []uint32{0, uint32(len(data))}
		var b Buffer
		if err := loadGlyf(f, &b, 0, 0, 0); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !reflect.DeepEqual(b.segments, tc.want) {
			t.Errorf("%s:\ngot  %v\nwant %v", tc.name, b.segments, tc.want)
		}
	}
}
