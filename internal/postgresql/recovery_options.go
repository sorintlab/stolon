// Copyright 2026 PgVillage
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

	"github.com/pgvillage-tools/stolon/internal/common"

	"github.com/mitchellh/copystructure"
)

// RecoveryMode is an enum for the type of recover an instance is in
type RecoveryMode int

const (
	// RecoveryModeNone is set when an instance is a primary instance
	RecoveryModeNone RecoveryMode = iota
	// RecoveryModeStandby is set when an instance is a replica instance
	RecoveryModeStandby
	// RecoveryModeRecovery is set during Point in time recovery
	RecoveryModeRecovery
)

// RecoveryOptions set recovery options
type RecoveryOptions struct {
	RecoveryMode       RecoveryMode
	RecoveryParameters common.Parameters
}

// NewRecoveryOptions returns a freshly initialized Recoveryoptions resource
func NewRecoveryOptions() *RecoveryOptions {
	return &RecoveryOptions{RecoveryParameters: make(common.Parameters)}
}

// DeepCopy returns a full copy
func (r *RecoveryOptions) DeepCopy() (ro *RecoveryOptions) {
	var ok bool
	if nr, err := copystructure.Copy(r); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(r, nr) {
		panic("not equal")
	} else if ro, ok = nr.(*RecoveryOptions); !ok {
		panic("type is different after copy")
	}
	return ro
}
