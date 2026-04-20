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

// Package util has all kind of util functions to support stolon
package util

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
)

const (
	// KubePodName defines the env variabele for a Pod when used in k8s
	KubePodName = "POD_NAME"

	// KubeResourcePrefix is a hardcoded prefix for the configmap when running stolon on k8s
	KubeResourcePrefix = "stolon-cluster"

	// KubeClusterLabel is a hardcoded label key to be set to the name of the cluster for all k8s resources belonging to
	// the cluster
	KubeClusterLabel = "stolon-cluster"

	// KubeClusterDataAnnotation is the key of the annotation that should be set on a ClusterData configmap
	KubeClusterDataAnnotation = "stolon-clusterdata"

	// KubeStatusAnnnotation is the key of the annotation that should be set to define the status for every Pod for
	// this cluster
	KubeStatusAnnnotation = "stolon-status"
)

// PodName returns the name of the Pod from the env variabele
func PodName() (string, error) {
	podName := os.Getenv(KubePodName)
	if len(podName) == 0 {
		return "", fmt.Errorf("missing required env variable %q", KubePodName)
	}
	return podName, nil
}

// NewKubeClientConfig return a kube client config that will by default use an
// in cluster client config or, if not available or overridden, an external client
// config using the default client behavior used also by kubectl.
func NewKubeClientConfig(kubeconfigPath, context, namespace string) clientcmd.ClientConfig {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	if kubeconfigPath != "" {
		rules.ExplicitPath = kubeconfigPath
	}

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}

	if context != "" {
		overrides.CurrentContext = context
	}

	if namespace != "" {
		overrides.Context.Namespace = namespace
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
}
