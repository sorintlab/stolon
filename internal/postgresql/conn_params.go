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
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strings"
)

// This is based on github.com/lib/pq

// ConnParams defines key/value pairs for connecting to PostgreSQL
type ConnParams map[ConnParamKey]string

// Set can add/update a key
func (cp ConnParams) Set(k ConnParamKey, v string) {
	cp[k] = v
}

// Get can retrieve a key/value pai
func (cp ConnParams) Get(k ConnParamKey) (v string) {
	return cp[k]
}

// Del can remove a key
func (cp ConnParams) Del(k ConnParamKey) {
	delete(cp, k)
}

// Isset returns true if the key is set
func (cp ConnParams) Isset(k ConnParamKey) bool {
	_, ok := cp[k]
	return ok
}

// Equals checks 2 ConnParams to be the same
func (cp ConnParams) Equals(cp2 ConnParams) bool {
	return reflect.DeepEqual(cp, cp2)
}

// Copy returns a shallow copy
func (cp ConnParams) Copy() ConnParams {
	ncp := ConnParams{}
	for k, v := range cp {
		ncp[k] = v
	}
	return ncp
}

// ConnString returns a connection string, its entries are sorted so the
// returned string can be reproducible and comparable
func (cp ConnParams) ConnString() string {
	var kvs []string
	escaper := strings.NewReplacer(` `, `\ `, `'`, `\'`, `\`, `\\`)
	for k, v := range cp {
		if v != "" {
			kvs = append(kvs, fmt.Sprintf("%s=%s", k, escaper.Replace(v)))
		}
	}
	sort.Strings(kvs)
	return strings.Join(kvs, " ")
}

// WithUser returns a clone with the user fields set to the specified userName
func (cp ConnParams) WithUser(userName string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeyUser] = userName
	return q
}

// WithHost returns a clone with the host fields set to the specified hostName
func (cp ConnParams) WithHost(hostName string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeyHost] = hostName
	return q
}

// WithDbName returns a clone with the host fields set to the specified hostName
func (cp ConnParams) WithDbName(dbName string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeyDbName] = dbName
	return q
}

// WithPort returns a clone with the port fields set as specified
func (cp ConnParams) WithPort(port uint) ConnParams {
	return cp.WithSPort(fmt.Sprintf("%d", port))
}

// WithSPort returns a clone with the port fields set as specified
func (cp ConnParams) WithSPort(port string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeyPort] = port
	return q
}

// WithAppName returns a clone with the port fields set as specified
func (cp ConnParams) WithAppName(appName string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeyAppName] = appName
	return q
}

// WithSSLMode returns a clone with the port fields set as specified
func (cp ConnParams) WithSSLMode(sslMode string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeySSLMode] = sslMode
	return q
}

// WithPassword returns a clone with the password field set as specified
func (cp ConnParams) WithPassword(password string) ConnParams {
	q := maps.Clone(cp)
	q[ConnParamKeyPassword] = password
	return q
}
