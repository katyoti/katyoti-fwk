// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"strings"
	"testing"
)

// : appcore.SidecarExt registers the system contributions and the
// services producer; a bootstrap that uses it and names either of them again
// enumerates every entry twice, merged at every manifest build.
func TestSidecarExtProducersAreRegisteredOnce(t *testing.T) {
	src, err := os.ReadFile("bootstrap.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	if !strings.Contains(code, "appcore.SidecarExt(") {
		t.Skip("bootstrap.go no longer uses appcore.SidecarExt: nothing it registers can be named twice")
	}
	for _, again := range []string{"publish.ServicesContributionsExtension(", "contribution.SystemContributionsExtension("} {
		if strings.Contains(code, again) {
			t.Errorf("bootstrap.go names %s although appcore.SidecarExt already registers it", strings.TrimSuffix(again, "("))
		}
	}
}
