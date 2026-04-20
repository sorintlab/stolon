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

package common_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/stolon/internal/common"
)

var _ = Describe("TLS", func() {
	var (
		validCertsDir                             string
		invalidCertsDir                           string
		caCertPath, serverCertPath, serverKeyPath string
	)

	BeforeEach(func() {
		var err error
		validCertsDir, err = filepath.Abs("../../tests/testcerts/")
		Expect(err).ToNot(HaveOccurred())

		invalidCertsDir = filepath.Join(GinkgoT().TempDir(), "tls_test_certs")
		err = os.MkdirAll(invalidCertsDir, 0755)
		Expect(err).NotTo(HaveOccurred())

		caCertPath = filepath.Join(validCertsDir, "ca.crt")
		serverCertPath = filepath.Join(validCertsDir, "server.crt")
		serverKeyPath = filepath.Join(validCertsDir, "server.key")
	})

	AfterEach(func() {
	})

	Context("NewTLSConfig", func() {
		It("should return a tls.Config with CA certificate when only caFile is provided", func() {
			config, err := common.NewTLSConfig("", "", caCertPath, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).NotTo(BeNil())
			Expect(config.RootCAs).NotTo(BeNil())
			Expect(config.Certificates).To(BeEmpty())
			Expect(config.InsecureSkipVerify).To(BeFalse())
		})

		It("should return a tls.Config with client certificate when certFile and keyFile are provided", func() {
			config, err := common.NewTLSConfig(serverCertPath, serverKeyPath, "", false)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).NotTo(BeNil())
			Expect(config.RootCAs).To(BeNil())
			Expect(config.Certificates).ToNot(BeEmpty())
			Expect(config.InsecureSkipVerify).To(BeFalse())
		})

		It("should return a tls.Config with CA and client certificates when all files are provided", func() {
			config, err := common.NewTLSConfig(serverCertPath, serverKeyPath, caCertPath, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).NotTo(BeNil())
			Expect(config.RootCAs).NotTo(BeNil())
			Expect(config.Certificates).ToNot(BeEmpty())
			Expect(config.InsecureSkipVerify).To(BeFalse())
		})

		It("should set InsecureSkipVerify to true when specified", func() {
			config, err := common.NewTLSConfig("", "", "", true)
			Expect(err).ToNot(HaveOccurred())
			Expect(config).NotTo(BeNil())
			Expect(config.InsecureSkipVerify).To(BeTrue())
		})

		It("should return an error when caFile does not exist", func() {
			config, err := common.NewTLSConfig("", "", "/non/existent/ca.crt", false)
			Expect(err).To(HaveOccurred())
			Expect(config).To(BeNil())
		})

		It("should return an error when certFile does not exist", func() {
			config, err := common.NewTLSConfig("/non/existent/server.crt", serverKeyPath, "", false)
			Expect(err).To(HaveOccurred())
			Expect(config).To(BeNil())
		})

		It("should return an error when keyFile does not exist", func() {
			config, err := common.NewTLSConfig(serverCertPath, "/non/existent/server.key", "", false)
			Expect(err).To(HaveOccurred())
			Expect(config).To(BeNil())
		})

		It("should return an error with invalid caFile content", func() {
			invalidCaPath := filepath.Join(invalidCertsDir, "invalid-ca.crt")
			err := os.WriteFile(invalidCaPath, []byte("invalid cert content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			config, err := common.NewTLSConfig("", "", invalidCaPath, false)
			Expect(err).To(HaveOccurred())
			Expect(config).To(BeNil())
		})

		It("should return an error with invalid certFile/keyFile content", func() {
			invalidCertPath := filepath.Join(invalidCertsDir, "invalid-server.crt")
			invalidKeyPath := filepath.Join(invalidCertsDir, "invalid-server.key")
			err := os.WriteFile(invalidCertPath, []byte("invalid cert content"), 0644)
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(invalidKeyPath, []byte("invalid key content"), 0644)
			Expect(err).ToNot(HaveOccurred())

			config, err := common.NewTLSConfig(invalidCertPath, invalidKeyPath, "", false)
			Expect(err).To(HaveOccurred())
			Expect(config).To(BeNil())
		})
	})
})

// copyFile is a helper function to copy a file from src to dst.
func copyFile(dst, src string) {
	input, err := os.ReadFile(src)
	Expect(err).ToNot(HaveOccurred())

	err = os.WriteFile(dst, input, 0644)
	Expect(err).ToNot(HaveOccurred())
}
