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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"errors"
	"os"
	"os/user"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("User", func() {
	var originalUserEnv string
	var originalCurrentUserFunc func() (*user.User, error)

	BeforeEach(func() {
		originalUserEnv = os.Getenv("USER")
		originalCurrentUserFunc = currentUserFunc
	})

	AfterEach(func() {
		os.Setenv("USER", originalUserEnv)
		currentUserFunc = originalCurrentUserFunc
	})

	Context("when user.Current() succeeds", func() {
		BeforeEach(func() {
			currentUserFunc = func() (*user.User, error) {
				return &user.User{Username: "mockuser_current"}, nil
			}
		})

		It("should return the username from user.Current()", func() {
			os.Setenv("USER", "env_user") // This should be ignored
			username, err := GetUser()
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("mockuser_current"))
		})
	})

	Context("when user.Current() fails and USER environment variable is set", func() {
		BeforeEach(func() {
			currentUserFunc = func() (*user.User, error) {
				return nil, errors.New("mock user.Current() error")
			}
		})

		It("should return the username from the USER environment variable", func() {
			expectedUser := "env_user"
			os.Setenv("USER", expectedUser)
			username, err := GetUser()
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal(expectedUser))
		})
	})

	Context("when user.Current() fails and USER environment variable is not set", func() {
		BeforeEach(func() {
			currentUserFunc = func() (*user.User, error) {
				return nil, errors.New("mock user.Current() error")
			}
		})

		It("should return an error", func() {
			os.Unsetenv("USER")
			username, err := GetUser()
			Expect(username).To(BeEmpty())
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("cannot detect current user"))
		})
	})
})
