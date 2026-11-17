// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : the export scans the files it is about to commit in process
// before any object is written: a credential planted in a to-do's text
// refuses the export - the ref stays where it was and no blob holding the key
// reaches the object store - and the finding names the to-do's file and the
// kind, never the value. The key is assembled at run time, so this file holds
// none.
func TestAnExportHoldingASecretIsRefusedAndTheRefStays(t *testing.T) {
	ctx := context.Background()
	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	svc := NewOver(st)
	out, err := svc.Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	tip := out.Refs[0].Commit

	key := "AK" + "IA" + strings.Repeat("ABCD", 4)
	planted, err := st.Create(ctx, api.Todo{Module: "billing", Title: "rotate", Note: "use " + key + " for the run", State: api.StateReady, For: api.ForAgent})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Export(ctx, api.ExportInput{})
	if !errors.Is(err, ErrSecretInExport) {
		t.Fatalf("export with a key in a to-do's note = %v, want ErrSecretInExport", err)
	}
	if !strings.Contains(err.Error(), planted.ID+".json") || !strings.Contains(err.Error(), "aws-key") || strings.Contains(err.Error(), key) {
		t.Fatalf("refusal %q: want the to-do's file and the kind, never the key", err)
	}
	if now, _ := refTip(repoDir, ExportRef("repo-1")); now != tip {
		t.Fatalf("the ref moved to %s on a refused export, want %s", now, tip)
	}
	all, err := gitcli.Run(repoDir, "cat-file", "--batch-all-objects", "--batch")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(all, key) {
		t.Fatal("a refused export wrote a blob holding the key")
	}

}
