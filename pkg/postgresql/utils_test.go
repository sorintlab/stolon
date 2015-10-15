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

package postgresql

import (
	"reflect"
	"testing"

	"github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/davecgh/go-spew/spew"
	"github.com/sorintlab/stolon/pkg/cluster"
)

func TestParseTimeLineHistory(t *testing.T) {
	tests := []struct {
		contents string
		tlsh     cluster.PostgresTimeLinesHistory
		err      error
	}{
		{
			contents: "",
			tlsh:     cluster.PostgresTimeLinesHistory{},
			err:      nil,
		},
		{
			contents: `1	0/5000090	no recovery target specified`,
			tlsh: cluster.PostgresTimeLinesHistory{
				{
					TimelineID:  1,
					SwitchPoint: 83886224,
					Reason:      "no recovery target specified",
				},
			},
			err: nil,
		},
	}

	for i, tt := range tests {
		tlsh, err := parseTimeLinesHistory(tt.contents)
		t.Logf("test #%d", i)
		if tt.err != nil {
			if err == nil {
				t.Errorf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(tlsh, tt.tlsh) {
				t.Errorf(spew.Sprintf("#%d: wrong timeline history: got: %#+v, want: %#+v", i, tlsh, tt.tlsh))
			}
		}
	}

}
