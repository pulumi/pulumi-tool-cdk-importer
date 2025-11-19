//go:build darwin && arm64

package main

import _ "embed"

//go:embed deps/darwin-arm64/cdk2pulumi
var cdk2pulumiBinary []byte
