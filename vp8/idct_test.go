// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vp8

import "testing"

// TestInverseDCT4 checks inverseDCT4 for coefficients large enough that the
// multiplications used to overflow an int32. Dequantized coefficients are stored
// as an int16 (see reconstruct.go), so a bitstream can hold any int16 value: an
// AC token value of 89 with a dequantization factor of 284 gives 25276, above
// the 25079 at which multiplying by 85627 wraps.
//
// The second pass multiplies the first pass's results, which range up to about
// 126000, so it overflows for smaller coefficients than the first pass does.
//
// The want values were computed with the same transform in int64 arithmetic.
func TestInverseDCT4(t *testing.T) {
	testCases := []struct {
		desc  string
		coeff map[int]int16
		want  [4][4]uint8
	}{{
		desc:  "no overflow",
		coeff: map[int]int16{0: 180, 1: -40, 2: 15, 4: 90, 5: 22, 8: -30, 12: 12},
		want: [4][4]uint8{
			{162, 159, 161, 166},
			{156, 155, 159, 165},
			{144, 145, 152, 160},
			{122, 125, 134, 144},
		},
	}, {
		desc:  "overflow in the first row's coeff[4]",
		coeff: map[int]int16{4: 25276},
		want: [4][4]uint8{
			{255, 255, 255, 255},
			{255, 255, 255, 255},
			{0, 0, 0, 0},
			{0, 0, 0, 0},
		},
	}, {
		desc:  "overflow in the first row's coeff[12]",
		coeff: map[int]int16{12: 25276},
		want: [4][4]uint8{
			{255, 255, 255, 255},
			{0, 0, 0, 0},
			{255, 255, 255, 255},
			{0, 0, 0, 0},
		},
	}, {
		desc:  "overflow with a negative coefficient",
		coeff: map[int]int16{4: -25276},
		want: [4][4]uint8{
			{0, 0, 0, 0},
			{0, 0, 0, 0},
			{255, 255, 255, 255},
			{255, 255, 255, 255},
		},
	}, {
		// Two maximal coefficients in the second row make the first pass
		// produce 75577, for which multiplying by 35468 in the second pass
		// overflows: that wraps once its input exceeds 60548.
		desc:  "overflow in the second pass",
		coeff: map[int]int16{1: 32767, 5: 32767},
		want: [4][4]uint8{
			{255, 255, 0, 0},
			{255, 255, 0, 0},
			{255, 255, 0, 0},
			{0, 0, 255, 255},
		},
	}}

	for _, tc := range testCases {
		d := NewDecoder()
		for i := range d.ybr {
			for j := range d.ybr[i] {
				d.ybr[i][j] = 128
			}
		}
		for i, c := range tc.coeff {
			d.coeff[i] = c
		}

		d.inverseDCT4(0, 0, 0)

		var got [4][4]uint8
		for j := range got {
			copy(got[j][:], d.ybr[j][:4])
		}
		if got != tc.want {
			t.Errorf("%s:\ngot  %v\nwant %v", tc.desc, got, tc.want)
		}
	}
}
