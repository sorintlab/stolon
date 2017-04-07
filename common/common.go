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

package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/satori/go.uuid"
)

const (
	StoreBasePath = "stolon/cluster"

	SentinelLeaderKey = "sentinel-leader"
)

type Role string

const (
	RoleUndefined Role = "undefined"
	RoleMaster    Role = "master"
	RoleStandby   Role = "standby"
)

func UID() string {
	u := uuid.NewV4()
	return fmt.Sprintf("%x", u[:4])
}

func UUID() string {
	return uuid.NewV4().String()
}

const (
	stolonPrefix = "stolon_"
)

func StolonName(name string) string {
	return stolonPrefix + name
}

func NameFromStolonName(stolonName string) string {
	return strings.TrimPrefix(stolonName, stolonPrefix)
}

func IsStolonName(name string) bool {
	return strings.HasPrefix(name, stolonPrefix)
}

type Parameters map[string]string

func (s Parameters) Equals(is Parameters) bool {
	return reflect.DeepEqual(s, is)
}

// Write file to temp and atomically move when everything else succeeds.
// This function is taken from
//   https://github.com/youtube/vitess/blob/master/go/ioutil2/ioutil.go
// Copyright 2012, Google Inc. BSD-license
func WriteFileAtomic(filename string, data []byte, perm os.FileMode) error {
	dir, name := path.Split(filename)
	f, err := ioutil.TempFile(dir, name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
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
		os.Remove(f.Name())
	}
	return err
}
