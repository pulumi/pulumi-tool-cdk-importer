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
