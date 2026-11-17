// SPDX-License-Identifier: Apache-2.0

package union

import (
	"encoding/json"
	"testing"
	"time"
)

// --- Test types ---

type Animal interface {
	Species() string
}

type Dog struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Bark bool   `json:"bark"`
}

func (d Dog) Species() string { return "dog" }

type Cat struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Lives int    `json:"lives"`
}

func (c Cat) Species() string { return "cat" }

// --- Tests ---

func TestRegistry_Deserialize(t *testing.T) {
	reg := NewRegistry[Animal]("type")
	reg.Register("dog", Dog{})
	reg.Register("cat", Cat{})

	// Dog
	raw := json.RawMessage(`{"type":"dog","name":"Rex","bark":true}`)
	result, err := reg.Deserialize(raw)
	if err != nil {
		t.Fatalf("deserialize dog: %v", err)
	}
	dog, ok := result.(Dog)
	if !ok {
		t.Fatalf("expected Dog, got %T", result)
	}
	if dog.Name != "Rex" || !dog.Bark {
		t.Errorf("dog = %+v", dog)
	}

	// Cat
	raw = json.RawMessage(`{"type":"cat","name":"Whiskers","lives":9}`)
	result, err = reg.Deserialize(raw)
	if err != nil {
		t.Fatalf("deserialize cat: %v", err)
	}
	cat, ok := result.(Cat)
	if !ok {
		t.Fatalf("expected Cat, got %T", result)
	}
	if cat.Name != "Whiskers" || cat.Lives != 9 {
		t.Errorf("cat = %+v", cat)
	}
}

func TestRegistry_UnknownType(t *testing.T) {
	reg := NewRegistry[Animal]("type")
	reg.Register("dog", Dog{})

	raw := json.RawMessage(`{"type":"fish","name":"Nemo"}`)
	_, err := reg.Deserialize(raw)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestRegistry_MissingField(t *testing.T) {
	reg := NewRegistry[Animal]("type")
	reg.Register("dog", Dog{})

	raw := json.RawMessage(`{"name":"Rex"}`)
	_, err := reg.Deserialize(raw)
	if err == nil {
		t.Fatal("expected error for missing discriminator")
	}
}

func TestRegistry_CustomField(t *testing.T) {
	type Message struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}

	reg := NewRegistry[any]("kind")
	reg.Register("text", Message{})

	raw := json.RawMessage(`{"kind":"text","content":"hello"}`)
	result, err := reg.Deserialize(raw)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	msg, ok := result.(Message)
	if !ok {
		t.Fatalf("expected Message, got %T", result)
	}
	if msg.Content != "hello" {
		t.Errorf("content = %q", msg.Content)
	}
}

func TestRegistry_Types(t *testing.T) {
	reg := NewRegistry[Animal]("type")
	reg.Register("dog", Dog{})
	reg.Register("cat", Cat{})

	types := reg.Types()
	if len(types) != 2 {
		t.Errorf("expected 2 types, got %d", len(types))
	}
}

// --- Test with embedded base (like events) ---

type EventBase struct {
	Type    string    `json:"type"`
	Time    time.Time `json:"ts"`
	Emitter string    `json:"emitter"`
}

type Event interface {
	EventType() string
}

type ClickEvent struct {
	EventBase
	X int `json:"x"`
	Y int `json:"y"`
}

func (e ClickEvent) EventType() string { return e.Type }

type KeyEvent struct {
	EventBase
	Key string `json:"key"`
}

func (e KeyEvent) EventType() string { return e.Type }

func TestRegistry_EmbeddedBase(t *testing.T) {
	reg := NewRegistry[Event]("type")
	reg.Register("click", ClickEvent{})
	reg.Register("key", KeyEvent{})

	raw := json.RawMessage(`{"type":"click","ts":"2026-03-28T10:00:00Z","emitter":"ui","x":100,"y":200}`)
	result, err := reg.Deserialize(raw)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	click, ok := result.(ClickEvent)
	if !ok {
		t.Fatalf("expected ClickEvent, got %T", result)
	}
	if click.X != 100 || click.Y != 200 {
		t.Errorf("click = %+v", click)
	}
	if click.Emitter != "ui" {
		t.Errorf("emitter = %q", click.Emitter)
	}
	if click.EventType() != "click" {
		t.Errorf("type = %q", click.EventType())
	}
}
