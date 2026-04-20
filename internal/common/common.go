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

// Package common contains some common constants and structures for stolon
package common

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/gofrs/uuid"
)

const (
	// StorePrefix is a default for the store-prefix parameter
	StorePrefix = "stolon/cluster"

	// SentinelLeaderKey defines the key in the KeyValue store that is used for leader election of the sentinel
	SentinelLeaderKey = "sentinel-leader"
)

// PgUnixSocketDirectories is the default unix socket dorectories as passed when starting PostgreSQL
const PgUnixSocketDirectories = "/tmp"

// Role is an enum defining the role of a PostgreSQL instance
type Role string

const (
	// RoleUndefined is set when the role cannot be deducted (e.a. no initialized data directory)
	RoleUndefined Role = "undefined"
	// RolePrimary is set when the data directory belongs to a primary (e.a. no recovery.conf)
	RolePrimary Role = "master"
	// RoleReplica is set when the data directory belongs to a replica (e.a. recovery.conf)
	RoleReplica Role = "standby"
)

// Roles enumerates all possible Role values
var Roles = []Role{
	RoleUndefined,
	RolePrimary,
	RoleReplica,
}

// UID returns a new UID (4 bytes of a UUID)
func UID() string {
	u := uuid.Must(uuid.NewV4())
	return fmt.Sprintf("%x", u[:4])
}

// UUID returns a new UUID
func UUID() string {
	return uuid.Must(uuid.NewV4()).String()
}

const (
	stolonPrefix = "stolon_"
)

// StolonName returns the prefixed name
func StolonName(name string) string {
	return stolonPrefix + name
}

// NameFromStolonName returns the name without the prefix
func NameFromStolonName(stolonName string) string {
	return strings.TrimPrefix(stolonName, stolonPrefix)
}

// IsStolonName returns true if the passed value is prefixed
func IsStolonName(name string) bool {
	return strings.HasPrefix(name, stolonPrefix)
}

// Parameters is a map with PostgreSQL parameters
type Parameters map[string]string

// Equals verifies 2 parameter objects to be the same
func (s Parameters) Equals(is Parameters) bool {
	return reflect.DeepEqual(s, is)
}

// Diff returns the list of pgParameters changed(newly added, existing deleted and value changed)
func (s Parameters) Diff(newParams Parameters) []string {
	var changedParams []string
	for k, v := range newParams {
		if val, ok := s[k]; !ok || v != val {
			changedParams = append(changedParams, k)
		}
	}

	for k := range s {
		if _, ok := newParams[k]; !ok {
			changedParams = append(changedParams, k)
		}
	}
	return changedParams
}

// WriteFileAtomicFunc atomically writes a file, it achieves this by creating a
// temporary file and then moving it. writeFunc is the func that will write
// data to the file.
// This function is taken from
//
//	https://github.com/youtube/vitess/blob/master/go/ioutil2/ioutil.go
//
// Copyright 2012, Google Inc. BSD-license, see licenses/LICENSE-BSD-3-Clause
func WriteFileAtomicFunc(filename string, perm os.FileMode, writeFunc func(f io.Writer) error) error {
	dir, name := path.Split(filename)
	f, err := os.CreateTemp(dir, name)
	if err != nil {
		return err
	}
	err = writeFunc(f)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if permErr := os.Chmod(f.Name(), perm); err == nil {
		err = permErr
	}
	if err == nil {
		err = os.Rename(f.Name(), filename)
	}
	// Any err should result in full cleanup.
	if err != nil {
		if err := os.Remove(f.Name()); err != nil {
			log.Fatalf("failed to remove temp file %s: %v", f.Name(), err)
		}
	}
	return err
}

// WriteFileAtomic atomically writes a file
func WriteFileAtomic(filename string, perm os.FileMode, data []byte) error {
	return WriteFileAtomicFunc(filename, perm,
		func(f io.Writer) error {
			_, err := f.Write(data)
			return err
		})
}
