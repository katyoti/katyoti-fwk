// SPDX-License-Identifier: Apache-2.0

// The feedback form in the published build: the feedbacks extension is not
// part of it, so the bindings stay (the frontend is the same) and answer the
// error code `unavailable`; the form translates the code into the person's
// language (i18n key feedback.unavailable).

package main

// feedbackErrUnavailable is the error code every feedback binding of the
// published build answers.
const feedbackErrUnavailable = "unavailable"

// FeedbackSubmit answers that feedback is not available in this build.
//
// Wails-bound: window.go.main.App.FeedbackSubmit(input).
func (a *App) FeedbackSubmit(FeedbackSubmitInput) FeedbackSubmitResult {
	return FeedbackSubmitResult{Error: feedbackErrUnavailable, ErrorCode: feedbackErrUnavailable}
}

// FeedbackGetConfig answers that feedback is not available in this build.
//
// Wails-bound: window.go.main.App.FeedbackGetConfig().
func (a *App) FeedbackGetConfig() FeedbackConfigResult {
	return FeedbackConfigResult{Error: feedbackErrUnavailable, ErrorCode: feedbackErrUnavailable}
}

// FeedbackSaveConfig answers the error code: feedback is not available in
// this build.
//
// Wails-bound: window.go.main.App.FeedbackSaveConfig(folder).
func (a *App) FeedbackSaveConfig(string) string {
	return feedbackErrUnavailable
}
