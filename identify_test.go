// Copyright 2016-2024, Pulumi Corporation.
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

package main

import (
	"fmt"
	"testing"

	"github.com/pulumi/pulumi-tool-cdk-importer/internal/common"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

func TestRecoverPrimaryResourceID(t *testing.T) {
	testCases := []struct {
		resource  tokens.Type
		data      resource.PropertyMap
		primaryID common.PrimaryResourceID
	}{
		{
			resource: "aws-native:s3:Bucket",
			data: resource.PropertyMap{
				"bucketName": resource.NewStringProperty("my-bucket"),
			},
			primaryID: "my-bucket",
		},
		{
			resource: "aws-native:cassandra:Table",
			data: resource.PropertyMap{
				"keyspaceName": resource.NewStringProperty("my-keyspace"),
				"tableName":    resource.NewStringProperty("my-table"),
			},
			primaryID: "my-keyspace|my-table",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%s-%d", tc.resource, i), func(t *testing.T) {
			id, ok := RecoverPrimaryResourceID(tc.resource, tc.data)
			assert.True(t, ok)
			assert.Equal(t, tc.primaryID, id)
		})
	}
}
