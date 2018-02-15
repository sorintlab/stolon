// Copyright 2017 Sorint.lab
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

package cmd

import (
	"errors"
	"reflect"
	"testing"
)

var curUID int

func TestParseSynchronousStandbyNames(t *testing.T) {
	tests := []struct {
		in  string
		out []string
		err error
	}{
		{
			in:  "2 (stolon_2c3870f3,stolon_c874a3cb)",
			out: []string{"stolon_2c3870f3", "stolon_c874a3cb"},
		},
		{
			in:  "2 ( stolon_2c3870f3 , stolon_c874a3cb )",
			out: []string{"stolon_2c3870f3", "stolon_c874a3cb"},
		},
		{
			in:  "21 (\" stolon_2c3870f3\",stolon_c874a3cb)",
			out: []string{"\" stolon_2c3870f3\"", "stolon_c874a3cb"},
		},
		{
			in:  "stolon_2c3870f3,stolon_c874a3cb",
			out: []string{"stolon_2c3870f3", "stolon_c874a3cb"},
		},
		{
			in:  "node1",
			out: []string{"node1"},
		},
		{
			in:  "2 (node1,",
			out: []string{"node1"},
			err: errors.New("synchronous standby string has number but lacks brackets"),
		},
	}

	for i, tt := range tests {
		out, err := parseSynchronousStandbyNames(tt.in)

		if tt.err != nil {
			if err == nil {
				t.Errorf("%d: got no error, wanted error: %v", i, tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("%d: got error: %v, wanted error: %v", i, err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("%d: unexpected error: %v", i, err)
			} else if !reflect.DeepEqual(out, tt.out) {
				t.Errorf("%d: wrong output: got:\n%s\nwant:\n%s", i, out, tt.out)
			}
		}
	}
}
