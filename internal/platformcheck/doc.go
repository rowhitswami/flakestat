// Package platformcheck holds a deliberately platform-dependent test used to
// validate the CI observation pipeline end to end.
//
// It is excluded from the normal test suite by a build tag and must stay that
// way. Its whole purpose is to fail on one platform, which would otherwise
// make this repository's own CI untrustworthy and pollute the very history
// flakestat is accumulating about itself.
//
// Run it deliberately:
//
//	go test -tags ciplatform ./internal/platformcheck/
package platformcheck
