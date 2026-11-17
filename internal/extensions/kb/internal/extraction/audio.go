// SPDX-License-Identifier: Apache-2.0

package extraction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// AssemblyAIExtractor transcribes audio/video files using AssemblyAI API.
// For video: extracts audio track with ffmpeg first, then transcribes.
// Features: speaker diarization, language detection, auto chapters.
type AssemblyAIExtractor struct {
	APIKey string
}

func (e *AssemblyAIExtractor) Name() string { return "assemblyai-transcribe" }

func (e *AssemblyAIExtractor) Formats() []string {
	return []string{
		// Audio
		"mp3", "m4a", "wav", "flac", "ogg", "aac", "wma", "amr",
		// Video (audio extracted via ffmpeg)
		"mp4", "avi", "mkv", "mov", "webm", "wmv",
	}
}

func (e *AssemblyAIExtractor) Extract(ctx context.Context, inputPath string, outputDir string, config map[string]any, _ *kbapi.ExtractLimits) (*kbapi.ExtractResult, error) {
	if e.APIKey == "" {
		return nil, fmt.Errorf("ASSEMBLY_AI_API_KEY not set")
	}

	ext := strings.ToLower(filepath.Ext(inputPath))
	audioPath := inputPath

	// For video files: extract audio first via ffmpeg
	videoExts := map[string]bool{".mp4": true, ".avi": true, ".mkv": true, ".mov": true, ".webm": true, ".wmv": true}
	if videoExts[ext] {
		audioPath = filepath.Join(outputDir, "audio.mp3")
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", inputPath,
			"-vn", "-acodec", "libmp3lame", "-ar", "16000", "-ac", "1",
			"-y", audioPath,
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("ffmpeg audio extract: %w\n%s", err, string(output))
		}
		defer os.Remove(audioPath)
	}

	// Parse config options
	speakerLabels := true
	languageDetection := true
	autoChapters := false
	if config != nil {
		if v, ok := config["speaker_labels"].(bool); ok {
			speakerLabels = v
		}
		if v, ok := config["language_detection"].(bool); ok {
			languageDetection = v
		}
		if v, ok := config["auto_chapters"].(bool); ok {
			autoChapters = v
		}
	}

	// Step 1: Upload file to AssemblyAI
	uploadURL, err := e.uploadFile(ctx, audioPath)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}

	// Step 2: Submit transcription
	transcriptID, err := e.submitTranscript(ctx, uploadURL, speakerLabels, languageDetection, autoChapters)
	if err != nil {
		return nil, fmt.Errorf("submit: %w", err)
	}

	// Step 3: Poll until complete
	result, err := e.pollTranscript(ctx, transcriptID)
	if err != nil {
		return nil, fmt.Errorf("transcribe: %w", err)
	}

	// Step 4: Format as markdown
	md := formatTranscriptMarkdown(result, speakerLabels)

	contentFile := "content.md"
	if err := os.WriteFile(filepath.Join(outputDir, contentFile), []byte(md), 0o644); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}

	return &kbapi.ExtractResult{
		ContentFiles: []kbapi.ContentFile{
			{File: contentFile, Part: "whole", Index: 0},
		},
		TotalParts: 1,
	}, nil
}

// --- AssemblyAI API calls ---

func (e *AssemblyAIExtractor) uploadFile(ctx context.Context, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.assemblyai.com/v2/upload", f)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", e.APIKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		UploadURL string `json:"upload_url"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("upload error: %s", result.Error)
	}
	return result.UploadURL, nil
}

func (e *AssemblyAIExtractor) submitTranscript(ctx context.Context, audioURL string, speakerLabels, languageDetection, autoChapters bool) (string, error) {
	body := map[string]any{
		"audio_url":          audioURL,
		"speaker_labels":     speakerLabels,
		"language_detection": languageDetection,
	}
	if autoChapters {
		body["auto_chapters"] = true
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.assemblyai.com/v2/transcript", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", e.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("submit error: %s", result.Error)
	}
	return result.ID, nil
}

type assemblyAITranscript struct {
	Status     string `json:"status"`
	Text       string `json:"text"`
	Error      string `json:"error"`
	Utterances []struct {
		Speaker    string  `json:"speaker"`
		Text       string  `json:"text"`
		Start      int     `json:"start"`
		End        int     `json:"end"`
		Confidence float64 `json:"confidence"`
	} `json:"utterances"`
	Chapters []struct {
		Summary  string `json:"summary"`
		Headline string `json:"headline"`
		Start    int    `json:"start"`
		End      int    `json:"end"`
	} `json:"chapters"`
}

func (e *AssemblyAIExtractor) pollTranscript(ctx context.Context, id string) (*assemblyAITranscript, error) {
	url := "https://api.assemblyai.com/v2/transcript/" + id

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", e.APIKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result assemblyAITranscript
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}

		switch result.Status {
		case "completed":
			return &result, nil
		case "error":
			return nil, fmt.Errorf("transcription failed: %s", result.Error)
		default:
			// queued or processing — wait and retry
			time.Sleep(3 * time.Second)
		}
	}
}

// --- Markdown formatting ---

func formatTranscriptMarkdown(t *assemblyAITranscript, speakerLabels bool) string {
	var sb strings.Builder

	// Chapters as headings (if available)
	if len(t.Chapters) > 0 {
		for _, ch := range t.Chapters {
			sb.WriteString(fmt.Sprintf("## %s\n\n", ch.Headline))
			sb.WriteString(ch.Summary + "\n\n")
		}
		sb.WriteString("---\n\n")
	}

	// Utterances with speaker labels
	if speakerLabels && len(t.Utterances) > 0 {
		sb.WriteString("## Transcript\n\n")
		for _, u := range t.Utterances {
			timestamp := formatTimestamp(u.Start)
			sb.WriteString(fmt.Sprintf("**%s** (%s): %s\n\n", u.Speaker, timestamp, u.Text))
		}
	} else if t.Text != "" {
		// Plain text fallback
		sb.WriteString(t.Text)
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatTimestamp(ms int) string {
	s := ms / 1000
	m := s / 60
	s = s % 60
	h := m / 60
	m = m % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// AssemblyAIAvailable checks if the ASSEMBLY_AI_API_KEY env var is set.
func AssemblyAIAvailable() bool {
	return os.Getenv("ASSEMBLY_AI_API_KEY") != ""
}
