// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// REQUESTS TO THE CLIENT — an elicitation form (MCP elicitation/create): the
// server asks the person something through the client itself, and the answer
// comes back to the server only, never into the model's context. Claude Code's
// CLI declares it at initialize; where a client does not, the server never asks.

// clientRequests are the server's requests waiting for the client's answer.
type clientRequests struct {
	asking  sync.Mutex
	elicits bool // the client declared elicitation at initialize
	seq     int
	waiting map[string]chan request
}

// canElicit reports whether the client takes elicitation forms.
func (s *Server) canElicit() bool {
	s.asking.Lock()
	defer s.asking.Unlock()
	return s.elicits
}

// answered hands a client's answer to the request waiting for it.
func (s *Server) answered(msg request) {
	var id string
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		return
	}
	s.asking.Lock()
	ch := s.waiting[id]
	delete(s.waiting, id)
	s.asking.Unlock()
	if ch != nil {
		ch <- msg
	}
}

// ask sends method to the client and waits for its answer, up to timeout.
func (s *Server) ask(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	s.asking.Lock()
	s.seq++
	id := "ktw-mcp-" + strconv.Itoa(s.seq)
	ch := make(chan request, 1)
	if s.waiting == nil {
		s.waiting = map[string]chan request{}
	}
	s.waiting[id] = ch
	s.asking.Unlock()

	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	s.mu.Lock()
	_, err := s.out.Write(append(data, '\n'))
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case msg := <-ch:
		if msg.Error != nil {
			return nil, fmt.Errorf("%s", msg.Error.Message)
		}
		return msg.Result, nil
	case <-time.After(timeout):
		s.asking.Lock()
		delete(s.waiting, id)
		s.asking.Unlock()
		return nil, errors.New("no answer in time")
	}
}

// askCode asks the person, through the client's form, the code of their
// authenticator app proving act: the code, or why there is none (declined,
// cancelled, no answer).
func (s *Server) askCode(act string) (string, error) {
	raw, err := s.ask("elicitation/create", map[string]any{
		"message": fmt.Sprintf("KaTyoti: the code of your authenticator app, to prove %s", act),
		"requestedSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "title": "Code", "description": "the 6 digits your authenticator app shows for KaTyoti",
					"minLength": 6, "maxLength": 6},
			},
			"required": []string{"code"},
		},
	}, 3*time.Minute)
	if err != nil {
		return "", err
	}
	var res struct {
		Action  string `json:"action"`
		Content struct {
			Code string `json:"code"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return "", err
	}
	if res.Action != "accept" || res.Content.Code == "" {
		return "", fmt.Errorf("the person %s the code", map[string]string{"decline": "declined", "cancel": "cancelled"}[res.Action]+orAnswered(res.Action))
	}
	return res.Content.Code, nil
}

func orAnswered(action string) string {
	if action == "decline" || action == "cancel" {
		return ""
	}
	return "gave no"
}
