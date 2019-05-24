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

package common

import (
	"fmt"
	"regexp"
	"strconv"
)

// Bytesize is useful to represent bytes and perform operations on it
type Bytesize int

// Common bytesizes than can be used
const (
	Byte     Bytesize = 1
	Kilobyte          = 1024 * Byte
	Megabyte          = 1024 * Kilobyte
)

const pattern = `([0-9]+)(k|M)`

// ParseBytesize allows to parse the given input into Bytesize
// Allowed formats is [:digit:] suffixed by k (kilobyte) or M (Megabyte)
func ParseBytesize(input string) (Bytesize, error) {
	re := regexp.MustCompile(pattern)
	allMatches := re.FindStringSubmatch(input)
	if len(allMatches) != 3 {
		return 0, fmt.Errorf("unable to parse %s: must be of the format %s", input, pattern)
	}

	size, err := strconv.Atoi(allMatches[1])
	if err != nil {
		return 0, fmt.Errorf("unable to parse as integer %s: %v", allMatches[1], err)
	}

	unit := allMatches[2]
	switch unit {
	case "k":
		return Bytesize(size) * Kilobyte, nil
	case "M":
		return Bytesize(size) * Megabyte, nil
	default:
		return 0, fmt.Errorf("%s can either be suffixed with k or M", allMatches[2])
	}
}
