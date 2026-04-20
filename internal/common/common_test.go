// Copyright 2026 PgVillage
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
	"io"
	"os"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/stolon/internal/common"
	"github.com/pgvillage-tools/stolon/internal/util"
)

var _ = Describe("UID", func() {
	It("should return a new unique uid", func() {
		uids := []string{common.UID()}
		for i := 0; i < 100; i++ {
			uid := common.UID()
			Expect(slices.Contains(uids, uid)).To(BeFalse(), "Expected every new uid to be unique, but %s is duplicate after %d rounds", uid, len(uids))
			uids = append(uids, uid)
		}
	})
})

var _ = Describe("UUID", func() {
	It("should return a new unique uuid", func() {
		uuids := []string{common.UUID()}
		for i := 0; i < 100; i++ {
			uuid := common.UUID()
			Expect(slices.Contains(uuids, uuid)).To(BeFalse(), "Expected every new uuid to be unique, but %s is duplicate after %d rounds", uuid, len(uuids))
			uuids = append(uuids, uuid)
		}
	})
})

var _ = Describe("Parameters", func() {
	Describe("Diff", func() {
		It("should return the changed parameters", func() {
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
			Expect(util.CompareStringSliceNoOrder(expectedDiff, diff)).To(BeTrue(), "Expected diff is %v, but got %v", expectedDiff, diff)
		})
	})
})

var _ = Describe("StolonName", func() {
	It("should return the stolon prefixed name", func() {
		Expect(common.StolonName("test")).To(Equal("stolon_test"))
	})
})

var _ = Describe("NameFromStolonName", func() {
	It("should return the name without the stolon prefix", func() {
		Expect(common.NameFromStolonName("stolon_test")).To(Equal("test"))
	})
})

var _ = Describe("IsStolonName", func() {
	It("should return true if the name is a stolon name", func() {
		Expect(common.IsStolonName("stolon_test")).To(BeTrue())
	})

	It("should return false if the name is not a stolon name", func() {
		Expect(common.IsStolonName("test")).To(BeFalse())
	})
})

var _ = Describe("Parameters.Equals", func() {
	It("should return true for equal parameters", func() {
		p1 := common.Parameters{"a": "1", "b": "2"}
		p2 := common.Parameters{"a": "1", "b": "2"}
		Expect(p1.Equals(p2)).To(BeTrue())
	})

	It("should return false for non-equal parameters", func() {
		p1 := common.Parameters{"a": "1", "b": "2"}
		p2 := common.Parameters{"a": "1", "b": "3"}
		Expect(p1.Equals(p2)).To(BeFalse())
	})
})

var _ = Describe("WriteFileAtomic", func() {
	var tmpDir string
	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "stolon-test")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("should write the data to a file", func() {
		filePath := tmpDir + "/test.txt"
		data := []byte("test data")
		err := common.WriteFileAtomic(filePath, 0644, data)
		Expect(err).ToNot(HaveOccurred())

		readData, err := os.ReadFile(filePath)
		Expect(err).ToNot(HaveOccurred())
		Expect(readData).To(Equal(data))
	})
})

var _ = Describe("WriteFileAtomicFunc", func() {
	var tmpDir string
	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "stolon-test")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("should write the data to a file using the provided function", func() {
		filePath := tmpDir + "/test.txt"
		data := []byte("test data")
		err := common.WriteFileAtomicFunc(filePath, 0644, func(f io.Writer) error {
			_, err := f.Write(data)
			return err
		})
		Expect(err).ToNot(HaveOccurred())

		readData, err := os.ReadFile(filePath)
		Expect(err).ToNot(HaveOccurred())
		Expect(readData).To(Equal(data))
	})

	It("should return an error if the write function returns an error", func() {
		filePath := tmpDir + "/test.txt"
		err := common.WriteFileAtomicFunc(filePath, 0644, func(f io.Writer) error {
			return io.ErrShortWrite
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(io.ErrShortWrite))
	})
})
