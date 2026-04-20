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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("k8s", func() {
	Describe("PodName", func() {
		var originalPodName string

		BeforeEach(func() {
			originalPodName = os.Getenv(KubePodName)
		})

		AfterEach(func() {
			os.Setenv(KubePodName, originalPodName)
		})

		Context("when the POD_NAME environment variable is set", func() {
			It("should return the pod name", func() {
				expectedPodName := "my-pod"
				os.Setenv(KubePodName, expectedPodName)
				podName, err := PodName()
				Expect(err).NotTo(HaveOccurred())
				Expect(podName).To(Equal(expectedPodName))
			})
		})

		Context("when the POD_NAME environment variable is not set", func() {
			It("should return an error", func() {
				os.Unsetenv(KubePodName)
				_, err := PodName()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("NewKubeClientConfig", func() {
		It("should create a client config with the provided parameters", func() {
			kubeconfigContent := `
apiVersion: v1
clusters:
- cluster:
    server: https://server1:8443
  name: cluster1
- cluster:
    server: https://server2:8443
  name: cluster2
contexts:
- context:
    cluster: cluster1
    user: user1
  name: context1
- context:
    cluster: cluster2
    user: user2
  name: context2
current-context: context1
kind: Config
preferences: {}
users:
- name: user1
  user:
    token: token1
- name: user2
  user:
    token: token2
`
			kubeconfigFile, err := os.CreateTemp("", "kubeconfig")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(kubeconfigFile.Name())

			_, err = kubeconfigFile.Write([]byte(kubeconfigContent))
			Expect(err).NotTo(HaveOccurred())
			err = kubeconfigFile.Close()
			Expect(err).NotTo(HaveOccurred())

			kubeconfigPath := kubeconfigFile.Name()
			context := "context2"
			namespace := "my-namespace"

			clientConfig := NewKubeClientConfig(kubeconfigPath, context, namespace)

			// verify namespace
			ns, _, err := clientConfig.Namespace()
			Expect(err).NotTo(HaveOccurred())
			Expect(ns).To(Equal(namespace))

			// verify that the correct context is used
			restConfig, err := clientConfig.ClientConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(restConfig.Host).To(Equal("https://server2:8443"))
		})
	})
})
