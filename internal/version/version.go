package version

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"
	ctrl "sigs.k8s.io/controller-runtime"
)

var log = ctrl.Log.WithName("version")

var versionRegexp *regexp.Regexp

func init() {
	var err error
	versionRegexp, err = regexp.Compile("v?([0-9]+.[0-9]+.[0-9]+)")
	if err != nil {
		panic(err)
	}
}

func CleanupVersion(version string) string {
	return versionRegexp.FindString(version)
}

// IsSupported takes two versions, current version (haveVersion) and a
// minimum requirement version (wantVersion) and checks if the current version
// is supported by comparing it with the minimum requirement.
func IsSupported(haveVersion, wantVersion string) bool {
	haveVersion = strings.Trim(versionRegexp.FindString(haveVersion), "v")
	wantVersion = strings.Trim(versionRegexp.FindString(wantVersion), "v")

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

// MajorMinor takes a version (haveversion) and converts it to a string of just version
// major and minor. Eg v1.5.8-alpha.3 = "1.5"
func MajorMinor(haveVersion string) string {
	haveVersion = strings.Trim(versionRegexp.FindString(haveVersion), "v")

	parsedVersion, err := semver.Parse(haveVersion)
	if err != nil {
		log.Info("Failed to parse version", "error", err)
		return ""
	}

	return strconv.FormatUint(parsedVersion.Major, 10) + "." + strconv.FormatUint(parsedVersion.Minor, 10)
}
