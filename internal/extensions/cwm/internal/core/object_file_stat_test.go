// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectFileStat describes a file of the object's module — a named file once
// written — and answers not found for one that is not there.
func TestObjectFileStatDescribesAnExistingFileAndRefusesAMissingOne(t *testing.T) {
	svc, _ := namedFileFixture(t)
	ctx := stdctx.Background()

	// Declared but never written: not there yet.
	if _, err := svc.ObjectFileStat(ctx, api.ObjectFileStatInput{ID: "billing.spec", Path: "memory/billing.spec.md"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("an unwritten named file: %v, want ErrNotFound", err)
	}

	w, err := svc.ObjectNamedFileWrite(ctx, api.ObjectNamedFileWriteInput{ID: "billing.spec", Capability: "memory", Name: "notes", Content: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}
	st, err := svc.ObjectFileStat(ctx, api.ObjectFileStatInput{ID: "billing.spec", Path: w.File.Path})
	if err != nil {
		t.Fatalf("stat %s: %v", w.File.Path, err)
	}
	if st.Entry.Path != "memory/billing.spec.md" || st.Entry.IsDir || st.Entry.SizeBytes != 5 || st.Entry.ModTime.IsZero() {
		t.Errorf("entry = %+v, want memory/billing.spec.md, a 5-byte file", st.Entry)
	}
	if st.ProjectPath == "" || st.ProjectPath != w.ProjectPath {
		t.Errorf("project path %q, want the write's %q", st.ProjectPath, w.ProjectPath)
	}

	// The folder that holds it is a directory entry.
	dir, err := svc.ObjectFileStat(ctx, api.ObjectFileStatInput{ID: "billing.spec", Path: "memory"})
	if err != nil || !dir.Entry.IsDir {
		t.Errorf("the folder: %+v %v, want a directory", dir.Entry, err)
	}

	for _, missing := range []string{"memory/nope.md", "nowhere/x.md"} {
		if _, err := svc.ObjectFileStat(ctx, api.ObjectFileStatInput{ID: "billing.spec", Path: missing}); !errors.Is(err, api.ErrNotFound) {
			t.Errorf("%s: %v, want ErrNotFound", missing, err)
		}
	}
}
