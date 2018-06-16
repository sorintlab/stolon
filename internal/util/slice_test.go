// Copyright 2018 Sorint.lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import "testing"

func TestCompareStringSlice(t *testing.T) {
	tests := []struct {
		a  []string
		b  []string
		ok bool
	}{
		{[]string{}, []string{}, true},
		{[]string{"", ""}, []string{""}, false},
		{[]string{"", ""}, []string{"", ""}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, false},
		{[]string{"a", "b", "c"}, []string{"a", "b"}, false},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, true},
		{[]string{"a", "b", "c"}, []string{"b", "c", "a"}, false},
		{[]string{"a", "b", "c", "a"}, []string{"a", "c", "b", "b"}, false},
		{[]string{"a", "b", "c", "a"}, []string{"a", "c", "b", "b"}, false},
	}

	for i, tt := range tests {
		ok := CompareStringSlice(tt.a, tt.b)
		if ok != tt.ok {
			t.Errorf("%d: got %t but wanted: %t a: %v, b: %v", i, ok, tt.ok, tt.a, tt.b)
		}
	}
}

func TestCompareStringSliceNoOrder(t *testing.T) {
	tests := []struct {
		a  []string
		b  []string
		ok bool
	}{
		{[]string{}, []string{}, true},
		{[]string{"", ""}, []string{""}, false},
		{[]string{"", ""}, []string{"", ""}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a", "b", "c"}, []string{"a", "b"}, false},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, true},
		{[]string{"a", "b", "c"}, []string{"b", "c", "a"}, true},
		{[]string{"a", "b", "c", "a"}, []string{"a", "c", "b", "b"}, false},
		{[]string{"a", "b", "c", "a"}, []string{"a", "c", "b", "b"}, false},
	}

	for i, tt := range tests {
		ok := CompareStringSliceNoOrder(tt.a, tt.b)
		if ok != tt.ok {
			t.Errorf("%d: got %t but wanted: %t a: %v, b: %v", i, ok, tt.ok, tt.a, tt.b)
		}
	}
}

func TestDifference(t *testing.T) {
	tests := []struct {
		a []string
		b []string
		r []string
	}{
		{[]string{}, []string{}, []string{}},
		{[]string{"", ""}, []string{""}, []string{}},
		{[]string{"", ""}, []string{"", ""}, []string{}},
		{[]string{"", ""}, []string{"a", "", "b"}, []string{}},
		{[]string{"a", "b"}, []string{"a", "b"}, []string{}},
		{[]string{"a", "b"}, []string{"b", "a"}, []string{}},
		{[]string{"a", "b", "c"}, []string{}, []string{"a", "b", "c"}},
		{[]string{"a", "b", "c"}, []string{"a", "b"}, []string{"c"}},
		{[]string{"a", "b"}, []string{"a", "b", "c"}, []string{}},
		{[]string{"a", "b"}, []string{"c", "a", "b"}, []string{}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, []string{}},
		{[]string{"a", "b", "c"}, []string{"b", "c", "a"}, []string{}},
		{[]string{"a", "b", "c", "a"}, []string{"a", "c", "b", "b"}, []string{}},
		{[]string{"a", "b", "c", "a"}, []string{"a", "c", "b", "b"}, []string{}},
	}

	for i, tt := range tests {
		r := Difference(tt.a, tt.b)
		if !CompareStringSliceNoOrder(r, tt.r) {
			t.Errorf("%d: got %v but wanted: %v a: %v, b: %v", i, r, tt.r, tt.a, tt.b)
		}
	}
}
