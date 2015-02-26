// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package f32 implements float32 vector and matrix types.
package f32 // import "golang.org/x/image/math/f32"

// Vec2 is a 2-element vector.
type Vec2 [2]float32

// Vec3 is a 3-element vector.
type Vec3 [3]float32

// Vec4 is a 4-element vector.
type Vec4 [4]float32

// Mat3 is a 3x3 matrix in row major order.
//
// m[3*r + c] is the element in the r'th row and c'th column.
type Mat3 [9]float32

// Mat4 is a 4x4 matrix in row major order.
//
// m[4*r + c] is the element in the r'th row and c'th column.
type Mat4 [16]float32
