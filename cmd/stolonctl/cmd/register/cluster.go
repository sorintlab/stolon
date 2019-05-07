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
	"context"
	"errors"

	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/store"
)

// Cluster type exposes necessary methods to find master and slave
// from underlying store
type Cluster struct {
	name        string
	cd          *cluster.ClusterData
	tagMasterAs Tags
	tagSlaveAs  Tags
}

// NewCluster returns an new instance of Cluster
func NewCluster(name string, rCfg Config, store store.Store) (*Cluster, error) {
	cd, _, err := store.GetClusterData(context.TODO())

	if err != nil {
		return nil, err
	} else if cd == nil {
		return nil, errors.New("no cluster data available")
	}
	return &Cluster{name: name, cd: cd, tagMasterAs: NewTags(rCfg.TagMasterAs), tagSlaveAs: NewTags(rCfg.TagSlaveAs)}, nil
}

// ServiceInfos returns all the service information from the cluster data in underlying store
func (c *Cluster) ServiceInfos() (ServiceInfos, error) {
	if c.cd.Cluster == nil {
		return nil, errors.New("cluster data not available")
	}

	serviceInfos := ServiceInfos{}
	master := c.cd.Cluster.Status.Master
	for uid, db := range c.cd.DBs {
		if db.Status.Healthy {
			tags := c.tagSlaveAs
			isMaster := false
			if uid == master {
				tags = c.tagMasterAs
				isMaster = true
			}
			info, err := NewServiceInfo(c.name, db, tags, isMaster)
			if err != nil {
				return nil, err
			}
			serviceInfos[uid] = *info
		}
	}

	return serviceInfos, nil
}
