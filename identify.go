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
	_ "embed"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// Given a set of Pulumi resource properties, try to recover the primary CF resource ID. This does not work if the
// necessary data is not present in the property map, in which case the function returns false.
func RecoverPrimaryResourceID(resTok tokens.Type, data resource.PropertyMap) (PrimaryResourceID, bool) {
	pi, ok := awsNativeMetadata.PrimaryIdentifier(resTok)
	if !ok {
		return "", false
	}
	// TODO: Should this use naming.CdkToCfn instead?
	// See "github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	components := []string{}
	for _, prop := range pi {
		dp, ok := data[prop]
		if !ok || !dp.IsString() {
			return "", false
		}
		components = append(components, dp.StringValue())
	}

	return PrimaryResourceID(strings.Join(components, "|")), true
}
