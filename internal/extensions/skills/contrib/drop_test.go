// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// drop_test.go — a file that publishes nothing has to SAY so.
//
// Skipping is the right runtime behaviour and stays. What changes is that the
// fact now travels, because packaging is not watching any log and otherwise
// ships a pack that is simply short of the content it was built to carry.

func providerOver(files fstest.MapFS) *manifestcontrib.EmbedFSResourceProvider {
	return SkillProvider("ktw-test", loader.NewEmbedFSLoader(files, ""), "skills")
}

func TestAMalformedSkillCostsOnlyItself(t *testing.T) {
	p := providerOver(fstest.MapFS{
		"skills/good.md":   {Data: []byte("---\nname: good\ndescription: A good skill.\n---\n\nBody.\n")},
		"skills/broken.md": {Data: []byte("---\ndescription: a skill that names no name\n---\n\nBody.\n")},
	})

	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("one malformed file failed the whole enumeration: %v", err)
	}
	// Runtime resilience, unchanged: the good skill is still published.
	if len(entries) != 1 || entries[0].ID != "good" {
		t.Fatalf("published %v, want just the good skill", entries)
	}

	// And the bad one is now accounted for, naming the file and the reason.
	drops := p.Drops()
	if len(drops) != 1 {
		t.Fatalf("drops = %v, want the broken skill", drops)
	}
	if drops[0].Path != "skills/broken.md" {
		t.Errorf("drop names %q, want skills/broken.md", drops[0].Path)
	}
	if drops[0].Kind != "skill" {
		t.Errorf("drop kind = %q, want skill", drops[0].Kind)
	}
	if drops[0].Reason == "" {
		t.Error("drop carries no reason; the producer's account is the only one that will ever exist")
	}
}

// Drops describe the enumeration that just happened, not every enumeration
// since the process started — otherwise a long-running host would accumulate a
// history nobody asked for, and a second probe would report a problem twice.
func TestDropsDescribeTheLatestEnumeration(t *testing.T) {
	p := providerOver(fstest.MapFS{
		"skills/broken.md": {Data: []byte("---\ndescription: a skill that names no name\n---\n\nBody.\n")},
	})
	for i := 0; i < 3; i++ {
		if _, err := p.Enumerate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(p.Drops()); got != 1 {
		t.Errorf("after 3 enumerations, drops = %d, want 1", got)
	}
}

// A folder with nothing wrong reports nothing. The absence of drops is what
// makes their presence mean something.
func TestACleanFolderDropsNothing(t *testing.T) {
	p := providerOver(fstest.MapFS{
		"skills/good.md": {Data: []byte("---\nname: good\ndescription: A good skill.\n---\n\nBody.\n")},
	})
	if _, err := p.Enumerate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if drops := p.Drops(); len(drops) != 0 {
		t.Errorf("a clean folder reported %v", drops)
	}
}

func TestProviderReportsDrops(t *testing.T) {
	var p any = providerOver(fstest.MapFS{})
	if _, ok := p.(contribapi.DropReporter); !ok {
		t.Error("the skill provider no longer satisfies DropReporter")
	}
}
