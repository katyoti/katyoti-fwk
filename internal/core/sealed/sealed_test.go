// SPDX-License-Identifier: Apache-2.0

package sealed

import (
	"errors"
	"strings"
	"testing"
)

const pw = "correct horse battery"

// What is sealed opens with its password, and only with it.
func TestSealedOpensWithItsPasswordOnly(t *testing.T) {
	file, err := Seal([]byte("LEANIX_API_TOKEN=tok"), pw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(file), "tok") {
		t.Fatal("the secret is readable in the sealed file")
	}
	got, err := Open(file, pw)
	if err != nil || string(got) != "LEANIX_API_TOKEN=tok" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := Open(file, pw+"!"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("a wrong password = %v, want ErrWrongPassword", err)
	}
	again, _ := Seal([]byte("LEANIX_API_TOKEN=tok"), pw)
	if string(again) == string(file) {
		t.Error("sealing twice gives the same bytes: the salt or nonce is not random")
	}
}

// A change to the data or to any header parameter fails like a wrong password
// — a file cannot be tampered with or downgraded to weaker settings.
func TestAChangedFileDoesNotOpen(t *testing.T) {
	file, err := Seal([]byte("secret"), pw)
	if err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string][2]string{
		"time":    {"time: 3", "time: 1"},
		"memory":  {"memory_kib: 65536", "memory_kib: 8"},
		"threads": {"threads: 4", "threads: 1"},
	} {
		changed := strings.Replace(string(file), edit[0], edit[1], 1)
		if changed == string(file) {
			t.Fatalf("%s: the edit did not apply to:\n%s", name, file)
		}
		if _, err := Open([]byte(changed), pw); !errors.Is(err, ErrWrongPassword) {
			t.Errorf("%s changed: %v, want ErrWrongPassword", name, err)
		}
	}
	i := strings.Index(string(file), "data: ") + 6
	b := []byte(string(file))
	if b[i] == 'A' {
		b[i] = 'B'
	} else {
		b[i] = 'A'
	}
	if _, err := Open(b, pw); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("data changed: %v, want ErrWrongPassword", err)
	}
}

// A short password is refused; a file of another format is told as such.
func TestSealRefusesAShortPassword(t *testing.T) {
	if _, err := Seal([]byte("x"), "short"); err == nil {
		t.Fatal("a 5-character password was accepted")
	}
	if _, err := Open([]byte("format: something-else/1\n"), pw); err == nil || errors.Is(err, ErrWrongPassword) {
		t.Fatalf("another format = %v, want a format error", err)
	}
}
