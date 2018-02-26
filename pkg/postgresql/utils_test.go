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

	"github.com/davecgh/go-spew/spew"
)

func TestParseTimelineHistory(t *testing.T) {
	tests := []struct {
		contents string
		tlsh     []*TimelineHistory
		err      error
	}{
		{
			contents: "",
			tlsh:     []*TimelineHistory{},
			err:      nil,
		},
		{
			contents: `1	0/5000090	no recovery target specified`,
			tlsh: []*TimelineHistory{
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
		tlsh, err := parseTimelinesHistory(tt.contents)
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

func TestValidReplSlotName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"aaaaaaaa", true},
		{"a12345aa", true},
		{"_a1_2345aa_", true},
		{"", false},
		{"a-aaaaaaa", false},
		{"_a1_-2345aa_", false},
		{"ABC123", false},
		{"$123", false},
	}

	for i, tt := range tests {
		valid := IsValidReplSlotName(tt.name)
		if valid != tt.valid {
			t.Errorf("%d: replication slot name %q got valid: %t but wanted valid: %t", i, tt.name, valid, tt.valid)
		}
	}
}

func TestExpand(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{
			in:  "",
			out: "",
		},
		{
			in:  "%a",
			out: "%a",
		},
		{
			in:  "%d",
			out: "/datadir",
		},
		{
			in:  "%%d",
			out: "%d",
		},
		{
			in:  "%%%d",
			out: "%/datadir",
		},
		{
			in:  "%%%danother/string/%%%%%%%f%dddddblabla",
			out: "%/datadiranother/string/%%%%f/datadirddddblabla",
		},
	}

	for i, tt := range tests {
		out := expand(tt.in, "/datadir")
		if out != tt.out {
			t.Errorf("#%d: wrong expanded string: got: %s, want: %s", i, out, tt.out)
		}
	}
}

func TestParseBinaryVersion(t *testing.T) {
	tests := []struct {
		in  string
		maj int
		min int
		err error
	}{
		{
			in:  "postgres (PostgreSQL) 9.5.7",
			maj: 9,
			min: 5,
		},
		{
			in:  "postgres (PostgreSQL) 9.6.7\n",
			maj: 9,
			min: 6,
		},
		{
			in:  "postgres (PostgreSQL) 10beta1",
			maj: 10,
			min: 0,
		},
		{
			in:  "postgres (PostgreSQL) 10.1.2",
			maj: 10,
			min: 1,
		},
	}

	for i, tt := range tests {
		maj, min, err := ParseBinaryVersion(tt.in)
		t.Logf("test #%d", i)
		if tt.err != nil {
			if err == nil {
				t.Fatalf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Fatalf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if maj != tt.maj || min != tt.min {
				t.Fatalf("#%d: wrong maj.min version: got: %d.%d, want: %d.%d", i, maj, min, tt.maj, tt.min)
			}
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in  string
		maj int
		min int
		err error
	}{
		{
			in:  "9.5.7",
			maj: 9,
			min: 5,
		},
	}

	for i, tt := range tests {
		maj, min, err := ParseVersion(tt.in)
		t.Logf("test #%d", i)
		if tt.err != nil {
			if err == nil {
				t.Fatalf("got no error, wanted error: %v", tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Fatalf("got error: %v, wanted error: %v", err, tt.err)
			}
		} else {
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if maj != tt.maj || min != tt.min {
				t.Fatalf("#%d: wrong maj.min versio : got: %d.%d, want: %d.%d", i, maj, min, tt.maj, tt.min)
			}
		}
	}
}

func TestIsWalFileName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"000000000000000000000000", true},
		{"ABCDEF0123456789ABCDEF00", true},
		{"", false},
		{"ABCDEF0123456789ABCDEF0", false},
		{"$123", false},
	}

	for i, tt := range tests {
		valid := IsWalFileName(tt.name)
		if valid != tt.valid {
			t.Errorf("%d: wal filename %q got valid: %t but wanted valid: %t", i, tt.name, valid, tt.valid)
		}
	}
}

func TestWalFileNameNoTimeLine(t *testing.T) {
	tests := []struct {
		name string
		out  string
	}{
		{"000000000000000000000000", "0000000000000000"},
		{"ABCDEF0123456789ABCDEF00", "23456789ABCDEF00"},
	}

	for i, tt := range tests {
		out, _ := WalFileNameNoTimeLine(tt.name)
		if out != tt.out {
			t.Errorf("%d: wal filename %q got: %s but wanted: %s", i, tt.name, out, tt.out)
		}
	}
}
