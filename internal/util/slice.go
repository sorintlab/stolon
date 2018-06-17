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

import "sort"

func StringInSlice(s []string, e string) bool {
	for _, v := range s {
		if v == e {
			return true
		}
	}
	return false
}

// CompareStringSlice compares two slices of strings, a nil slice is considered an empty one
func CompareStringSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// CompareStringSliceNoOrder compares two slices of strings regardless of their order, a nil slice is considered an empty one
func CompareStringSliceNoOrder(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// This isn't the faster way but it's cleaner and enough for us

	// Take a copy of the original slice
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)

	sort.Sort(sort.StringSlice(a))
	sort.Sort(sort.StringSlice(b))

	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// CommonElements return the common elements in two slices of strings
func CommonElements(a []string, b []string) []string {
	common := []string{}
	for _, v := range a {
		if StringInSlice(b, v) {
			common = append(common, v)
		}
	}
	return common
}

// Difference returns elements in a - b
func Difference(a []string, b []string) []string {
	diff := []string{}
	for _, v := range a {
		if !StringInSlice(b, v) {
			diff = append(diff, v)
		}
	}
	return diff
}
