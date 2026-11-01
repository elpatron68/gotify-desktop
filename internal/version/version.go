// Package version holds the app release version shown in About and builds.
// Keep Version in sync with build/config.yml info.version and wails.json (CI: packaging/set-version.sh).
package version

// Version is the product version without a leading "v".
const Version = "0.2.1"

// Author is shown in the About menu.
const Author = "Markus F.J. Busche"

// License is the SPDX short identifier for the project license.
const License = "MIT"

// RepositoryURL is the public source repository.
const RepositoryURL = "https://github.com/elpatron68/gotify-desktop"

// LicenseURL points at the LICENSE file in the repository.
const LicenseURL = RepositoryURL + "/blob/main/LICENSE"

// ReleasesURL points at the latest GitHub release.
const ReleasesURL = RepositoryURL + "/releases/latest"
