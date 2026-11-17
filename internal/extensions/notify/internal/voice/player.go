// SPDX-License-Identifier: Apache-2.0

package voice

import "io"

// AudioPlayer is the interface for playing audio.
//
// The implementation is platform-split — see player_beep.go and
// player_nosound.go. The audio backend (ebitengine/oto, under gopxl/beep)
// reaches the sound device without cgo on Windows (syscall) and macOS
// (purego), but on Linux it binds ALSA through cgo. A cgo-less Linux build —
// which is how Linux binaries are cross-compiled from a Windows or macOS host —
// therefore cannot link it at all, so that combination gets a stub that reports
// the limitation instead of failing to compile.
type AudioPlayer interface {
	Play(audio []byte, volume float64) error
	PlayStream(reader io.ReadCloser, volume float64) error
}
