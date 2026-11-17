// SPDX-License-Identifier: Apache-2.0

//go:build linux && !cgo

package voice

import (
	"fmt"
	"io"
)

// errNoAudioBackend explains the one build combination that has no sound
// device available, and how to get one.
var errNoAudioBackend = fmt.Errorf(
	"voice playback unavailable: this Linux binary was built without cgo, and the ALSA backend needs it — " +
		"rebuild on a Linux host with CGO_ENABLED=1, or use the desktop notification channel instead")

// silentPlayer stands in for the real player on cgo-less Linux builds. Every
// call reports errNoAudioBackend rather than pretending the audio played:
// notify's channel dispatch logs the failure, so a misconfigured voice channel
// is visible instead of silently doing nothing.
type silentPlayer struct{}

// NewPlayer returns the stub player — see errNoAudioBackend.
func NewPlayer() AudioPlayer { return &silentPlayer{} }

func (p *silentPlayer) Play(_ []byte, _ float64) error { return errNoAudioBackend }

func (p *silentPlayer) PlayStream(reader io.ReadCloser, _ float64) error {
	reader.Close()
	return errNoAudioBackend
}
