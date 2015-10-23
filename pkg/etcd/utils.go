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
package etcd

import etcd "github.com/sorintlab/stolon/Godeps/_workspace/src/github.com/coreos/etcd/client"

// IsEtcdNotFound returns true if err is an etcd not found error.
func IsEtcdNotFound(err error) bool {
	return isEtcdErrorNum(err, etcd.ErrorCodeKeyNotFound)
}

// IsEtcdNodeExist returns true if err is an etcd node aleady exist error.
func IsEtcdNodeExist(err error) bool {
	return isEtcdErrorNum(err, etcd.ErrorCodeNodeExist)
}

// IsEtcdTestFailed returns true if err is an etcd write conflict.
func IsEtcdTestFailed(err error) bool {
	return isEtcdErrorNum(err, etcd.ErrorCodeTestFailed)
}

// isEtcdErrorNum returns true if err is an etcd error, whose errorCode matches errorCode
func isEtcdErrorNum(err error, errorCode int) bool {
	etcdError, ok := err.(etcd.Error)
	return ok && etcdError.Code == errorCode
}
