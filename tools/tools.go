//go:build tools
// +build tools

package tools

import (
	_ "github.com/openshift/api/config/v1/zz_generated.crd-manifests"
	_ "github.com/openshift/api/machine/v1beta1/zz_generated.crd-manifests"
	_ "go.uber.org/mock/mockgen"
	_ "sigs.k8s.io/controller-runtime/tools/setup-envtest"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
