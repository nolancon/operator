package version

import (
	"strings"

	"github.com/blang/semver"
	ctrl "sigs.k8s.io/controller-runtime"
)

var log = ctrl.Log.WithName("version")

// IsSupported takes two versions, current version (haveVersion) and a
// minimum requirement version (wantVersion) and checks if the current version
// is supported by comparing it with the minimum requirement.
func IsSupported(haveVersion, wantVersion string) bool {
	haveVersion = strings.Trim(haveVersion, "v")
	wantVersion = strings.Trim(wantVersion, "v")

	supportedVersion, err := semver.Parse(wantVersion)
	if err != nil {
		log.Info("Failed to parse version", "error", err, "want", wantVersion)
		return false
	}

	currentVersion, err := semver.Parse(haveVersion)
	if err != nil {
		log.Info("Failed to parse version", "error", err, "have", haveVersion)
		return false
	}

	return currentVersion.Compare(supportedVersion) >= 0
}
