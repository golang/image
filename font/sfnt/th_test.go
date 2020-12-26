package sfnt

import "testing"

func Test_thrune(t *testing.T) {

	r := thrune('ู')

	if !r.isLower() {
		t.Errorf("ู is lower vowel")
	}

}
