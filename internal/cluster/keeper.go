package cluster

import (
	"reflect"
	"sort"
	"time"

	"github.com/mitchellh/copystructure"
)

// KeeperSpec defines a spec for a Keeper resource
type KeeperSpec struct{}

// KeeperStatus defines all staus fields on a Keeper
type KeeperStatus struct {
	Healthy         bool      `json:"healthy,omitempty"`
	LastHealthyTime time.Time `json:"lastHealthyTime,omitempty"`

	BootUUID string `json:"bootUUID,omitempty"`

	PostgresBinaryVersion PostgresBinaryVersion `json:"postgresBinaryVersion,omitempty"`

	ForceFail bool `json:"forceFail,omitempty"`

	CanBeMaster             *bool `json:"canBeMaster,omitempty"`
	CanBeSynchronousReplica *bool `json:"canBeSynchronousReplica,omitempty"`
}

// Keeper combines the spec, status and other fields belonging to a keeper
type Keeper struct {
	// Keeper ID
	UID        string    `json:"uid,omitempty"`
	Generation int64     `json:"generation,omitempty"`
	ChangeTime time.Time `json:"changeTime,omitempty"`

	Spec *KeeperSpec `json:"spec,omitempty"`

	Status KeeperStatus `json:"status,omitempty"`
}

// NewKeeperFromKeeperInfo returns a freshly initialized keeper created from a KeeperInfo object
func NewKeeperFromKeeperInfo(ki *KeeperInfo) *Keeper {
	return &Keeper{
		UID:        ki.UID,
		Generation: InitialGeneration,
		ChangeTime: time.Time{},
		Spec:       &KeeperSpec{},
		Status: KeeperStatus{
			Healthy:         true,
			LastHealthyTime: time.Now(),
			BootUUID:        ki.BootUUID,
		},
	}
}

// Keepers can store all keepers for a cluster
type Keepers map[string]*Keeper

// SortedKeys returns all keys of the Keepers as a sorted list
func (kss Keepers) SortedKeys() []string {
	keys := []string{}
	for k := range kss {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// KeepersInfo stores all KeeperInfo resources belonging to this cluster
type KeepersInfo map[string]*KeeperInfo

// DeepCopy returns a copy of the KeepersInfo resource
func (k KeepersInfo) DeepCopy() (dc KeepersInfo) {
	if k == nil {
		return nil
	}
	nkis := KeepersInfo{}
	for k, v := range k {
		nkis[k] = v.DeepCopy()
	}
	return nkis
}

// KeeperInfo can store all info belonging to a Keeper
type KeeperInfo struct {
	// An unique id for this info, used to know when this the keeper info
	// has been updated
	InfoUID string `json:"infoUID,omitempty"`

	UID        string `json:"uid,omitempty"`
	ClusterUID string `json:"clusterUID,omitempty"`
	BootUUID   string `json:"bootUUID,omitempty"`

	PostgresBinaryVersion PostgresBinaryVersion `json:"postgresBinaryVersion,omitempty"`

	PostgresState *PostgresState `json:"postgresState,omitempty"`

	CanBeMaster             *bool `json:"canBeMaster,omitempty"`
	CanBeSynchronousReplica *bool `json:"canBeSynchronousReplica,omitempty"`
}

// TODO: replace all specific DeepCopy method bodies with call of a generic function

// DeepCopy returns a copy of the KeeperInfo resource
func (k *KeeperInfo) DeepCopy() (dc *KeeperInfo) {
	var ok bool
	if k == nil {
		return nil
	}
	if nk, err := copystructure.Copy(k); err != nil {
		panic(err)
	} else if !reflect.DeepEqual(k, nk) {
		panic("not equal")
	} else if dc, ok = nk.(*KeeperInfo); !ok {
		panic("different type after copy")
	}
	return dc
}
