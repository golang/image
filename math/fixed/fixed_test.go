// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fixed

import (
	"testing"
)

func TestInt26_6(t *testing.T) {
	x := Int26_6(1<<6 + 1<<4)
	if got, want := x.String(), "1:16"; got != want {
		t.Errorf("String: got %q, want %q", got, want)
	}
	if got, want := x.Floor(), Int26_6(1<<6); got != want {
		t.Errorf("Floor: got %v, want %v", got, want)
	}
	if got, want := x.Round(), Int26_6(1<<6); got != want {
		t.Errorf("Round: got %v, want %v", got, want)
	}
	if got, want := x.Ceil(), Int26_6(2<<6); got != want {
		t.Errorf("Ceil: got %v, want %v", got, want)
	}
}

func TestInt52_12(t *testing.T) {
	x := Int52_12(1<<12 + 1<<10)
	if got, want := x.String(), "1:1024"; got != want {
		t.Errorf("String: got %q, want %q", got, want)
	}
	if got, want := x.Floor(), Int52_12(1<<12); got != want {
		t.Errorf("Floor: got %v, want %v", got, want)
	}
	if got, want := x.Round(), Int52_12(1<<12); got != want {
		t.Errorf("Round: got %v, want %v", got, want)
	}
	if got, want := x.Ceil(), Int52_12(2<<12); got != want {
		t.Errorf("Ceil: got %v, want %v", got, want)
	}
}
