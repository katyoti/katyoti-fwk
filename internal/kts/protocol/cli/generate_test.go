// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	kcontainer "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
)

// testContainer builds a Endpoint with typed methods and CLIMeta for testing.
func testContainer() *kcontainer.Endpoint {
	c := kcontainer.NewEndpoint("test-svc", "Test service", "1.0.0")

	// "node" slot with create/list methods
	nodeSlot := kcontainer.NewSlot("node", kcontainer.DefaultInstance, "Node operations", kcontainer.SlotTags{
		kcontainer.TagSlotCLIName: "nodes",
	})
	nodeSlot.Add(kcontainer.RawMethod("create", kcontainer.MethodOpts{
		Desc: "Create a node",
		CLIMeta: &kcontainer.CLIMeta{
			Args: []kcontainer.CLIArg{
				{Field: "Name", Desc: "Node name", Required: true},
			},
			Flags: []kcontainer.CLIFlag{
				{Field: "Type", Name: "type", Short: "t", Desc: "Node type", Type: "string", Enum: []string{"a", "b"}},
				{Field: "Parent", Name: "parent", Short: "p", Desc: "Parent ID", Type: "string"},
			},
		},
	}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		return params, nil // echo
	}))
	nodeSlot.Add(kcontainer.RawMethod("list", kcontainer.MethodOpts{
		Desc: "List nodes",
		CLIMeta: &kcontainer.CLIMeta{
			Flags: []kcontainer.CLIFlag{
				{Field: "Limit", Name: "limit", Short: "l", Desc: "Max results", Type: "integer", Default: "50"},
			},
		},
	}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		return params, nil
	}))
	c.AddSlot(nodeSlot)

	// "internal" slot — should be skipped
	internalSlot := kcontainer.NewSlot("internal", kcontainer.DefaultInstance, "Internal ops", kcontainer.SlotTags{
		kcontainer.TagSlotInternal: "true",
	})
	internalSlot.Add(kcontainer.RawMethod("ping", kcontainer.MethodOpts{Desc: "Ping"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal("pong")
	}))
	c.AddSlot(internalSlot)

	// "hidden" slot with cli="false" — should be skipped
	hiddenSlot := kcontainer.NewSlot("hidden", kcontainer.DefaultInstance, "Hidden ops", kcontainer.SlotTags{
		kcontainer.TagSlotCLI: "false",
	})
	hiddenSlot.Add(kcontainer.RawMethod("secret", kcontainer.MethodOpts{Desc: "Secret"}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal("shhh")
	}))
	c.AddSlot(hiddenSlot)

	return c
}

func TestGenerateCLI(t *testing.T) {
	container := testContainer()
	rootCmd := &cobra.Command{Use: "test"}
	GenerateCLI(rootCmd, container)

	// Should have exactly 1 command (nodes) — internal and hidden are skipped
	cmds := rootCmd.Commands()
	found := map[string]bool{}
	for _, c := range cmds {
		found[c.Name()] = true
	}

	if !found["nodes"] {
		t.Errorf("expected 'nodes' command, got commands: %v", cmds)
	}
	if found["internal"] {
		t.Error("internal slot should be skipped")
	}
	if found["hidden"] {
		t.Error("hidden slot should be skipped")
	}
	if found["ktsp"] {
		t.Error("ktsp slot should be skipped (internal tag)")
	}
}

func TestSlotCommandHasSubcommands(t *testing.T) {
	container := testContainer()
	rootCmd := &cobra.Command{Use: "test"}
	GenerateCLI(rootCmd, container)

	nodesCmd := findCommand(rootCmd, "nodes")
	if nodesCmd == nil {
		t.Fatal("nodes command not found")
	}

	subs := map[string]bool{}
	for _, c := range nodesCmd.Commands() {
		subs[c.Name()] = true
	}

	if !subs["create"] {
		t.Error("expected 'create' subcommand")
	}
	if !subs["list"] {
		t.Error("expected 'list' subcommand")
	}
}

func TestMethodCommandArgs(t *testing.T) {
	container := testContainer()
	rootCmd := &cobra.Command{Use: "test"}
	GenerateCLI(rootCmd, container)

	createCmd := findCommand(findCommand(rootCmd, "nodes"), "create")
	if createCmd == nil {
		t.Fatal("nodes create command not found")
	}

	// Check Use string includes arg placeholder
	if createCmd.Use != "create <name>" {
		t.Errorf("Use = %q, want %q", createCmd.Use, "create <name>")
	}

	// Check flags exist
	typeFlag := createCmd.Flags().Lookup("type")
	if typeFlag == nil {
		t.Fatal("--type flag not found")
	}
	if typeFlag.Shorthand != "t" {
		t.Errorf("type shorthand = %q, want %q", typeFlag.Shorthand, "t")
	}

	parentFlag := createCmd.Flags().Lookup("parent")
	if parentFlag == nil {
		t.Fatal("--parent flag not found")
	}
}

func TestMethodCommandExecution(t *testing.T) {
	container := testContainer()
	rootCmd := &cobra.Command{Use: "test"}
	GenerateCLI(rootCmd, container)

	// Execute: test nodes create mynode --type a --parent p1
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{"nodes", "create", "mynode", "--type", "a", "--parent", "p1"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// The handler echoes params — verify no error occurred.
	// Output goes to os.Stdout via writeOutput; we verify the handler was called
	// by checking no error from Execute above.
}

func TestMethodCommandWithDefaultFlag(t *testing.T) {
	container := testContainer()
	rootCmd := &cobra.Command{Use: "test"}
	GenerateCLI(rootCmd, container)

	listCmd := findCommand(findCommand(rootCmd, "nodes"), "list")
	if listCmd == nil {
		t.Fatal("nodes list command not found")
	}

	limitFlag := listCmd.Flags().Lookup("limit")
	if limitFlag == nil {
		t.Fatal("--limit flag not found")
	}
	if limitFlag.DefValue != "50" {
		t.Errorf("limit default = %q, want %q", limitFlag.DefValue, "50")
	}
}

func TestBuildParams(t *testing.T) {
	meta := &kcontainer.CLIMeta{
		Args: []kcontainer.CLIArg{
			{Field: "Name", Desc: "Name", Required: true},
		},
		Flags: []kcontainer.CLIFlag{
			{Field: "Type", Name: "type", Type: "string"},
			{Field: "Count", Name: "count", Type: "integer"},
			{Field: "Score", Name: "score", Type: "number"},
			{Field: "Enabled", Name: "enabled", Type: "boolean"},
		},
	}

	typeVal := "workspace"
	countVal := "42"
	scoreVal := "3.14"
	enabledVal := true

	flagValues := map[string]*string{
		"Type":  &typeVal,
		"Count": &countVal,
		"Score": &scoreVal,
	}
	boolFlagValues := map[string]*bool{
		"Enabled": &enabledVal,
	}

	params, err := buildParams(meta, flagValues, boolFlagValues, []string{"mynode"})
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	if params["name"] != "mynode" {
		t.Errorf("name = %v, want %q", params["name"], "mynode")
	}
	if params["type"] != "workspace" {
		t.Errorf("type = %v, want %q", params["type"], "workspace")
	}
	if params["count"] != 42 {
		t.Errorf("count = %v, want 42", params["count"])
	}
	if params["score"] != 3.14 {
		t.Errorf("score = %v, want 3.14", params["score"])
	}
	if params["enabled"] != true {
		t.Errorf("enabled = %v, want true", params["enabled"])
	}
}

func TestBuildParams_StringSliceAndStringMap(t *testing.T) {
	meta := &kcontainer.CLIMeta{
		Flags: []kcontainer.CLIFlag{
			{Field: "Formats", Name: "formats", Type: "stringSlice"},
			{Field: "Vars", Name: "vars", Type: "stringMap"},
		},
	}
	formats := "csv, json , xlsx"
	vars := "app_id='00000000-0000-7000-8000-000000000003', region=eu"
	flagValues := map[string]*string{
		"Formats": &formats,
		"Vars":    &vars,
	}
	params, err := buildParams(meta, flagValues, nil, nil)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}

	gotFormats, ok := params["formats"].([]string)
	if !ok {
		t.Fatalf("formats type = %T, want []string", params["formats"])
	}
	wantFormats := []string{"csv", "json", "xlsx"}
	if len(gotFormats) != len(wantFormats) {
		t.Fatalf("formats len = %d, want %d", len(gotFormats), len(wantFormats))
	}
	for i, v := range wantFormats {
		if gotFormats[i] != v {
			t.Errorf("formats[%d] = %q, want %q", i, gotFormats[i], v)
		}
	}

	gotVars, ok := params["vars"].(map[string]string)
	if !ok {
		t.Fatalf("vars type = %T, want map[string]string", params["vars"])
	}
	if gotVars["app_id"] != "00000000-0000-7000-8000-000000000003" {
		t.Errorf("vars[app_id] = %q", gotVars["app_id"])
	}
	if gotVars["region"] != "eu" {
		t.Errorf("vars[region] = %q", gotVars["region"])
	}
}

func TestBuildParams_StringMapErrorPropagates(t *testing.T) {
	meta := &kcontainer.CLIMeta{
		Flags: []kcontainer.CLIFlag{
			{Field: "Vars", Name: "vars", Type: "stringMap"},
		},
	}
	bad := "no_equals_sign"
	flagValues := map[string]*string{"Vars": &bad}
	_, err := buildParams(meta, flagValues, nil, nil)
	if err == nil {
		t.Fatal("expected error for malformed stringMap")
	}
}

// A number or integer flag sends a number; a value that is not one is
// refused at the command line, naming the flag and the value, never sent on
// as text for the handler to fail on.
func TestBuildParams_NumberFlagRefusesText(t *testing.T) {
	meta := &kcontainer.CLIMeta{
		Flags: []kcontainer.CLIFlag{
			{Field: "Estimate", Name: "estimate", JSONKey: "estimate", Type: "number"},
			{Field: "Limit", Name: "limit", JSONKey: "limit", Type: "integer"},
		},
	}
	est, lim := "1.5", "3"
	params, err := buildParams(meta, map[string]*string{"Estimate": &est, "Limit": &lim}, nil, nil)
	if err != nil {
		t.Fatalf("buildParams: %v", err)
	}
	if params["estimate"] != 1.5 || params["limit"] != 3 {
		t.Errorf("params %#v; want estimate 1.5 and limit 3, as numbers", params)
	}
	for field, bad := range map[string]string{"Estimate": "1h", "Limit": "2.5"} {
		v := bad
		_, err := buildParams(meta, map[string]*string{field: &v}, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "--"+strings.ToLower(field)) || !strings.Contains(err.Error(), bad) {
			t.Errorf("--%s %q: err %v; want a refusal naming the flag and the value", strings.ToLower(field), bad, err)
		}
	}
	if u := buildFlagUsage(meta.Flags[0]); !strings.Contains(u, "a number") {
		t.Errorf("usage %q does not say the flag takes a number", u)
	}
}

// A struct or a list of structs arrives as its JSON, not as the text of it —
// the handler decodes it into the field (an actor, deliverables).
func TestBuildParams_JSONValue(t *testing.T) {
	meta := &kcontainer.CLIMeta{
		Flags: []kcontainer.CLIFlag{
			{Field: "By", Name: "by", Type: "jsonValue"},
			{Field: "Deliverables", Name: "deliverables", Type: "jsonValue"},
		},
	}
	by := `{"id":"jdoe","kind":"person"}`
	deliverables := `[{"title":"links fixed"}]`
	params, err := buildParams(meta, map[string]*string{"By": &by, "Deliverables": &deliverables}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if actor, ok := params["by"].(map[string]any); !ok || actor["kind"] != "person" {
		t.Errorf("by = %#v, want the decoded object", params["by"])
	}
	if list, ok := params["deliverables"].([]any); !ok || len(list) != 1 {
		t.Errorf("deliverables = %#v, want the decoded list", params["deliverables"])
	}
	bad := `{"id":`
	if _, err := buildParams(meta, map[string]*string{"By": &bad}, nil, nil); err == nil {
		t.Error("malformed JSON accepted")
	}
	// @path reads the JSON from a file — for JSON too long for a command line.
	file := filepath.Join(t.TempDir(), "deliverables.json")
	if err := os.WriteFile(file, []byte(deliverables), 0o644); err != nil {
		t.Fatal(err)
	}
	fromFile := "@" + file
	params, err = buildParams(meta, map[string]*string{"Deliverables": &fromFile}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if list, ok := params["deliverables"].([]any); !ok || len(list) != 1 {
		t.Errorf("deliverables from @file = %#v, want the decoded list", params["deliverables"])
	}
	missing := "@" + filepath.Join(t.TempDir(), "none.json")
	if _, err := buildParams(meta, map[string]*string{"By": &missing}, nil, nil); err == nil {
		t.Error("a missing @file was accepted")
	}
}

func TestParseStringSlice(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b ,c", []string{"a", "b", "c"}},
		{",a,,b,", []string{"a", "b"}},
		// A JSON array is the list as is: an element may hold a comma.
		{`["What carries the bus, and why?","hand-rolled transports"]`, []string{"What carries the bus, and why?", "hand-rolled transports"}},
		{` ["one"]`, []string{"one"}},
		{`[]`, []string{}},
	}
	for _, c := range cases {
		got, err := parseStringSlice(c.in)
		if err != nil {
			t.Errorf("parseStringSlice(%q): %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseStringSlice(%q) len = %d, want %d (%v)", c.in, len(got), len(c.want), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseStringSlice(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

// A malformed JSON list is refused, not split on its commas; @path reads the
// array from a file.
func TestParseStringSlice_JSON(t *testing.T) {
	if _, err := parseStringSlice(`["a",`); err == nil {
		t.Error("malformed JSON array accepted")
	}
	if _, err := parseStringSlice(`[1,2]`); err == nil {
		t.Error("a JSON array of numbers accepted for a list of strings")
	}
	file := filepath.Join(t.TempDir(), "options.json")
	if err := os.WriteFile(file, []byte(`["a, b", "c"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseStringSlice("@" + file)
	if err != nil || len(got) != 2 || got[0] != "a, b" {
		t.Errorf("@file = %v, %v; want [a, b c]", got, err)
	}
	meta := &kcontainer.CLIMeta{Flags: []kcontainer.CLIFlag{{Field: "Options", Name: "options", Type: "stringSlice"}}}
	bad := `["a",`
	if _, err := buildParams(meta, map[string]*string{"Options": &bad}, nil, nil); err == nil {
		t.Error("buildParams accepted a malformed JSON list")
	}
}

func TestParseStringMap(t *testing.T) {
	type kv struct{ k, v string }
	cases := []struct {
		in   string
		want []kv
	}{
		{"", nil},
		{"a=1", []kv{{"a", "1"}}},
		{"a=1,b=2", []kv{{"a", "1"}, {"b", "2"}}},
		{"a = 1 , b = 2", []kv{{"a", "1"}, {"b", "2"}}},
		{"x='hello world'", []kv{{"x", "hello world"}}},
		{`y="quoted"`, []kv{{"y", "quoted"}}},
		{"app_id=00000000-0000-7000-8000-000000000003", []kv{{"app_id", "00000000-0000-7000-8000-000000000003"}}},
		// A quoted value may itself contain commas — preserved as part of the value.
		{`app_ids="uuid1,uuid2"`, []kv{{"app_ids", "uuid1,uuid2"}}},
		{"app_ids='uuid1,uuid2,uuid3'", []kv{{"app_ids", "uuid1,uuid2,uuid3"}}},
		// Quoted value coexists with a regular pair separated by a top-level comma.
		{`a=1,app_ids="uuid1,uuid2"`, []kv{{"a", "1"}, {"app_ids", "uuid1,uuid2"}}},
	}
	for _, c := range cases {
		got, err := parseStringMap(c.in)
		if err != nil {
			t.Errorf("parseStringMap(%q) error: %v", c.in, err)
			continue
		}
		for _, p := range c.want {
			if got[p.k] != p.v {
				t.Errorf("parseStringMap(%q)[%q] = %q, want %q", c.in, p.k, got[p.k], p.v)
			}
		}
	}
}

func TestParseStringMap_Errors(t *testing.T) {
	bad := []string{
		"no_equals",
		"=novalue",
		"a=1,no_equals",
	}
	for _, in := range bad {
		if _, err := parseStringMap(in); err == nil {
			t.Errorf("parseStringMap(%q): expected error, got nil", in)
		}
	}
}

func TestFieldToJSON(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Name", "name"},
		{"ParentID", "parentID"},
		{"type", "type"},
		{"", ""},
	}
	for _, tt := range tests {
		got := fieldToJSON(tt.input)
		if got != tt.want {
			t.Errorf("fieldToJSON(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func findCommand(cmd *cobra.Command, name string) *cobra.Command {
	if cmd == nil {
		return nil
	}
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func hasAlias(cmd *cobra.Command, alias string) bool {
	for _, a := range cmd.Aliases {
		if a == alias {
			return true
		}
	}
	return false
}

// TestGenerateCLI_NounVerbGroups verifies that multi-segment cli.name paths
// build noun-verb subcommand trees (with group aliases/help from cli.groups),
// that plain methods stay top-level, and that nesting works at arbitrary depth.
func TestGenerateCLI_NounVerbGroups(t *testing.T) {
	echo := func(_ context.Context, params json.RawMessage) (json.RawMessage, error) { return params, nil }

	c := kcontainer.NewEndpoint("grp-svc", "Group service", "1.0.0")
	groupsJSON := `[{"Name":"page","Aliases":["pg"],"Short":"Single-page ops"},` +
		`{"Name":"workspace","Aliases":["ws"],"Short":"Workspace mgmt"}]`
	slot := kcontainer.NewSlot("grp", kcontainer.DefaultInstance, "Grouped", kcontainer.SlotTags{
		kcontainer.TagSlotCLIFlat:   "true",
		kcontainer.TagSlotCLIGroups: groupsJSON,
	})
	slot.Add(kcontainer.RawMethod("page-pull", kcontainer.MethodOpts{
		Desc: "Pull a page", Tags: kcontainer.MethodTags{kcontainer.TagMethodCLIName: "page pull"},
	}, echo))
	slot.Add(kcontainer.RawMethod("page-read", kcontainer.MethodOpts{
		Desc: "Read a page", Tags: kcontainer.MethodTags{kcontainer.TagMethodCLIName: "page read"},
	}, echo))
	slot.Add(kcontainer.RawMethod("search", kcontainer.MethodOpts{Desc: "Search"}, echo)) // stays top-level
	slot.Add(kcontainer.RawMethod("deep", kcontainer.MethodOpts{
		Desc: "Deep", Tags: kcontainer.MethodTags{kcontainer.TagMethodCLIName: "a b c cmd"},
	}, echo)) // arbitrary-depth nesting
	c.AddSlot(slot)

	rootCmd := &cobra.Command{Use: "test"}
	GenerateCLI(rootCmd, c)

	// page group: alias pg + short + pull/read children
	page := findCommand(rootCmd, "page")
	if page == nil {
		t.Fatal("expected 'page' group command")
	}
	if !hasAlias(page, "pg") {
		t.Errorf("page group missing alias pg; got %v", page.Aliases)
	}
	if page.Short != "Single-page ops" {
		t.Errorf("page short = %q, want %q", page.Short, "Single-page ops")
	}
	if findCommand(page, "pull") == nil {
		t.Error("expected 'page pull' subcommand")
	}
	if findCommand(page, "read") == nil {
		t.Error("expected 'page read' subcommand")
	}

	// plain method stays top-level; grouped methods do NOT leak to top-level
	if findCommand(rootCmd, "search") == nil {
		t.Error("expected 'search' at top level")
	}
	if findCommand(rootCmd, "page-pull") != nil {
		t.Error("'page-pull' must not be a top-level command (it is grouped)")
	}

	// arbitrary-depth nesting: a > b > c > cmd
	a := findCommand(rootCmd, "a")
	if a == nil {
		t.Fatal("expected group 'a'")
	}
	b := findCommand(a, "b")
	if b == nil {
		t.Fatal("expected group 'a b'")
	}
	cc := findCommand(b, "c")
	if cc == nil {
		t.Fatal("expected group 'a b c'")
	}
	if findCommand(cc, "cmd") == nil {
		t.Error("expected leaf 'a b c cmd'")
	}
}

// A method that refuses (its handler answers an error) is not a usage mistake:
// the generated command prints the error alone. A missing argument still is,
// and keeps cobra's usage text.
func TestMethodCommand_RefusalPrintsNoUsage(t *testing.T) {
	c := kcontainer.NewEndpoint("refuse-svc", "Refusing service", "1.0.0")
	slot := kcontainer.NewSlot("thing", kcontainer.DefaultInstance, "Things", nil)
	slot.Add(kcontainer.RawMethod("move", kcontainer.MethodOpts{
		Desc: "Move a thing",
		CLIMeta: &kcontainer.CLIMeta{
			Args: []kcontainer.CLIArg{{Field: "ID", Desc: "Thing id", Required: true}},
		},
	}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("refused by guard")
	}))
	c.AddSlot(slot)

	run := func(args ...string) (string, error) {
		rootCmd := &cobra.Command{Use: "test"}
		GenerateCLI(rootCmd, c)
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs(args)
		err := rootCmd.Execute()
		return out.String(), err
	}

	out, err := run("thing", "move", "t1")
	if err == nil || !strings.Contains(err.Error(), "refused by guard") {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("a refusal printed the usage text:\n%s", out)
	}

	out, err = run("thing", "move")
	if err == nil {
		t.Fatal("a missing argument was accepted")
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("a missing argument printed no usage text:\n%s", out)
	}
}

// A []byte field takes text, not its base64: a person or an agent writing a
// markdown overview passes the markdown. `@path` reads a file, `base64:` keeps
// binary reachable. The bytes the handler decodes are the bytes meant.
func TestBuildParams_BytesAreTextByDefault(t *testing.T) {
	type input struct {
		Content []byte `json:"content"`
	}
	field, _ := reflect.TypeOf(input{}).FieldByName("Content")
	meta := &kcontainer.CLIMeta{Flags: []kcontainer.CLIFlag{
		{Field: "Content", Name: "content", Type: corereflect.GoTypeToFlagType(field.Type)},
	}}
	file := filepath.Join(t.TempDir(), "overview.md")
	if err := os.WriteFile(file, []byte("# From a file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ flag, want string }{
		"text":   {"# Overview\n\nPlain *markdown*.", "# Overview\n\nPlain *markdown*."},
		"file":   {"@" + file, "# From a file\n"},
		"base64": {"base64:AAEC/w==", "\x00\x01\x02\xff"},
	} {
		v := c.flag
		params, err := buildParams(meta, map[string]*string{"Content": &v}, nil, nil)
		if err != nil {
			t.Fatalf("%s: buildParams: %v", name, err)
		}
		raw, _ := json.Marshal(params)
		var got input
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s: decode into the input: %v", name, err)
		}
		if string(got.Content) != c.want {
			t.Errorf("%s: content = %q, want %q", name, got.Content, c.want)
		}
	}
	bad := "base64:not base64!"
	if _, err := buildParams(meta, map[string]*string{"Content": &bad}, nil, nil); err == nil {
		t.Error("an invalid base64: value was accepted")
	}
}

// A path flag is the person's: the command line resolves it against its working
// directory, empty meaning that directory, before the method sees it.
func TestBuildParams_PathFlagIsResolvedAgainstTheWorkingDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	meta := &kcontainer.CLIMeta{Flags: []kcontainer.CLIFlag{
		{Field: "Folder", Name: "folder", JSONKey: "folder", Type: "string", Path: true},
		{Field: "Other", Name: "other", JSONKey: "other", Type: "string"},
	}}
	abs := filepath.Join(t.TempDir(), "x")
	for _, c := range []struct{ in, want string }{
		{"", cwd},
		{"sub", filepath.Join(cwd, "sub")},
		{abs, abs},
	} {
		in, other := c.in, "rel"
		params, err := buildParams(meta, map[string]*string{"Folder": &in, "Other": &other}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if params["folder"] != c.want {
			t.Errorf("--folder %q = %v; want %s", c.in, params["folder"], c.want)
		}
		if params["other"] != "rel" {
			t.Errorf("a flag not marked path changed: %v", params["other"])
		}
	}
}

// A generated flag is spelled with dashes, the kebab form of its field's name
// (dry_run → --dry-run); the underscore spelling stays a hidden alias that
// parses to the same flag and lands under the same JSON key, and the help
// lists only the dashed name.
func TestGeneratedFlagsAreDashedWithAnUnderscoreAlias(t *testing.T) {
	c := kcontainer.NewEndpoint("dash-svc", "Dashed flags", "1.0.0")
	slot := kcontainer.NewSlot("thing", kcontainer.DefaultInstance, "Things", nil)
	var got map[string]any
	slot.Add(kcontainer.RawMethod("migrate", kcontainer.MethodOpts{
		Desc: "Migrate things",
		CLIMeta: &kcontainer.CLIMeta{
			Flags: []kcontainer.CLIFlag{
				{Field: "DryRun", Name: "dry_run", JSONKey: "dry_run", Desc: "Only say", Type: "boolean"},
				{Field: "DependsOn", Name: "depends_on", JSONKey: "depends_on", Desc: "Prerequisites", Type: "stringSlice"},
				{Field: "NoIntake", Name: "no-intake", JSONKey: "no_intake", Desc: "Skip the intake", Type: "boolean"},
			},
		},
	}, func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		got = map[string]any{}
		_ = json.Unmarshal(params, &got)
		return nil, nil
	}))
	c.AddSlot(slot)

	run := func(args ...string) (string, error) {
		got = nil
		rootCmd := &cobra.Command{Use: "test"}
		GenerateCLI(rootCmd, c)
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs(args)
		err := rootCmd.Execute()
		return out.String(), err
	}

	for _, args := range [][]string{
		{"thing", "migrate", "--dry-run", "--depends-on", "a,b", "--no-intake"},
		{"thing", "migrate", "--dry_run", "--depends_on", "a,b", "--no_intake"},
	} {
		if _, err := run(args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got["dry_run"] != true || got["no_intake"] != true {
			t.Errorf("%v: params = %v, want dry_run and no_intake true", args, got)
		}
		if list, _ := got["depends_on"].([]any); len(list) != 2 {
			t.Errorf("%v: depends_on = %v, want [a b]", args, got["depends_on"])
		}
	}

	help, err := run("thing", "migrate", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"--dry-run", "--depends-on", "--no-intake"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lists no %s:\n%s", want, help)
		}
	}
	for _, alias := range []string{"dry_run", "depends_on", "no_intake"} {
		if strings.Contains(help, alias) {
			t.Errorf("help shows the alias %s:\n%s", alias, help)
		}
	}
}
