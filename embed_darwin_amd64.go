//go:build darwin && amd64

package main

import _ "embed"

//go:embed deps/darwin-amd64/cdk2pulumi
var cdk2pulumiBinary []byte
