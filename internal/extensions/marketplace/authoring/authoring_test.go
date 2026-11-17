// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/memberversion"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// TestDeriveMemberVersion pins a member to the version its store path resolves
// to — content via --folder, sidecar via the binary's parent dir — and returns
// "" for a non-store (dev/PATH) binary.
func TestDeriveMemberVersion(t *testing.T) {
	root := t.TempDir() // absolute on the host OS

	content := orchapi.SidecarSpec{
		Name:   "acme-flows",
		Binary: "ktw-host",
		Args:   []string{"--folder", filepath.Join(root, "bin", "store-content", "acme-flows", "1.0.0"), "sc", "ktsp", "stdio"},
	}
	if got := memberversion.Derive(content); got != "1.0.0" {
		t.Errorf("content version = %q, want 1.0.0", got)
	}

	sidecar := orchapi.SidecarSpec{
		Name:   "ktw-example",
		Binary: filepath.Join(root, "bin", "store", "ktw-example", "1.4.2", "ktw-example.exe"),
		Args:   []string{"sc", "ktsp", "stdio"},
	}
	if got := memberversion.Derive(sidecar); got != "1.4.2" {
		t.Errorf("sidecar version = %q, want 1.4.2", got)
	}

	dev := orchapi.SidecarSpec{Name: "ktw-dev", Binary: "ktw-dev", Args: []string{"sc", "ktsp", "stdio"}}
	if got := memberversion.Derive(dev); got != "" {
		t.Errorf("dev/PATH binary version = %q, want empty (unpinned)", got)
	}
}

// TestKeygenSignVerify: a keypair from Keygen signs a catalog whose signature
// verifies against the produced public key (the whole publisher→consumer trust
// handshake, in-process).
func TestKeygenSignVerify(t *testing.T) {
	dir := t.TempDir()
	svc := New()

	pubFile, secFile, pubLine, err := svc.Keygen("test key")
	if err != nil {
		t.Fatal(err)
	}
	secPath := filepath.Join(dir, "team.key")
	if err := os.WriteFile(secPath, []byte(secFile), 0o600); err != nil {
		t.Fatal(err)
	}

	catalogPath := filepath.Join(dir, "katyoti-marketplace.yaml")
	index := []byte("apiVersion: 1\nmarketplace: {name: team}\nitems: []\n")
	if err := os.WriteFile(catalogPath, index, 0o644); err != nil {
		t.Fatal(err)
	}

	sigPath, err := svc.Sign(catalogPath, secPath, "v1")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if sigPath != catalogPath+".minisig" {
		t.Fatalf("sig path = %q", sigPath)
	}

	// The consumer's pinned pubkey (either the file or the bare line) verifies it.
	pk, err := signing.ParsePublicKey([]byte(pubFile))
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := os.ReadFile(sigPath)
	if err := signing.Verify(index, sig, pk); err != nil {
		t.Fatalf("verify with pubkey file: %v", err)
	}
	pkLine, err := signing.ParsePublicKey([]byte(pubLine))
	if err != nil {
		t.Fatal(err)
	}
	if err := signing.Verify(index, sig, pkLine); err != nil {
		t.Fatalf("verify with pubkey line: %v", err)
	}
}
