// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strings"
	"testing"
)

func TestDestructiveWarning_GitResetHard(t *testing.T) {
	w := GetDestructiveWarning("git reset --hard HEAD~1")
	if !strings.Contains(w, "discard uncommitted") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_GitPushForce(t *testing.T) {
	w := GetDestructiveWarning("git push --force origin main")
	if !strings.Contains(w, "overwrite remote") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_GitPushForceShortFlag(t *testing.T) {
	w := GetDestructiveWarning("git push -f origin main")
	if !strings.Contains(w, "overwrite remote") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_GitClean(t *testing.T) {
	w := GetDestructiveWarning("git clean -fd")
	if !strings.Contains(w, "delete untracked") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_GitCleanDryRun_NoWarning(t *testing.T) {
	w := GetDestructiveWarning("git clean -nd")
	if w != "" {
		t.Errorf("dry-run should not warn, got %q", w)
	}
}

func TestDestructiveWarning_GitCheckoutDot(t *testing.T) {
	w := GetDestructiveWarning("git checkout .")
	if !strings.Contains(w, "discard all working tree") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_GitCommitAmend(t *testing.T) {
	w := GetDestructiveWarning("git commit --amend -m 'fix'")
	if !strings.Contains(w, "rewrite the last commit") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_GitNoVerify(t *testing.T) {
	w := GetDestructiveWarning("git commit --no-verify -m 'skip hooks'")
	if !strings.Contains(w, "skip safety hooks") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_RmRf(t *testing.T) {
	w := GetDestructiveWarning("rm -rf /tmp/test")
	if !strings.Contains(w, "recursively force-remove") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_RmRecursive(t *testing.T) {
	w := GetDestructiveWarning("rm -r /tmp/test")
	if !strings.Contains(w, "recursively remove") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_DropTable(t *testing.T) {
	w := GetDestructiveWarning("psql -c 'DROP TABLE users;'")
	if !strings.Contains(w, "drop or truncate") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_KubectlDelete(t *testing.T) {
	w := GetDestructiveWarning("kubectl delete pod my-pod")
	if !strings.Contains(w, "Kubernetes") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_TerraformDestroy(t *testing.T) {
	w := GetDestructiveWarning("terraform destroy -auto-approve")
	if !strings.Contains(w, "Terraform") {
		t.Errorf("warning = %q", w)
	}
}

func TestDestructiveWarning_SafeCommand_NoWarning(t *testing.T) {
	safe := []string{
		"echo hello",
		"ls -la",
		"git status",
		"git log --oneline",
		"cat file.txt",
		"grep pattern file",
	}
	for _, cmd := range safe {
		w := GetDestructiveWarning(cmd)
		if w != "" {
			t.Errorf("GetDestructiveWarning(%q) = %q, want empty", cmd, w)
		}
	}
}
