//go:build linux && arm64

package main

import _ "embed"

//go:embed deps/linux-arm64/cdk2pulumi
var cdk2pulumiBinary []byte
