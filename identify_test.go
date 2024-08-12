package main

import (
	"fmt"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
)

func TestRecoverPrimaryResourceID(t *testing.T) {
	testCases := []struct {
		resource  tokens.Token
		data      resource.PropertyMap
		primaryID CFPrimaryResourceID
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
