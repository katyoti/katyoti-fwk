// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The feedback bindings both builds carry: the form's types, the folder picker
// and the window capture. Filing and Feedback.yaml are in app_feedback_private.go
// (the development build, with the feedbacks extension) and app_feedback_oss.go
// (the published build, which answers that feedback is not available).

// FeedbackSubmitInput is what the form sends: the person's words, where they
// were, and the screenshot (base64 PNG, empty when none or removed). Who files
// it, the build and the OS are filled in here.
type FeedbackSubmitInput struct {
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Universe      string `json:"universe"`
	Route         string `json:"route"`
	ScreenshotPNG string `json:"screenshotPng"`
}

// FeedbackSubmitResult says where the feedback was filed, or why not.
type FeedbackSubmitResult struct {
	Name      string `json:"name,omitempty"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// FeedbackConfigResult is Feedback.yaml as the settings form edits it.
type FeedbackConfigResult struct {
	Folder    string `json:"folder"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

// FeedbackCaptureResult is the window as a base64 PNG, or why it could not be
// taken (the form then opens without a screenshot).
type FeedbackCaptureResult struct {
	PNG   string `json:"png,omitempty"`
	Error string `json:"error,omitempty"`
}

// FeedbackPickFolder opens a folder picker for the feedback folder.
//
// Wails-bound: window.go.main.App.FeedbackPickFolder().
func (a *App) FeedbackPickFolder() PickFolderResult {
	path, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                "Choose the folder feedback is written to",
		CanCreateDirectories: true,
	})
	if err != nil {
		return PickFolderResult{Error: err.Error()}
	}
	return PickFolderResult{Path: path}
}

// FeedbackCaptureWindow takes the GUI window as it is now, as a base64 PNG.
// The form calls it before it opens, so the form is not in the picture.
//
// Wails-bound: window.go.main.App.FeedbackCaptureWindow().
func (a *App) FeedbackCaptureWindow() FeedbackCaptureResult {
	png, err := captureOwnWindowPNG()
	if err != nil {
		return FeedbackCaptureResult{Error: err.Error()}
	}
	return FeedbackCaptureResult{PNG: base64.StdEncoding.EncodeToString(png)}
}
