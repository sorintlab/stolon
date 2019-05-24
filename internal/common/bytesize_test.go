// Copyright 2016 Sorint.lab
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

package common

import "testing"

func TestParseBytesize(t *testing.T) {
	tests := []struct {
		input       string
		expected    Bytesize
		shouldError bool
	}{
		{"10", 0, true},
		{"10k", 10 * Kilobyte, false},
		{"32k", 32 * Kilobyte, false},
		{"10M", 10 * Megabyte, false},
		{"fM", 0, true},
		{"", 0, true},
	}

	for i, tt := range tests {
		actual, err := ParseBytesize(tt.input)

		if tt.shouldError && (err == nil) {
			t.Errorf("#%d: expected error for: %s, actual no error", i, tt.input)
		}

		if !tt.shouldError && (err != nil) {
			t.Errorf("#%d: unexpected error for: %s, error: %v", i, tt.input, err)
		}

		if actual != tt.expected {
			t.Errorf("#%d: unexpected value. got: %d, expected: %d", i, actual, tt.expected)
		}
	}
}
