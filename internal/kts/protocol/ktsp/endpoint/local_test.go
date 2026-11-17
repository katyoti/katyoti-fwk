// SPDX-License-Identifier: Apache-2.0

package endpoint_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/endpoint"
)

func TestLocalSendReceive(t *testing.T) {
	c := ktsp.NewEndpoint("echo-svc", "Echo", "1.0.0")

	echo := ktsp.NewSlot("echo", ktsp.DefaultInstance, "Echo slot", nil)
	echo.Add(ktsp.RawMethod("say", ktsp.MethodOpts{}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		return params, nil
	}))
	c.AddSlot(echo)

	tr := endpoint.NewLocal(c)
	defer tr.Close()

	req, _ := ktsp.NewRequest(1, "echo/say", map[string]string{"msg": "hello"})
	resp, err := tr.Send(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result map[string]string
	if err := resp.ParseResult(&result); err != nil {
		t.Fatal(err)
	}
	if result["msg"] != "hello" {
		t.Errorf("msg = %q, want %q", result["msg"], "hello")
	}
}

func TestLocalSlotNotFound(t *testing.T) {
	c := ktsp.NewEndpoint("test-svc", "Test", "1.0.0")
	tr := endpoint.NewLocal(c)
	defer tr.Close()

	req, _ := ktsp.NewRequest(1, "missing/method", nil)
	resp, err := tr.Send(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != ktsp.CodeSlotNotFound {
		t.Errorf("Code = %d, want %d", resp.Error.Code, ktsp.CodeSlotNotFound)
	}
}

func TestLocalClosed(t *testing.T) {
	c := ktsp.NewEndpoint("test-svc", "Test", "1.0.0")
	tr := endpoint.NewLocal(c)
	tr.Close()

	req, _ := ktsp.NewRequest(1, "ktsp/ping", nil)
	_, err := tr.Send(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on closed transport")
	}
}
