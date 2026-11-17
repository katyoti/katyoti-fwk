// SPDX-License-Identifier: Apache-2.0

// Package sealed protects a secret with a password: Seal turns bytes into a
// text file only that password opens, Open turns it back. It is the one place
// katyoti encrypts at rest — a team's keys bundle today, a signing key next.
//
// The file is YAML, readable and mailable, its parameters in clear:
//
//	format: katyoti-sealed/1
//	kdf: {name: argon2id, time: 3, memory_kib: 65536, threads: 4, salt: <base64>}
//	cipher: {name: xchacha20-poly1305, nonce: <base64>}
//	data: <base64 ciphertext>
//
// The key is derived from the password with argon2id (memory-hard: guessing a
// password costs 64 MiB and a few hundred milliseconds per try) and the data is
// sealed with XChaCha20-Poly1305 (authenticated: a changed byte fails to open).
// The header is authenticated too — sealed as associated data — so changing a
// parameter fails exactly like a wrong password, and a file cannot be
// downgraded to weaker settings.
package sealed

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// Format names the file's layout and version.
const Format = "katyoti-sealed/1"

// MinPassword is the shortest password Seal accepts, in characters.
const MinPassword = 12

// The key derivation's cost: OWASP's argon2id baseline, raised in memory.
const (
	kdfTime    = 3
	kdfMemory  = 64 * 1024 // KiB
	kdfThreads = 4
	saltLen    = 16
)

// ErrWrongPassword is a file that does not open: the wrong password, or a file
// (or its header) that was changed. The two are not told apart on purpose.
var ErrWrongPassword = errors.New("sealed: the password does not open this file (or the file was changed)")

// KDF is the key derivation's parameters, as the file records them.
type KDF struct {
	Name      string `yaml:"name"`
	Time      uint32 `yaml:"time"`
	MemoryKiB uint32 `yaml:"memory_kib"`
	Threads   uint8  `yaml:"threads"`
	Salt      string `yaml:"salt"`
}

// Cipher is the encryption's parameters, as the file records them.
type Cipher struct {
	Name  string `yaml:"name"`
	Nonce string `yaml:"nonce"`
}

// File is a sealed file.
type File struct {
	Format string `yaml:"format"`
	KDF    KDF    `yaml:"kdf"`
	Cipher Cipher `yaml:"cipher"`
	Data   string `yaml:"data"`
}

// Seal encrypts plain with password into a sealed file's bytes. A password
// shorter than MinPassword characters is refused.
func Seal(plain []byte, password string) ([]byte, error) {
	if utf8.RuneCountInString(password) < MinPassword {
		return nil, fmt.Errorf("sealed: a password of at least %d characters", MinPassword)
	}
	salt := make([]byte, saltLen)
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	f := File{
		Format: Format,
		KDF: KDF{Name: "argon2id", Time: kdfTime, MemoryKiB: kdfMemory, Threads: kdfThreads,
			Salt: base64.StdEncoding.EncodeToString(salt)},
		Cipher: Cipher{Name: "xchacha20-poly1305", Nonce: base64.StdEncoding.EncodeToString(nonce)},
	}
	aead, err := chacha20poly1305.NewX(derive(password, salt, f.KDF))
	if err != nil {
		return nil, err
	}
	f.Data = base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, plain, header(f)))
	return marshal.Marshal(f, marshal.YAML)
}

// Open decrypts a sealed file's bytes with password. A wrong password, or a
// changed file or header, is ErrWrongPassword.
func Open(file []byte, password string) ([]byte, error) {
	var f File
	if err := config.DecodeBytes(file, marshal.YAML, &f); err != nil {
		return nil, fmt.Errorf("sealed: not a sealed file: %w", err)
	}
	if f.Format != Format {
		return nil, fmt.Errorf("sealed: format %q, want %q", f.Format, Format)
	}
	if f.KDF.Name != "argon2id" || f.Cipher.Name != "xchacha20-poly1305" {
		return nil, fmt.Errorf("sealed: unknown kdf %q or cipher %q", f.KDF.Name, f.Cipher.Name)
	}
	salt, err1 := base64.StdEncoding.DecodeString(f.KDF.Salt)
	nonce, err2 := base64.StdEncoding.DecodeString(f.Cipher.Nonce)
	data, err3 := base64.StdEncoding.DecodeString(f.Data)
	if err := errors.Join(err1, err2, err3); err != nil || len(nonce) != chacha20poly1305.NonceSizeX {
		return nil, ErrWrongPassword
	}
	if f.KDF.Time == 0 || f.KDF.MemoryKiB == 0 || f.KDF.Threads == 0 {
		return nil, ErrWrongPassword
	}
	aead, err := chacha20poly1305.NewX(derive(password, salt, f.KDF))
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, data, header(f))
	if err != nil {
		return nil, ErrWrongPassword
	}
	return plain, nil
}

func derive(password string, salt []byte, k KDF) []byte {
	return argon2.IDKey([]byte(password), salt, k.Time, k.MemoryKiB, k.Threads, chacha20poly1305.KeySize)
}

// header is what the encryption authenticates beside the data: every
// parameter of the file.
func header(f File) []byte {
	return []byte(strings.Join([]string{f.Format, f.KDF.Name, fmt.Sprint(f.KDF.Time), fmt.Sprint(f.KDF.MemoryKiB),
		fmt.Sprint(f.KDF.Threads), f.KDF.Salt, f.Cipher.Name, f.Cipher.Nonce}, "\n"))
}
