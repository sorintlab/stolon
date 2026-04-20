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

package store

// TODO: implement context

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cluster "github.com/pgvillage-tools/stolon/api/v1"
	"github.com/pgvillage-tools/stolon/internal/util"

	jsonpatch "github.com/evanphx/json-patch"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
)

// ComponentLabelValue is an enum that can be used to set the type of pod
type ComponentLabelValue string

const (
	// DefaultComponentLabel is the defaultlabel to set when no label is defined
	DefaultComponentLabel = "component"

	// KeeperLabelValue states the pod to be a Keeper pod
	KeeperLabelValue ComponentLabelValue = "stolon-keeper"
	// SentinelLabelValue states the pod to be a sentinel pod
	SentinelLabelValue ComponentLabelValue = "stolon-sentinel"
	// ProxyLabelValue states the pod to be a Proxy pod
	ProxyLabelValue ComponentLabelValue = "stolon-proxy"
)

// KubeStore is a struct stores information about the pod, for example the client, name and clustername
type KubeStore struct {
	client       *kubernetes.Clientset
	podName      string
	namespace    string
	clusterName  string
	resourceName string
}

// NewKubeStore return a freshly initialized store in k8s
func NewKubeStore(kubecli *kubernetes.Clientset, podName, namespace, clusterName string) (*KubeStore, error) {
	return &KubeStore{
		client:       kubecli,
		podName:      podName,
		namespace:    namespace,
		clusterName:  clusterName,
		resourceName: fmt.Sprintf("%s-%s", util.KubeResourcePrefix, clusterName),
	}, nil
}

func (s *KubeStore) labelSelector(componentLabel ComponentLabelValue) labels.Selector {
	selector := map[string]string{
		DefaultComponentLabel: string(componentLabel),
		util.KubeClusterLabel: s.clusterName,
	}
	return labels.SelectorFromSet(selector)
}

func (s *KubeStore) patchKubeStatusAnnotation(annotationData []byte) error {
	podsClient := s.client.CoreV1().Pods(s.namespace)
	ctx := context.Background()
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pod, err := podsClient.Get(ctx, s.podName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get latest version of pod: %v", err)
		}

		oldPodJSON, err := json.Marshal(pod)
		if err != nil {
			return fmt.Errorf("failed to marshal pod: %v", err)
		}

		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		pod.Annotations[util.KubeStatusAnnnotation] = string(annotationData)

		newPodJSON, err := json.Marshal(pod)
		if err != nil {
			return fmt.Errorf("failed to marshal pod: %v", err)
		}

		patchBytes, err := jsonpatch.CreateMergePatch(oldPodJSON, newPodJSON)
		if err != nil {
			return fmt.Errorf("failed to create pod merge patch: %v", err)
		}
		_, err = podsClient.Patch(ctx, s.podName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	})
	if retryErr != nil {
		return fmt.Errorf("update failed: %v", retryErr)
	}
	return nil
}

// AtomicPutClusterData is an atomic way to write ClusterData to the configmap
func (s *KubeStore) AtomicPutClusterData(ctx context.Context, cd *cluster.Data, previous *KVPair) (*KVPair, error) {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return nil, err
	}
	epsClient := s.client.CoreV1().ConfigMaps(s.namespace)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := epsClient.Get(ctx, s.resourceName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get latest version of configmap: %v", err)
		}
		if !apierrors.IsNotFound(err) {
			// configmap exists

			if previous == nil {
				if result.Annotations != nil {
					_, ok := result.Annotations[util.KubeClusterDataAnnotation]
					if ok {
						// cd exists but previous is nil
						return ErrKeyModified
					}
				}
			}

			if previous != nil {
				if result.Annotations == nil {
					// empty annotations but previous isn't nil
					return ErrKeyModified
				}
				curcd, ok := result.Annotations[util.KubeClusterDataAnnotation]
				if !ok {
					// no cd but previous isn't nil
					return ErrKeyModified
				}
				// check that the previous cd is the same as the current one in the
				// configmap annotation
				if string(previous.Value) != string(curcd) {
					return ErrKeyModified
				}
			}
			if result.Annotations == nil {
				result.Annotations = map[string]string{}
			}
			result.Annotations[util.KubeClusterDataAnnotation] = string(cdj)
			_, err = epsClient.Update(ctx, result, metav1.UpdateOptions{})
			return err
		}
		// configmap does not exists

		// previous isn't nil but configmap doesn't exists
		if previous != nil {
			return ErrKeyModified
		}
		annotations := map[string]string{util.KubeClusterDataAnnotation: string(cdj)}
		_, err = epsClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        s.resourceName,
				Annotations: annotations,
			},
		}, metav1.CreateOptions{})
		return err
	})
	if retryErr != nil {
		return nil, fmt.Errorf("update failed: %v", retryErr)
	}
	return &KVPair{Value: cdj}, nil
}

// PutClusterData writes ClusterData to the configmap
func (s *KubeStore) PutClusterData(ctx context.Context, cd *cluster.Data) error {
	cdj, err := json.Marshal(cd)
	if err != nil {
		return err
	}
	epsClient := s.client.CoreV1().ConfigMaps(s.namespace)

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := epsClient.Get(ctx, s.resourceName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get latest version of configmap: %v", err)
		}
		if !apierrors.IsNotFound(err) {
			// configmap exists
			if result.Annotations == nil {
				result.Annotations = map[string]string{}
			}
			result.Annotations[util.KubeClusterDataAnnotation] = string(cdj)
			_, err = epsClient.Update(ctx, result, metav1.UpdateOptions{})
			return err
		}
		// configmap does not exists
		annotations := map[string]string{util.KubeClusterDataAnnotation: string(cdj)}
		_, err = epsClient.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        s.resourceName,
				Annotations: annotations,
			},
		}, metav1.CreateOptions{})
		return err
	})
	if retryErr != nil {
		return fmt.Errorf("update failed: %v", retryErr)
	}
	return nil
}

// GetClusterData reads ClusterData from the configmap
func (s *KubeStore) GetClusterData(ctx context.Context) (*cluster.Data, *KVPair, error) {
	epsClient := s.client.CoreV1().ConfigMaps(s.namespace)
	result, err := epsClient.Get(ctx, s.resourceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get latest version of configmap: %v", err)
	}
	cdj, ok := result.Annotations[util.KubeClusterDataAnnotation]
	if !ok {
		return nil, nil, nil
	}

	var cd *cluster.Data
	if err := json.Unmarshal([]byte(cdj), &cd); err != nil {
		return nil, nil, err
	}

	return cd, &KVPair{Value: []byte(cdj)}, nil
}

// SetKeeperInfo updates info to annotations on a Keeper pod
func (s *KubeStore) SetKeeperInfo(_ context.Context, _ string, ms *cluster.KeeperInfo, _ time.Duration) error {
	msj, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	return s.patchKubeStatusAnnotation(msj)
}

// GetKeepersInfo reads info from annotations on all Keeper pods
func (s *KubeStore) GetKeepersInfo(ctx context.Context) (cluster.KeepersInfo, error) {
	keepers := cluster.KeepersInfo{}

	podsClient := s.client.CoreV1().Pods(s.namespace)

	listOpts := metav1.ListOptions{
		LabelSelector: s.labelSelector(KeeperLabelValue).String(),
	}
	result, err := podsClient.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest version of pod: %v", err)
	}

	pods := result.Items
	for _, pod := range pods {
		var ki cluster.KeeperInfo
		if kij, ok := pod.Annotations[util.KubeStatusAnnnotation]; ok {
			err = json.Unmarshal([]byte(kij), &ki)
			if err != nil {
				return nil, err
			}
			keepers[ki.UID] = &ki
		}
	}
	return keepers, nil
}

// SetSentinelInfo updates info on a Sentinel pod
func (s *KubeStore) SetSentinelInfo(_ context.Context, si *cluster.SentinelInfo, _ time.Duration) error {
	sij, err := json.Marshal(si)
	if err != nil {
		return err
	}
	return s.patchKubeStatusAnnotation(sij)
}

// GetSentinelsInfo reads info on all Sentinel pods
func (s *KubeStore) GetSentinelsInfo(ctx context.Context) (cluster.SentinelsInfo, error) {
	ssi := cluster.SentinelsInfo{}

	podsClient := s.client.CoreV1().Pods(s.namespace)

	listOpts := metav1.ListOptions{
		LabelSelector: s.labelSelector(SentinelLabelValue).String(),
	}
	result, err := podsClient.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest version of pod: %v", err)
	}

	pods := result.Items
	for _, pod := range pods {
		var si cluster.SentinelInfo
		if sij, ok := pod.Annotations[util.KubeStatusAnnnotation]; ok {
			err = json.Unmarshal([]byte(sij), &si)
			if err != nil {
				return nil, err
			}
		}
		ssi = append(ssi, &si)
	}
	return ssi, nil
}

// SetProxyInfo updates info to annotations on a Proxy pod
func (s *KubeStore) SetProxyInfo(_ context.Context, pi *cluster.ProxyInfo, _ time.Duration) error {
	pij, err := json.Marshal(pi)
	if err != nil {
		return err
	}
	return s.patchKubeStatusAnnotation(pij)
}

// GetProxiesInfo reads info from annotations on all Proxy pods
func (s *KubeStore) GetProxiesInfo(ctx context.Context) (cluster.ProxiesInfo, error) {
	psi := cluster.ProxiesInfo{}

	podsClient := s.client.CoreV1().Pods(s.namespace)

	listOpts := metav1.ListOptions{
		LabelSelector: s.labelSelector(ProxyLabelValue).String(),
	}
	result, err := podsClient.List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest version of pod: %v", err)
	}

	pods := result.Items
	for _, pod := range pods {
		var pi cluster.ProxyInfo
		if pij, ok := pod.Annotations[util.KubeStatusAnnnotation]; ok {
			err = json.Unmarshal([]byte(pij), &pi)
			if err != nil {
				return nil, err
			}
			psi[pi.UID] = &pi
		}
	}
	return psi, nil
}

// KubeElection takes care of the election proces for stolon on k8s
type KubeElection struct {
	client       *kubernetes.Clientset
	podName      string
	namespace    string
	resourceName string

	running bool

	electedCh chan bool
	errCh     chan error

	ctx    context.Context
	cancel context.CancelFunc

	rl resourcelock.Interface
}

// NewKubeElection returns a freshly initialized KubeElection resource
func NewKubeElection(
	kubecli *kubernetes.Clientset,
	podName, namespace, clusterName, candidateUID string,
) (*KubeElection, error) {
	resourceName := fmt.Sprintf("%s-%s", util.KubeResourcePrefix, clusterName)
	rl, err := resourcelock.New("configmaps",
		namespace,
		resourceName,
		kubecli.CoreV1(),
		nil,
		resourcelock.ResourceLockConfig{
			Identity:      candidateUID,
			EventRecorder: createRecorder(kubecli, "stolon-sentinel", namespace),
		})
	// https://github.com/kubernetes/client-go/blob/master/tools/leaderelection/resourcelock/interface.go
	if err != nil {
		return nil, fmt.Errorf("error creating lock: %v", err)
	}

	return &KubeElection{
		client:       kubecli,
		podName:      podName,
		namespace:    namespace,
		resourceName: resourceName,
		rl:           rl,
	}, nil
}

// RunForElection starts a campaign for getting elected
func (e *KubeElection) RunForElection(ctx context.Context) (<-chan bool, <-chan error) {
	if e.running {
		panic("already running")
	}

	e.electedCh = make(chan bool)
	e.errCh = make(chan error)
	e.ctx, e.cancel = context.WithCancel(ctx)

	e.running = true
	go e.campaign()

	return e.electedCh, e.errCh
}

// Stop cancels campaign
func (e *KubeElection) Stop() {
	if !e.running {
		panic("not running")
	}
	e.cancel()
	e.running = false
}

// Leader returns the current leader
func (e *KubeElection) Leader() (string, error) {
	ctx := context.Background()
	ler, _, err := e.rl.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get leader election record: %v", err)
	}
	if ler == nil {
		return "", nil
	}

	return ler.HolderIdentity, nil
}

func (e *KubeElection) campaign() {
	defer close(e.electedCh)
	defer close(e.errCh)

	for {
		e.electedCh <- false

		leaderelection.RunOrDie(e.ctx, leaderelection.LeaderElectionConfig{
			Lock:          e.rl,
			LeaseDuration: 15 * time.Second,
			RenewDeadline: 10 * time.Second,
			RetryPeriod:   2 * time.Second,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(context.Context) {
					e.electedCh <- true
				},
				OnStoppedLeading: func() {
					e.electedCh <- false
				},
			},
		})
	}
}

func createRecorder(kubecli kubernetes.Interface, name, namespace string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(kubecli.CoreV1().RESTClient()).Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: name})
}
