package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Main", func() {
	Describe("ToPtr", func() {
		Context("with an integer", func() {
			It("should return a pointer to the integer", func() {
				val := 42
				ptr := ToPtr(val)
				Expect(*ptr).To(Equal(val))
			})
		})

		Context("with a string", func() {
			It("should return a pointer to the string", func() {
				val := "hello"
				ptr := ToPtr(val)
				Expect(*ptr).To(Equal(val))
			})
		})

		Context("with a struct", func() {
			It("should return a pointer to the struct", func() {
				type myStruct struct {
					Field1 string
					Field2 int
				}
				val := myStruct{Field1: "test", Field2: 123}
				ptr := ToPtr(val)
				Expect(*ptr).To(Equal(val))
			})
		})
	})
})
