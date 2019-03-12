// Copyright 2019 Sorint.lab
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

package register

import (
	"reflect"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/sorintlab/stolon/internal/cluster"
)

func TestNewServiceInfo(t *testing.T) {
	t.Run("should join listen address and port", func(t *testing.T) {
		tags := Tags{"tags"}
		id := "unique"
		actual, _ := NewServiceInfo("test", &cluster.DB{
			UID: id,
			Status: cluster.DBStatus{
				ListenAddress: "127.0.0.1",
				Port:          "5432"},
		}, tags, true)

		if actual.Name != "test" {
			t.Errorf("expected name to be %s but got %s", "test", actual.Name)
		} else if !reflect.DeepEqual(tags, actual.Tags) {
			t.Errorf("expected tags to be %v but got %v", tags, actual.Name)
		} else if actual.Address != "127.0.0.1" {
			t.Errorf("expected address to be %v but got %v", "127.0.0.1", actual.Address)
		} else if actual.Port != 5432 {
			t.Errorf("expected port to be %v but got %v", "5432", actual.Port)
		} else if actual.ID != id {
			t.Errorf("expected id to be %v but got %v", id, actual.ID)
		} else if actual.IsMaster != true {
			t.Errorf("expected isMaster to be %v but got %v", true, actual.ID)
		}
	})

	t.Run("should return error if port is invalid", func(t *testing.T) {
		actual, err := NewServiceInfo("test", &cluster.DB{
			UID: "unique",
			Status: cluster.DBStatus{
				ListenAddress: "127.0.0.1",
				Port:          "cat"},
		}, []string{}, true)

		if actual != nil {
			t.Errorf("expected service info to be nil")
		} else if err == nil || err.Error() != "invalid database port 'cat' for test with uid unique" {
			t.Errorf("expected invalid database port error but was %s", err.Error())
		}
	})
}

func TestConsulAgentServiceRegistration(t *testing.T) {
	t.Run("should return consul agent service registration", func(t *testing.T) {
		tags := Tags{"tags"}
		id := "unique"
		service, _ := NewServiceInfo("test", &cluster.DB{
			UID: id,
			Status: cluster.DBStatus{
				ListenAddress: "127.0.0.1",
				Port:          "5432"},
		}, tags, true)

		actual := service.ConsulAgentServiceRegistration()

		if actual == nil {
			t.Errorf("expected consul agent service registration not to be nil")
		}
		if actual.ID != service.ID {
			t.Errorf("expected id to be %s but was %s", service.ID, actual.ID)
		} else if actual.Name != service.Name {
			t.Errorf("expected name to be %s but was %s", service.Name, actual.Name)
		} else if actual.Address != service.Address {
			t.Errorf("expected Address to be %s but was %s", service.Address, actual.Name)
		} else if !service.Tags.Compare(actual.Tags) {
			t.Errorf("expected tags to be %v but was %v", service.Tags, actual.Tags)
		} else if actual.Port != service.Port {
			t.Errorf("expected port to be %d but was %d", service.Port, actual.Port)
		} else if actual.Check.TCP != service.Check.TCP {
			t.Errorf("expected check tcp to be %s but was %s", service.Check.TCP, actual.Check.TCP)
		} else if actual.Check.Interval != service.Check.Interval {
			t.Errorf("expected check interval to be %s but was %s", service.Check.Interval, actual.Check.Interval)
		}
	})
}

func TestNewServiceInfoFromConsulService(t *testing.T) {
	t.Run("should return service info", func(t *testing.T) {
		service := api.CatalogService{
			ServiceName:    "service",
			ServiceTags:    Tags{"one", "two"},
			ServiceID:      "id",
			ServicePort:    1234,
			ServiceAddress: "address",
		}

		info := NewServiceInfoFromConsulService(service)

		if info.Name != service.ServiceName {
			t.Errorf("expected name to be %s but was %s", service.ServiceName, info.Name)
		} else if info.ID != service.ServiceID {
			t.Errorf("expected id to be %s but was %s", service.ID, info.ID)
		} else if !info.Tags.Compare(service.ServiceTags) {
			t.Errorf("expected tags to be %v but was %v", service.ServiceTags, info.Tags)
		} else if info.Port != service.ServicePort {
			t.Errorf("expected port to be %d but was %d", service.ServicePort, info.Port)
		} else if info.Address != service.ServiceAddress {
			t.Errorf("expected address to be %s but was %s", service.ServiceAddress, info.Address)
		}
	})
}

func TestCompareTags(t *testing.T) {
	t.Run("should return true if tags are equal", func(t *testing.T) {
		tag1 := Tags{"slave", "master"}
		tag2 := Tags{"slave", "master"}
		if !tag1.Compare(tag2) {
			t.Errorf("expected to be true")
		}
	})

	t.Run("should return false if tags are not equal", func(t *testing.T) {
		tag1 := Tags{"slave", "master"}
		tag2 := Tags{"slave"}
		if tag1.Compare(tag2) {
			t.Errorf("expected to be false")
		}
	})
}

func TestCompareServiceInfo(t *testing.T) {
	t.Run("should return true if ServiceInfo(s) are equal", func(t *testing.T) {
		serviceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		anotherServiceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		if !serviceInfo.Compare(anotherServiceInfo) {
			t.Errorf("expected to be true")
		}
	})

	t.Run("should return false if service names are not equal", func(t *testing.T) {
		serviceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		anotherServiceInfo := ServiceInfo{
			Name:    "test1",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		if serviceInfo.Compare(anotherServiceInfo) {
			t.Errorf("expected to be false")
		}
	})

	t.Run("should return false if service address are not equal", func(t *testing.T) {
		serviceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.2",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		anotherServiceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		if serviceInfo.Compare(anotherServiceInfo) {
			t.Errorf("expected to be false")
		}
	})

	t.Run("should return false if service port are not equal", func(t *testing.T) {
		serviceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5433,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		anotherServiceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		if serviceInfo.Compare(anotherServiceInfo) {
			t.Errorf("expected to be false")
		}
	})

	t.Run("should return false if Ids are not equal", func(t *testing.T) {
		serviceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "slave",
			Tags:    Tags{"master"},
		}
		anotherServiceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		if serviceInfo.Compare(anotherServiceInfo) {
			t.Errorf("expected to be false")
		}
	})

	t.Run("should return false if tags are not equal", func(t *testing.T) {
		serviceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master", "sdf"},
		}
		anotherServiceInfo := ServiceInfo{
			Name:    "test",
			Address: "127.0.0.1",
			Port:    5432,
			ID:      "master",
			Tags:    Tags{"master"},
		}
		if serviceInfo.Compare(anotherServiceInfo) {
			t.Errorf("expected to be false")
		}
	})
}

func TestServiceInfosDiff(t *testing.T) {
	t.Run("should not add or remove when discovered and existing services are empty", func(t *testing.T) {
		discoveredServiceInfos := ServiceInfos{}
		existingServiceInfos := ServiceInfos{}

		diff := existingServiceInfos.Diff(discoveredServiceInfos)

		if len(diff.Added) != 0 {
			t.Errorf("expected no service to be added but %d service got added", len(diff.Added))
		}
		if len(diff.Removed) != 0 {
			t.Errorf("expected no service to be removed but %d service got removed", len(diff.Removed))
		}

	})

	t.Run("should only add when new services are found", func(t *testing.T) {
		discoveredServiceInfos := ServiceInfos{}
		existingServiceInfos := ServiceInfos{"masterUID": ServiceInfo{}, "slaveUID": ServiceInfo{}}

		diff := existingServiceInfos.Diff(discoveredServiceInfos)

		if len(diff.Added) != 2 {
			t.Errorf("expected 2 service to be added but %d service got added", len(diff.Added))
		}
		if len(diff.Removed) != 0 {
			t.Errorf("expected no service to be removed but %d service got added", len(diff.Removed))
		}

		for _, id := range []string{"masterUID", "slaveUID"} {
			_, ok := diff.Added[id]
			if !ok {
				t.Errorf("expected %s to be added but not", id)
			}
		}

	})

	t.Run("should only remove when discovered services no longer exists", func(t *testing.T) {
		discoveredServiceInfos := ServiceInfos{"masterUID": ServiceInfo{ID: "masterUID"}, "slaveUID": ServiceInfo{ID: "slaveUID"}}
		existingServiceInfos := ServiceInfos{}

		diff := existingServiceInfos.Diff(discoveredServiceInfos)

		if len(diff.Added) != 0 {
			t.Errorf("expected no service to be added but %d service got added", len(diff.Added))
		}
		if len(diff.Removed) != 2 {
			t.Errorf("expected 2 service to be removed but %d service got added", len(diff.Removed))
		}

		for _, id := range []string{"masterUID", "slaveUID"} {
			_, ok := diff.Removed[id]
			if !ok {
				t.Errorf("expected %s to be removed but not", id)
			}
		}

	})

	t.Run("should not add or remove when discovered and existing services are same", func(t *testing.T) {
		discoveredServiceInfos := ServiceInfos{"masterUID": ServiceInfo{}, "slaveUID": ServiceInfo{}}
		existingServiceInfos := ServiceInfos{"masterUID": ServiceInfo{}, "slaveUID": ServiceInfo{}}

		diff := existingServiceInfos.Diff(discoveredServiceInfos)

		if len(diff.Added) != 0 {
			t.Errorf("expected no service to be added but %d service got added", len(diff.Added))
		}
		if len(diff.Removed) != 0 {
			t.Errorf("expected no service to be removed but %d service got removed", len(diff.Removed))
		}

	})

	t.Run("should add and remove corresponding service infos", func(t *testing.T) {
		discoveredServiceInfos := ServiceInfos{"masterUID": ServiceInfo{ID: "masterUID"}, "slaveUID": ServiceInfo{ID: "slaveUID"}}
		existingServiceInfos := ServiceInfos{"newSlaveUID": ServiceInfo{ID: "newSlaveUID"}, "slaveUID": ServiceInfo{ID: "slaveUID"}}

		diff := existingServiceInfos.Diff(discoveredServiceInfos)

		if len(diff.Added) != 1 {
			t.Errorf("expected 1 service to be added but %d service got added", len(diff.Added))
		}
		if len(diff.Removed) != 1 {
			t.Errorf("expected 1 service to be removed but %d service got added", len(diff.Removed))
		}

		for _, id := range []string{"masterUID"} {
			_, ok := diff.Removed[id]
			if !ok {
				t.Errorf("expected %s to be removed but not", id)
			}
		}

		for _, id := range []string{"newSlaveUID"} {
			_, ok := diff.Added[id]
			if !ok {
				t.Errorf("expected %s to be added but not", id)
			}
		}
	})

	t.Run("should add and remove corresponding service infos", func(t *testing.T) {
		discoveredServiceInfos := ServiceInfos{"masterUID": ServiceInfo{ID: "masterUID", Tags: Tags{"master"}}, "slaveUID": ServiceInfo{ID: "slaveUID"},
			"anotherSlaveUID": ServiceInfo{ID: "anotherSlaveUID"}}
		existingServiceInfos := ServiceInfos{"masterUID": ServiceInfo{ID: "masterUID", Tags: Tags{"slave"}}, "slaveUID": ServiceInfo{ID: "slaveUID"},
			"anotherSlaveUID": ServiceInfo{ID: "anotherSlaveUID", Tags: Tags{"master"}}}

		diff := existingServiceInfos.Diff(discoveredServiceInfos)

		if len(diff.Added) != 2 {
			t.Errorf("expected 2 service to be added but %d service got added", len(diff.Added))
		}
		if len(diff.Removed) != 2 {
			t.Errorf("expected 2 service to be removed but %d service got added", len(diff.Removed))
		}

		for _, id := range []string{"masterUID", "anotherSlaveUID"} {
			_, ok := diff.Removed[id]
			if !ok {
				t.Errorf("expected %s to be removed but not", id)
			}
		}

		for _, id := range []string{"masterUID", "anotherSlaveUID"} {
			_, ok := diff.Added[id]
			if !ok {
				t.Errorf("expected %s to be added but not", id)
			}
		}

	})
}

func TestNewTags(t *testing.T) {
	t.Run("should split comma separated string into tags", func(t *testing.T) {
		from := "one,two"

		actual := NewTags(from)

		if len(actual) != 2 {
			t.Errorf("expected 2 tags but got %d", len(actual))
		}
		if actual[0] != "one" {
			t.Errorf("expected to equal to one but was %s", actual[0])
		}
		if actual[1] != "two" {
			t.Errorf("expected to equal to one but was %s", actual[1])
		}
	})

	t.Run("should not error if strings are not comma separated", func(t *testing.T) {
		from := "one;two"

		actual := NewTags(from)

		if len(actual) != 1 {
			t.Errorf("expected 1 tags but got %d", len(actual))
		}
	})

	t.Run("should not error if string is empty", func(t *testing.T) {
		from := ""

		actual := NewTags(from)

		if len(actual) != 1 {
			t.Errorf("expected 1 tags but got %d", len(actual))
		}
	})
}
