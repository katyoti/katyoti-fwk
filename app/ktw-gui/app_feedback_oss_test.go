// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// The published build answers an error code the form translates, never an
// English sentence.
func TestFeedbackIsNotAvailableInThePublishedBuild(t *testing.T) {
	a := &App{}
	if got := a.FeedbackSubmit(FeedbackSubmitInput{Title: "x"}); got.ErrorCode != "unavailable" || got.Error == "" || got.Name != "" {
		t.Fatalf("FeedbackSubmit = %+v", got)
	}
	if got := a.FeedbackGetConfig(); got.ErrorCode != "unavailable" || got.Error == "" {
		t.Fatalf("FeedbackGetConfig = %+v", got)
	}
	if got := a.FeedbackSaveConfig("d"); got != "unavailable" {
		t.Fatalf("FeedbackSaveConfig = %q", got)
	}
}
