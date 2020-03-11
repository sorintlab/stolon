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

// +build linux

package timer

import (
	"syscall"
	"unsafe"
)

const (
	// from /usr/include/linux/time.h
	CLOCK_MONOTONIC = 1
)

// TODO(sgotti) for the moment just use a syscall so it'll work on all linux
// architectures. It's slower than using a vdso but we don't have such performance
// needs. Let's wait for a stdlib native monotonic clock.
func Now() int64 {
	var ts syscall.Timespec
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, CLOCK_MONOTONIC, uintptr(unsafe.Pointer(&ts)), 0)
	nsec := ts.Nano()
	return nsec
}
