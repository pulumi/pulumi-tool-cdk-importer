package main

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/pulumi/pulumi-aws-native/provider/pkg/metadata"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

// Given a set of Pulumi resource properties, try to recover the primary CF resource ID. This does not work if the
// necessary data is not present in the property map, in which case the function returns false.
func RecoverPrimaryResourceID(resTok tokens.Token, data resource.PropertyMap) (CFPrimaryResourceID, bool) {
	r, ok := awsNativeMetadata.Resources[string(resTok)]
	if !ok {
		return "", false
	}
	// TODO: Should this use naming.CdkToCfn instead?
	// See "github.com/pulumi/pulumi-aws-native/provider/pkg/naming"
	components := []string{}
	for _, rawProp := range r.PrimaryIdentifier {
		prop := resource.PropertyKey(rawProp)
		dp, ok := data[prop]
		if !ok || !dp.IsString() {
			return "", false
		}
		components = append(components, dp.StringValue())
	}

	return CFPrimaryResourceID(strings.Join(components, "|")), true
}

//go:embed pulumi-aws-native-metadata.json
var awsNativeMetadataBytes []byte

var awsNativeMetadata *metadata.CloudAPIMetadata

func init() {
	if err := json.Unmarshal(awsNativeMetadataBytes, &awsNativeMetadata); err != nil {
		panic(err)
	}
}
