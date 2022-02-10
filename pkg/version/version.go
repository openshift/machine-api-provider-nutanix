package version

import (
	"fmt"

	"github.com/blang/semver"
)

var (
	// Version is semver representation of the version.
	Version = semver.MustParse("1.0.1")

	// String is the human-friendly representation of the version.
	String = fmt.Sprintf("ClusterAPIProviderNutanix %s", Version.String())
)
