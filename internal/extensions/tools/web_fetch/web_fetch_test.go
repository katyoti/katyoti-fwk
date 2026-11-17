// SPDX-License-Identifier: Apache-2.0

package web_fetch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestScrape_EmptyURL(t *testing.T) {
	_, err := executeWebFetch(nil, FetchInput{URL: ""})
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("err = %v, want 'url is required'", err)
	}
}

func TestCrawl_EmptyURL(t *testing.T) {
	_, err := executeWebFetch(nil, FetchInput{Action: "crawl", URL: ""})
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("err = %v, want 'url is required'", err)
	}
}

func TestMap_EmptyURL(t *testing.T) {
	_, err := executeWebFetch(nil, FetchInput{Action: "map", URL: ""})
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("err = %v, want 'url is required'", err)
	}
}

func TestCrawlStatus_EmptyJobID(t *testing.T) {
	_, err := executeWebFetch(nil, FetchInput{Action: "crawl_status", JobID: ""})
	if err == nil || !strings.Contains(err.Error(), "job_id is required") {
		t.Errorf("err = %v, want 'job_id is required'", err)
	}
}

func TestUnknownAction(t *testing.T) {
	_, err := executeWebFetch(nil, FetchInput{Action: "invalid", URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("err = %v, want 'unknown action'", err)
	}
}

func TestDefaultAction_IsScrape(t *testing.T) {
	// Without a service, scrape will fail with "not available" rather than "unknown action".
	// Use context.Background() to avoid nil-ctx panic in DI resolution.
	_, err := executeWebFetch(context.Background(), FetchInput{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error (no service in context)")
	}
	// Should NOT say "unknown action" — default is scrape
	if strings.Contains(err.Error(), "unknown action") {
		t.Errorf("default action should be scrape, got: %v", err)
	}
}

func TestDescribe_Scrape(t *testing.T) {
	raw, _ := json.Marshal(FetchInput{URL: "https://example.com/page"})
	got := describeWebFetch(raw)
	if got != "web_fetch scrape url=https://example.com/page" {
		t.Errorf("got %q", got)
	}
}

func TestDescribe_Crawl(t *testing.T) {
	raw, _ := json.Marshal(FetchInput{Action: "crawl", URL: "https://example.com"})
	got := describeWebFetch(raw)
	if got != "web_fetch crawl url=https://example.com" {
		t.Errorf("got %q", got)
	}
}

func TestDescribe_CrawlStatus(t *testing.T) {
	raw, _ := json.Marshal(FetchInput{Action: "crawl_status", JobID: "abc-123"})
	got := describeWebFetch(raw)
	if got != "web_fetch crawl_status job_id=abc-123" {
		t.Errorf("got %q", got)
	}
}

func TestDescribe_LongURL(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("x", 100)
	raw, _ := json.Marshal(FetchInput{URL: long})
	got := describeWebFetch(raw)
	if !strings.Contains(got, "...") {
		t.Error("should truncate long URLs")
	}
}
