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

package common_test

import (
	"slices"
	"testing"

	"github.com/sorintlab/stolon/internal/common"
	"github.com/sorintlab/stolon/internal/util"
)

func TestDiffReturnsChangedParams(t *testing.T) {
	var curParams common.Parameters = map[string]string{
		"max_connections": "100",
		"shared_buffers":  "10MB",
		"huge":            "off",
	}

	var newParams common.Parameters = map[string]string{
		"max_connections": "200",
		"shared_buffers":  "10MB",
		"work_mem":        "4MB",
	}

	expectedDiff := []string{"max_connections", "huge", "work_mem"}

	diff := curParams.Diff(newParams)

	if !util.CompareStringSliceNoOrder(expectedDiff, diff) {
		t.Errorf("Expected diff is %v, but got %v", expectedDiff, diff)
	}
}

func TestUID(t *testing.T) {
	uids := []string{common.UID()}
	for i := 0; i < 100; i++ {
		uid := common.UID()
		if slices.Contains(uids, uid) {
			t.Errorf("Expected every new uid to be unique, but %s is duplicate after %d rounds", uid, len(uids))
		}
	}
}

func TestUUID(t *testing.T) {
	uuids := []string{common.UUID()}
	for i := 0; i < 100; i++ {
		uuid := common.UUID()
		if slices.Contains(uuids, uuid) {
			t.Errorf("Expected every new uuid to be unique, but %s is duplicate after %d rounds", uuid, len(uuids))
		}
	}
}
