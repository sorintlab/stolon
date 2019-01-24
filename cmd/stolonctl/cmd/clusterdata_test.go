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

package cmd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sorintlab/stolon/internal/cluster"
	"github.com/sorintlab/stolon/internal/mock/store"
)

func TestWriteClusterdata(t *testing.T) {
	writeClusterdataOpts.file = ""
	writeClusterdataOpts.forceYes = false

	t.Run("should handle error returned by stdin", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		store := mock_store.NewMockStore(ctrl)
		reader := strings.Reader{}
		err := writeClusterdata(&reader, store)

		if err == nil {
			t.Error("expected to have an error")
		}

		expectedErrorMessage := "invalid cluster data: unexpected end of JSON input"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected %s error message but instead got %s", expectedErrorMessage, err.Error())
		}
	})

	t.Run("should handle json unmarshal error", func(t *testing.T) {
		fileName := "cluster_data.json"
		writeClusterdataOpts.file = fileName
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		store := mock_store.NewMockStore(ctrl)
		reader := strings.NewReader("{a}")
		err := writeClusterdata(reader, store)

		if err == nil {
			t.Error("expected to have an error")
		}

		expectedErrorMessage := "invalid cluster data: invalid character 'a' looking for beginning of object key string"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected %s error message but instead got %s", expectedErrorMessage, err.Error())
		}
	})

	t.Run("should throw an error if the new store is not valid", func(t *testing.T) {
		fileName := "-"
		writeClusterdataOpts.file = fileName
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		reader := strings.NewReader("{}")
		store := mock_store.NewMockStore(ctrl)

		store.EXPECT().GetClusterData(gomock.Any()).Return(nil, nil, fmt.Errorf("Error in getting cluster data"))

		err := writeClusterdata(reader, store)

		if err == nil {
			t.Error("expected to have an error")
		}

		expectedErrorMessage := "Error in getting cluster data"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected %s error message but instead got %s", expectedErrorMessage, err.Error())
		}
	})

	t.Run("should throw an error if the there is an error while uploading the cluster data", func(t *testing.T) {
		fileName := "-"
		writeClusterdataOpts.file = fileName
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		reader := strings.NewReader("{}")
		store := mock_store.NewMockStore(ctrl)
		store.EXPECT().GetClusterData(gomock.Any()).Return(&cluster.ClusterData{}, nil, nil)

		err := writeClusterdata(reader, store)

		if err == nil {
			t.Error("expected to have an error")
		}

		expectedErrorMessage := "WARNING: cluster data already available use --yes to override"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected %s error message but instead got %s", expectedErrorMessage, err.Error())
		}
	})

	t.Run("should throw an error if the there is an error while uploading the cluster data", func(t *testing.T) {
		fileName := "-"
		writeClusterdataOpts.file = fileName
		writeClusterdataOpts.forceYes = true
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		reader := strings.NewReader("{}")
		store := mock_store.NewMockStore(ctrl)
		cd := &cluster.ClusterData{}
		store.EXPECT().GetClusterData(gomock.Any()).Return(cd, nil, nil)
		store.EXPECT().PutClusterData(gomock.Any(), cd).Return(fmt.Errorf("error while uploading the cluster data"))

		err := writeClusterdata(reader, store)

		if err == nil {
			t.Error("expected to have an error")
		}

		expectedErrorMessage := "failed to write cluster data into new store error while uploading the cluster data"
		if err.Error() != expectedErrorMessage {
			t.Errorf("expected %s error message but instead got %s", expectedErrorMessage, err.Error())
		}
	})

	t.Run("should successfully upload the cluster data", func(t *testing.T) {
		fileName := "-"
		writeClusterdataOpts.file = fileName
		writeClusterdataOpts.forceYes = true
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		reader := strings.NewReader("{}")
		store := mock_store.NewMockStore(ctrl)
		cd := &cluster.ClusterData{}
		store.EXPECT().GetClusterData(gomock.Any()).Return(cd, nil, nil)
		store.EXPECT().PutClusterData(gomock.Any(), cd).Return(nil)

		err := writeClusterdata(reader, store)

		if err != nil {
			t.Error("expected not to have an error")
		}
	})

}
