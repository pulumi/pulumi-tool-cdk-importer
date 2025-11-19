//go:build linux && amd64

package main

import _ "embed"

//go:embed deps/linux-amd64/cdk2pulumi
var cdk2pulumiBinary []byte
