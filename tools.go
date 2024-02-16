//go:build tools
// +build tools

// Official workaround to track tool dependencies with go modules:
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module

package tools

import (
	_ "github.com/golang/mock/mockgen"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
