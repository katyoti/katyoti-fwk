// SPDX-License-Identifier: Apache-2.0

// Command polyglot-producer is a contribution producer that links NOTHING from
// this repository — standard library only — and speaks only the wire a
// universe expects of a member. It stands for a producer written in any other
// language: every byte it emits is plain JSON a Python or TypeScript program
// could write the same way.
//
// It publishes the entries described by `<folder>/producer.json`, where the
// folder comes from KTW_CONTENT_ROOT or `--folder <path>` (the convention the
// generic host uses, so a universe declares it the same way).
//
// The two surfaces a member must answer, found by reading the hub (e19):
//
//  1. DISCOVERY is a process exec, not a KTSP call: the hub runs
//     `<binary> sc resources manifest [--include-system=false]` and parses the
//     Manifest JSON from stdout (appcore.RunResourcesList). A member that only
//     answered `resources/get-manifest` over KTSP would never be indexed.
//  2. CONTENT is KTSP over stdio: the hub spawns `<binary> <args…>` (the
//     membership's args, ending `sc ktsp stdio`) and exchanges newline-
//     delimited JSON-RPC 2.0 frames on stdin/stdout. The method is
//     `<slot>/<method>` in kebab case — `resources/open-layer`,
//     `resources/read`, … — with the params and results of
//     internal/extensions/manifest/api (ResourceService).
//
// Layer addresses are "sha256:" + hex(sha256(bytes)). The data layer is carried
// inline (base64 in JSON, as Go's []byte encodes); the source layer is not, so a
// reader's source request reaches this process through the hub.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// producerFile is this producer's own description of what it publishes.
type producerFile struct {
	Provider string      `json:"provider"`
	Version  string      `json:"version"`
	Entries  []entrySpec `json:"entries"`
}

type entrySpec struct {
	Type string          `json:"type"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
	// Source is a file under the folder, published as the `source` layer.
	Source          string `json:"source"`
	SourceMediaType string `json:"sourceMediaType"`
}

// --- the wire shapes (JSON only; field names are the contract) ---------------

type layerRef struct {
	Hash      string `json:"hash"`
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType,omitempty"`
	Inline    []byte `json:"inline,omitempty"`
}

type contribution struct {
	Provider string              `json:"provider"`
	Type     string              `json:"type"`
	ID       string              `json:"id"`
	URI      string              `json:"uri"`
	Layers   map[string]layerRef `json:"layers"`
}

type providerInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Mutable  bool   `json:"mutable,omitempty"`
	Declared bool   `json:"declared,omitempty"`
}

type manifest struct {
	Provider      string         `json:"provider"`
	Version       string         `json:"version"`
	Hash          string         `json:"hash"`
	Contributions []contribution `json:"contributions"`
	Providers     []providerInfo `json:"providers"`
}

type rpcFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// published holds the built manifest plus every layer's bytes, by URI.
type published struct {
	m      manifest
	layers map[string]map[string][]byte // uri -> layer -> bytes
}

func main() {
	folder := contentRoot(os.Args[1:])
	if folder == "" {
		fail("no content root: set KTW_CONTENT_ROOT or pass --folder")
	}
	pub, err := load(folder)
	if err != nil {
		fail(err.Error())
	}
	switch {
	case hasSeq(os.Args[1:], "sc", "resources", "manifest"):
		out, _ := json.MarshalIndent(pub.m, "", "  ")
		fmt.Println(string(out))
	case hasSeq(os.Args[1:], "sc", "ktsp", "stdio"):
		serve(pub)
	default:
		fail(fmt.Sprintf("unsupported invocation %q (this producer answers `sc resources manifest` and `sc ktsp stdio`)", strings.Join(os.Args[1:], " ")))
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "polyglot-producer:", msg)
	os.Exit(1)
}

func contentRoot(args []string) string {
	if r := strings.TrimSpace(os.Getenv("KTW_CONTENT_ROOT")); r != "" {
		return r
	}
	for i, a := range args {
		if a == "--folder" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--folder=") {
			return strings.TrimPrefix(a, "--folder=")
		}
	}
	return ""
}

func hasSeq(args []string, seq ...string) bool {
	for i := 0; i+len(seq) <= len(args); i++ {
		match := true
		for j, s := range seq {
			if args[i+j] != s {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func address(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func load(folder string) (*published, error) {
	raw, err := os.ReadFile(filepath.Join(folder, "producer.json"))
	if err != nil {
		return nil, err
	}
	var pf producerFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return nil, fmt.Errorf("producer.json: %w", err)
	}
	pub := &published{layers: map[string]map[string][]byte{}}
	m := manifest{Provider: pf.Provider, Version: pf.Version}
	kinds := map[string]bool{}
	whole := sha256.New()
	for _, e := range pf.Entries {
		data, err := json.Marshal(e.Data) // compacts: the canonical bytes
		if err != nil {
			return nil, fmt.Errorf("entry %s data: %w", e.ID, err)
		}
		src, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(e.Source)))
		if err != nil {
			return nil, fmt.Errorf("entry %s source: %w", e.ID, err)
		}
		uri := pf.Provider + ":/" + e.Type + "/" + e.ID
		m.Contributions = append(m.Contributions, contribution{
			Provider: pf.Provider, Type: e.Type, ID: e.ID, URI: uri,
			Layers: map[string]layerRef{
				"data":   {Hash: address(data), Size: int64(len(data)), MediaType: "application/json", Inline: data},
				"source": {Hash: address(src), Size: int64(len(src)), MediaType: e.SourceMediaType},
			},
		})
		pub.layers[uri] = map[string][]byte{"data": data, "source": src}
		if !kinds[e.Type] {
			kinds[e.Type] = true
			m.Providers = append(m.Providers, providerInfo{Name: pf.Provider, Type: e.Type, Mutable: true, Declared: true})
		}
		whole.Write([]byte(uri + "\n" + address(data) + "\n" + address(src) + "\n"))
	}
	m.Hash = "sha256:" + hex.EncodeToString(whole.Sum(nil))
	pub.m = m
	return pub, nil
}

// serve answers newline-delimited JSON-RPC 2.0 requests on stdin until EOF.
func serve(pub *published) {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		var req rpcFrame
		if err := json.Unmarshal(in.Bytes(), &req); err != nil || req.Method == "" || len(req.ID) == 0 {
			continue // not a request (or a notification): nothing to answer
		}
		result, rerr := handle(pub, req.Method, req.Params)
		resp := rpcFrame{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rerr}
		b, _ := json.Marshal(resp)
		_, _ = out.Write(append(b, '\n'))
		_ = out.Flush()
	}
}

func handle(pub *published, method string, params json.RawMessage) (any, *rpcError) {
	var in struct {
		URI   string `json:"uri"`
		Layer string `json:"layer"`
	}
	_ = json.Unmarshal(params, &in)
	switch method {
	case "ktsp/ping":
		return map[string]any{}, nil
	case "resources/get-manifest":
		return map[string]any{"manifest": pub.m}, nil
	case "resources/list":
		return map[string]any{"resources": pub.m.Contributions}, nil
	case "resources/types":
		var types []string
		for _, p := range pub.m.Providers {
			types = append(types, p.Type)
		}
		return types, nil
	case "resources/open-layer", "resources/read":
		layers, ok := pub.layers[in.URI]
		if !ok {
			return nil, &rpcError{Code: -32602, Message: "not found: " + in.URI}
		}
		name := in.Layer
		if method == "resources/read" || name == "" {
			name = "source"
		}
		b, ok := layers[name]
		if !ok {
			return nil, &rpcError{Code: -32602, Message: fmt.Sprintf("%s has no layer %q", in.URI, name)}
		}
		var ref layerRef
		for _, c := range pub.m.Contributions {
			if c.URI == in.URI {
				ref = c.Layers[name]
			}
		}
		ref.Inline = nil
		if method == "resources/read" {
			return map[string]any{"uri": in.URI, "mediaType": ref.MediaType, "content": b}, nil
		}
		return map[string]any{"ref": ref, "content": b}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
	}
}
