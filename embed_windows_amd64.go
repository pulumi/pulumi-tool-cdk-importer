//go:build windows && amd64

package main

import _ "embed"

//go:embed deps/windows-amd64/cdk2pulumi
var cdk2pulumiBinary []byte
