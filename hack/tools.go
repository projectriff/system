// +build tools

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
package tools

import (
	_ "github.com/google/ko/pkg"
	_ "github.com/vektra/mockery"
	_ "golang.org/x/tools/imports"
	_ "k8s.io/code-generator/pkg/util"
	_ "sigs.k8s.io/controller-tools/pkg/version"
)
