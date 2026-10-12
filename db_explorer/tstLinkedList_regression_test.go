package main

import (
	"reflect"
	"testing"
)

func TestAddTwoNumbersLargeValues(t *testing.T) {
	large := make([]int, 31)
	large[0], large[30] = 1, 1
	wantLarge := append([]int(nil), large...)
	wantLarge[0], wantLarge[1], wantLarge[2] = 6, 6, 4
	for _, tt := range []struct {
		name                string
		first, second, want []int
	}{
		{"ordinary", []int{2, 4, 3}, []int{5, 6, 4}, []int{7, 0, 8}},
		{"carry", []int{9, 9, 9}, []int{1}, []int{0, 0, 0, 1}},
		{"zero", []int{0}, []int{0}, []int{0}},
		{"large", large, []int{5, 6, 4}, wantLarge},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := addTwoNumbers(reverseListFinal(tt.first), reverseListFinal(tt.second))
			var got []int
			for node := result; node != nil; node = node.Next {
				got = append(got, node.Val)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
