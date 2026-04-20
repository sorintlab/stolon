// Copyright 2026 PgVillage
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

package util

import (
	"reflect"

	"github.com/mitchellh/copystructure"
)

// ToPtr returns the value as a pointer
func ToPtr[T any](v T) *T {
	return &v
}

// DeepCopy returns a copy of the KeeperInfo resource
func DeepCopy[T any](org *T) (dc *T) {
	var ok bool
	if org == nil {
		return nil
	}
	if nk, err := copystructure.Copy(org); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(org, nk) {
		panic("not equal")
	} else if dc, ok = nk.(*T); !ok {
		panic("different type after copy")
	}
	return dc
}
