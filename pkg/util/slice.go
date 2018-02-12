// Copyright 2015 Sorint.lab
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
