// +build tools

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
package tools

import (
	// TODO move back into /hack once ko uses k8s 1.18 apis
	_ "github.com/google/ko/pkg"
)
