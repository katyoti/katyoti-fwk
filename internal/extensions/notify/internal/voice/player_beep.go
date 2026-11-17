// SPDX-License-Identifier: Apache-2.0

//go:build !linux || cgo

package voice

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
)

// beepPlayer plays MP3 audio using the gopxl/beep library.
type beepPlayer struct {
	initialized bool
}

// NewPlayer returns the real audio player.
func NewPlayer() AudioPlayer { return &beepPlayer{} }

// Play decodes buffered MP3 audio and plays it.
func (p *beepPlayer) Play(audio []byte, volume float64) error {
	if len(audio) == 0 {
		return fmt.Errorf("no audio data to play")
	}
	return p.PlayStream(io.NopCloser(bytes.NewReader(audio)), volume)
}

// PlayStream decodes a streaming MP3 reader and plays it.
func (p *beepPlayer) PlayStream(reader io.ReadCloser, volume float64) error {
	streamer, format, err := mp3.Decode(reader)
	if err != nil {
		reader.Close()
		return fmt.Errorf("decode MP3: %w", err)
	}
	defer streamer.Close()

	if !p.initialized {
		if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
			return fmt.Errorf("init speaker: %w", err)
		}
		p.initialized = true
	}

	var s beep.Streamer = streamer
	if volume > 0 && volume != 1.0 {
		dbVolume := math.Log2(volume) * 3.0
		s = &effects.Volume{
			Streamer: streamer,
			Base:     2,
			Volume:   dbVolume,
			Silent:   false,
		}
	} else if volume <= 0 {
		s = &effects.Volume{
			Streamer: streamer,
			Base:     2,
			Volume:   -10,
			Silent:   true,
		}
	}

	done := make(chan struct{})
	speaker.Play(beep.Seq(s, beep.Callback(func() {
		close(done)
	})))

	<-done
	slog.Debug("audio playback completed", "volume", volume)
	return nil
}
