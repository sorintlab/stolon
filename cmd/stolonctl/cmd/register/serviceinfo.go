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
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/sorintlab/stolon/internal/cluster"
)

// HealthCheck holds necessary information for performing
// health check on the service
type HealthCheck struct {
	TCP      string
	Interval string
}

// Tags represents various way a service can be tagged
type Tags []string

// NewTags convert comma separated string into Tags
func NewTags(from string) Tags {
	return Tags(strings.Split(from, ","))
}

// Compare if two tags are equal
func (t Tags) Compare(tags Tags) bool {
	return reflect.DeepEqual(t, tags)
}

// ServiceInfo holds the necessary information about a service
// for service discovery
type ServiceInfo struct {
	Name     string
	Tags     Tags
	ID       string
	Address  string
	IsMaster bool
	Port     int
	Check    HealthCheck
}

// ConsulAgentServiceRegistration returns AgentServiceRegistration
func (info *ServiceInfo) ConsulAgentServiceRegistration() *api.AgentServiceRegistration {
	check := api.AgentServiceCheck{
		TCP:      info.Check.TCP,
		Interval: info.Check.Interval,
	}
	service := api.AgentServiceRegistration{
		ID:      info.ID,
		Name:    info.Name,
		Address: info.Address,
		Tags:    info.Tags,
		Port:    info.Port,
		Check:   &check,
	}
	return &service
}

// Compare if two ServiceInfo are equal
func (info *ServiceInfo) Compare(target ServiceInfo) bool {
	return info.Name == target.Name &&
		info.ID == target.ID &&
		info.Address == target.Address &&
		info.Port == target.Port &&
		info.Tags.Compare(target.Tags)
}

// NewServiceInfo return new ServiceInfo from name, db and tags
func NewServiceInfo(name string, db *cluster.DB, tags []string, isMaster bool) (*ServiceInfo, error) {
	port, err := strconv.Atoi(db.Status.Port)
	if err != nil {
		return nil, fmt.Errorf(fmt.Sprintf("invalid database port '%s' for %s with uid %s", db.Status.Port, name, db.UID))
	}
	return &ServiceInfo{
		Name:     name,
		Tags:     tags,
		Address:  db.Status.ListenAddress,
		ID:       db.UID,
		Port:     port,
		IsMaster: isMaster,
		Check: HealthCheck{
			TCP:      net.JoinHostPort(db.Status.ListenAddress, db.Status.Port),
			Interval: "10s",
		},
	}, nil
}

// NewServiceInfoFromConsulService returns ServiceInfo from consul service
func NewServiceInfoFromConsulService(service api.CatalogService) ServiceInfo {
	return ServiceInfo{
		Name:    service.ServiceName,
		Tags:    service.ServiceTags,
		ID:      service.ServiceID,
		Port:    service.ServicePort,
		Address: service.ServiceAddress,
	}
}

// ServiceInfos represents holds collection of ServiceInfo
type ServiceInfos map[string]ServiceInfo

// ServiceInfoStatus holds the services which are newly added / removed
type ServiceInfoStatus struct {
	Added   ServiceInfos
	Removed ServiceInfos
}

// Diff returns ServiceInfoStatus
func (existing ServiceInfos) Diff(discovered ServiceInfos) ServiceInfoStatus {
	result := ServiceInfoStatus{Added: ServiceInfos{}, Removed: ServiceInfos{}}
	for id, existingInfo := range existing {
		if discoveredInfo, ok := discovered[id]; !ok {
			result.Added[id] = existingInfo
		} else {
			if !existingInfo.Compare(discoveredInfo) {
				result.Added[id] = existingInfo
				result.Removed[id] = discoveredInfo
			}
		}
	}
	for id, discoveredInfo := range discovered {
		if existingInfo, ok := existing[id]; !ok {
			result.Removed[id] = discoveredInfo
		} else {
			if !discoveredInfo.Compare(existingInfo) {
				result.Added[id] = existingInfo
				result.Removed[id] = discoveredInfo
			}
		}
	}
	return result
}
