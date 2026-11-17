// SPDX-License-Identifier: Apache-2.0

// Package api is the publisher API (modules/publications/): one contract every
// publisher answers, so a publisher added later - a static site, a PDF, a
// SharePoint library, a markdown folder - works with no change to the API or
// to its callers.
//
// A publisher is a `publication-connector` contribution (ConnectorDecl): it
// declares the settings of its instances as a data type and one method,
// publish, which takes a DOCUMENT TREE (DocumentTree), the instance's settings
// and the state it kept from the last publish (PublishInput), and answers what
// it created, updated, skipped and refused and the new state it keeps
// (PublishOutput). Which publishers exist is not a list in this package: the
// PublishersService lists the contributed connectors and runs one by its
// method URI, after checking the settings against the data type it declares.
//
// The state is the connector's own - page ids, versions, what was sent and
// when - kept apart from the content: the tree never carries it.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ContributionTypePublicationConnector is the kind a publisher is contributed
// as.
const ContributionTypePublicationConnector = "publication-connector"

// ErrInvalidSettings is what a publish answers when an instance's settings do
// not match the data type its connector declares: the connector is not run.
var ErrInvalidSettings = errors.New("publications: invalid settings")

// ConnectorDecl is a `publication-connector` entry: one publisher.
type ConnectorDecl struct {
	Name        string `json:"name"                jsonschema:"description=The connector's name in kebab-case - what an instance names"`
	Label       string `json:"label,omitempty"     jsonschema:"description=The name shown to a person"`
	Description string `json:"description"         jsonschema:"description=Where it publishes - one sentence"`
	Icon        string `json:"icon,omitempty"      jsonschema:"description=A tabler icon name"`
	Settings    string `json:"settings,omitempty"  jsonschema:"description=The data-type URI of its instances' settings - checked before every publish; a connector without one takes no settings"`
	Publish     string `json:"publish"             jsonschema:"description=The method that publishes a tree: it answers PublishInput with PublishOutput"`
	TimeoutS    int    `json:"timeout_s,omitempty" jsonschema:"description=How long a publish may take in seconds - 600 when empty"`
	Hosts       string `json:"hosts,omitempty"     jsonschema:"description=The method that names the hosts its target lives on: it answers HostsInput with HostsOutput - a link to one of them passes the scan before publishing with no configuration"`
}

// HostsInput is what a connector's hosts method reads: the instance's
// settings.
type HostsInput struct {
	Settings map[string]any `json:"settings,omitempty" jsonschema:"description=The instance's settings - the shape of the data type its connector declares"`
}

// HostsOutput is the hosts a connector's target lives on (a host, or a
// domain a host ends with): links to them are allowed by the scan before
// publishing.
type HostsOutput struct {
	Hosts []string `json:"hosts" jsonschema:"description=The hosts its target lives on - a host or a domain a host ends with"`
}

// PublishTimeout is how long a publish of this connector may take.
func (d ConnectorDecl) PublishTimeout() time.Duration {
	if d.TimeoutS > 0 {
		return time.Duration(d.TimeoutS) * time.Second
	}
	return 10 * time.Minute
}

// DocumentTree is what a publisher publishes: documents and groups in order,
// and what the tree as a whole is.
type DocumentTree struct {
	Meta  TreeMeta `json:"meta"  jsonschema:"description=What the tree is - its name and what it documents"`
	Nodes []Node   `json:"nodes" jsonschema:"description=The top-level documents and groups in order"`
}

// TreeMeta is what a tree says about itself - what every published page may
// name.
type TreeMeta struct {
	Name     string `json:"name"               jsonschema:"description=The publication's name"`
	Product  string `json:"product,omitempty"  jsonschema:"description=The product it documents (KaTyoti or ArchiFactory)"`
	Version  string `json:"version,omitempty"  jsonschema:"description=The product's version it documents"`
	Language string `json:"language,omitempty" jsonschema:"description=The tree's language when every document shares one: en or fr"`
	Run      string `json:"run,omitempty"      jsonschema:"description=The run that made the tree"`
	At       string `json:"at,omitempty"       jsonschema:"description=When the tree was made - RFC 3339"`
}

// Node is one entry of a tree: a document, or a group holding children in
// order - never both.
type Node struct {
	Key      string    `json:"key"                jsonschema:"description=The node's key - stable across publishes and unique in the tree; a connector keys its state by it"`
	Title    string    `json:"title"              jsonschema:"description=Its title"`
	Document *Document `json:"document,omitempty" jsonschema:"description=The document this node is - empty for a group"`
	Children []Node    `json:"children,omitempty" jsonschema:"description=A group's children in order"`
}

// IsGroup is whether the node is a group.
func (n Node) IsGroup() bool { return n.Document == nil }

// Document is one markdown document of a tree.
type Document struct {
	Markdown    string  `json:"markdown"              jsonschema:"description=Its body in standard markdown - mermaid blocks included"`
	Language    string  `json:"language,omitempty"    jsonschema:"description=Its language: en or fr"`
	Translation string  `json:"translation,omitempty" jsonschema:"description=The key of the node holding its paired translation"`
	Assets      []Asset `json:"assets,omitempty"      jsonschema:"description=The files it shows or attaches - images and diagrams and attachments"`
	Source      string  `json:"source,omitempty"      jsonschema:"description=Where it comes from - a file path or an object id"`
	Version     string  `json:"version,omitempty"     jsonschema:"description=The version of its source it was made from - a hash or a revision"`
}

// Asset is a file a document shows or attaches: its bytes or the local file
// holding them.
type Asset struct {
	Name      string `json:"name"                 jsonschema:"description=Its file name - what the markdown refers to"`
	MediaType string `json:"media_type,omitempty" jsonschema:"description=Its media type"`
	Path      string `json:"path,omitempty"       jsonschema:"description=The absolute local file holding it"`
	Content   []byte `json:"content,omitempty"    jsonschema:"description=Its bytes - base64 in JSON - when no path holds it"`
}

// Validate refuses a tree a connector cannot publish: a node with no key, a
// key used twice, a group holding no child or a document holding children.
func (t DocumentTree) Validate() error {
	seen := map[string]bool{}
	var walk func(prefix string, nodes []Node) error
	walk = func(prefix string, nodes []Node) error {
		for i, n := range nodes {
			at := fmt.Sprintf("%s[%d]", prefix, i)
			if n.Key == "" {
				return fmt.Errorf("publications: the node at %s has no key", at)
			}
			if seen[n.Key] {
				return fmt.Errorf("publications: the key %q is used twice in the tree", n.Key)
			}
			seen[n.Key] = true
			if n.Document != nil && len(n.Children) > 0 {
				return fmt.Errorf("publications: the node %q is a document and holds children", n.Key)
			}
			if err := walk(at+".children", n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk("nodes", t.Nodes)
}

// Documents is every document of the tree in order, depth first.
func (t DocumentTree) Documents() []Node {
	var out []Node
	var walk func(nodes []Node)
	walk = func(nodes []Node) {
		for _, n := range nodes {
			if !n.IsGroup() {
				out = append(out, n)
			}
			walk(n.Children)
		}
	}
	walk(t.Nodes)
	return out
}

// PublishInput is what every connector's publish method takes.
type PublishInput struct {
	Tree      DocumentTree   `json:"tree"                jsonschema:"description=What to publish"`
	Settings  map[string]any `json:"settings,omitempty"  jsonschema:"description=The instance's settings - the shape of the data type its connector declares"`
	State     map[string]any `json:"state,omitempty"     jsonschema:"description=What the connector kept from the instance's last publish - empty the first time"`
	Preview   bool           `json:"preview,omitempty"   jsonschema:"description=Say what a publish would do and change nothing - the answer's state is the one given"`
	Overwrite []string       `json:"overwrite,omitempty" jsonschema:"description=The keys of the nodes a person confirmed to overwrite though they were changed at the target since the last publish"`
	// Receipt names where this run may write: a publish
	// that changes something is refused without one, and the connector
	// refuses every write outside its targets.
	Receipt *Receipt `json:"receipt,omitempty" jsonschema:"description=The run's receipt - the run and the actor and the targets it may write to; required unless preview"`
}

// Receipt is what a publishing run may write: the run, who
// started it and its targets. « Publier » starting a run is not a person's
// act for every page it writes - the connector writes only under the targets
// its receipt names and journals every write.
type Receipt struct {
	Run     string          `json:"run"               jsonschema:"description=The run's id - every write it makes is journaled with it"`
	Actor   string          `json:"actor"             jsonschema:"description=Who started the run"`
	Targets []ReceiptTarget `json:"targets,omitempty" jsonschema:"description=Where the run may write"`
}

// ReceiptTarget is one place a run may write: a connection (the connector
// instance), a space (a site or a library) and the page the writes go under.
type ReceiptTarget struct {
	Connection string `json:"connection,omitempty" jsonschema:"description=The connector it writes through"`
	Space      string `json:"space"                jsonschema:"description=The space (a site or a library) it writes in"`
	Parent     string `json:"parent"               jsonschema:"description=The page the writes go under - itself or a page below it"`
}

// NewReceipt is the receipt of a run publishing with an instance's settings:
// its one target is the settings' space and parent page - the keys every
// publisher writing under a page names them by.
func NewReceipt(run, actor, connector string, settings map[string]any) Receipt {
	str := func(k string) string { v, _ := settings[k].(string); return v }
	return Receipt{Run: run, Actor: actor, Targets: []ReceiptTarget{{Connection: connector, Space: str("space"), Parent: str("parent")}}}
}

// ErrNoReceipt is what a publish that would write answers when it carries no
// receipt naming a target: nothing is written.
var ErrNoReceipt = errors.New("publications: a publish that writes needs a receipt naming its targets")

// ConnectorWrite is one line of a tenant's connector-write journal
// : a write a connector made - or refused - for a run.
type ConnectorWrite struct {
	At        string `json:"at"                jsonschema:"description=When - RFC 3339"`
	Run       string `json:"run,omitempty"     jsonschema:"description=The run it was made for"`
	Actor     string `json:"actor,omitempty"   jsonschema:"description=Who started the run"`
	Connector string `json:"connector"         jsonschema:"description=The connector that wrote"`
	Op        string `json:"op"                jsonschema:"description=create or update or move or delete or label or property or attachment or refused"`
	Space     string `json:"space,omitempty"   jsonschema:"description=The space written in"`
	Target    string `json:"target,omitempty"  jsonschema:"description=The page written - or the parent of a page created"`
	Page      string `json:"page,omitempty"    jsonschema:"description=The page id the write gave or changed"`
	Digest    string `json:"digest,omitempty"  jsonschema:"description=The sha256 of the content written"`
	Reason    string `json:"reason,omitempty"  jsonschema:"description=Why a write was refused"`
}

// Operations of a connector write.
const (
	WriteCreate     = "create"
	WriteUpdate     = "update"
	WriteMove       = "move"
	WriteDelete     = "delete"
	WriteLabel      = "label"
	WriteProperty   = "property"
	WriteAttachment = "attachment"
	WriteRefused    = "refused"
)

// PublishOutput is what every connector's publish method answers.
type PublishOutput struct {
	Created []Outcome      `json:"created,omitempty" jsonschema:"description=The nodes published for the first time"`
	Updated []Outcome      `json:"updated,omitempty" jsonschema:"description=The nodes published again"`
	Skipped []Outcome      `json:"skipped,omitempty" jsonschema:"description=The nodes left as they are at the target - unchanged or changed there since the last publish - each with why"`
	Refused []Outcome      `json:"refused,omitempty" jsonschema:"description=The nodes the target refused - each with why"`
	State   map[string]any `json:"state,omitempty"   jsonschema:"description=The new state the connector keeps for the instance - given back to its next publish"`
	Message string         `json:"message,omitempty" jsonschema:"description=One sentence for a person"`
	// Held is set when the scan before publishing found something: nothing
	// was written and a person decides in the inbox.
	Held     bool          `json:"held,omitempty"     jsonschema:"description=Nothing was written: the scan found what a person must see first"`
	Findings []ScanFinding `json:"findings,omitempty" jsonschema:"description=What the scan found - each in its document"`
	Inbox    string        `json:"inbox,omitempty"    jsonschema:"description=The inbox entry where a person approves or rejects the held publication"`
}

// Outcome is what happened to one node.
type Outcome struct {
	Key    string `json:"key"              jsonschema:"description=The node's key"`
	Title  string `json:"title,omitempty"  jsonschema:"description=Its title"`
	URL    string `json:"url,omitempty"    jsonschema:"description=Where it is at the target"`
	Reason string `json:"reason,omitempty" jsonschema:"description=Why it was skipped or refused"`
}

// DecodeSettings reads an instance's settings into the Go type of the data
// type its connector declares: how a connector reads what it was given.
func DecodeSettings[T any](settings map[string]any) (T, error) {
	var out T
	raw, err := json.Marshal(settings)
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	return out, nil
}

// PublishersServiceDef describes the service for KTSP/CLI auto-generation.
var PublishersServiceDef = meta.ServiceDef{
	Name:    "publishers",
	Short:   "Publish a document tree through a publication connector: the publishers contributed and one publish",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Connectors": {Short: "The publication connectors contributed in the universe", ReadOnly: true},
		"Publish": {Short: "Publish a document tree through a connector with an instance's settings and state", TimeoutS: 660,
			Long: "Checks the tree, then the settings against the data type the connector declares (refused without running it when they do not match), then calls the connector's publish method with the tree, the settings, the state it kept, preview and the keys confirmed to overwrite; answers what it created, updated, skipped and refused and the new state to keep."},
		"NewPublicationPage": {Short: "A publication's create page under its holder: one choice per contributed connector, its name, Créer", ReadOnly: true, TimeoutS: 30},
		"PublicationNew": {Short: "Create a publication of its holder through a publication connector, then open it", TimeoutS: 30,
			Long: "The create page's Créer: a `publication` object under the holder (parent) in its module, publishing the holder (object) through the connector named (a contributed publication connector, else refused), named by name or the connector's label; its destination is set afterwards in its own tabs. Answers {ok, message, publication, open}."},
	},
}

// ConnectorsInput lists the connectors.
type ConnectorsInput struct{}

// ConnectorsOutput is the contributed connectors, by name.
type ConnectorsOutput struct {
	Connectors []ConnectorDecl `json:"connectors"`
}

// PublishRequest is one publish: which connector, and its input.
type PublishRequest struct {
	Connector string `json:"connector" jsonschema:"description=The connector's name"`
	// ApprovedScan is the inbox entry a person approved for a held
	// publication: its findings no longer hold it.
	ApprovedScan string `json:"approved_scan,omitempty" jsonschema:"description=The inbox entry a person approved for this publication held by its scan - its findings no longer hold it while they are the ones it listed"`
	// About is the object the publication is made from (a study): the inbox
	// entry of a held publication is about it and opens it.
	About string `json:"about,omitempty" jsonschema:"description=The object the publication is made from - the inbox entry of a held publication opens it"`
	PublishInput
}

// The kinds of a scan finding.
const (
	ScanExternalURL = "external-url"
	ScanHiddenText  = "hidden-text"
	ScanTenantTerm  = "tenant-term"
)

// ScanFinding is one thing the scan before publishing found in a document.
type ScanFinding struct {
	Key     string `json:"key"     jsonschema:"description=The node it is in"`
	Kind    string `json:"kind"    jsonschema:"description=external-url or hidden-text or tenant-term"`
	Excerpt string `json:"excerpt" jsonschema:"description=What was found in its context"`
	Why     string `json:"why"     jsonschema:"description=Why it holds the publication"`
}

// PublicationScan is the scan's configuration (PublicationScan.yaml): the hosts
// a published link may point to and each tenant's terms that must never
// appear in another tenant's publication.
type PublicationScan struct {
	AllowedHosts []string            `yaml:"allowed_hosts" json:"allowed_hosts,omitempty"`
	TenantTerms  map[string][]string `yaml:"tenant_terms"  json:"tenant_terms,omitempty"`
}

// PublishersService lists the publishers and publishes through one.
type PublishersService interface {
	Connectors(ctx context.Context, in ConnectorsInput) (ConnectorsOutput, error)
	Publish(ctx context.Context, in PublishRequest) (PublishOutput, error)
	// NewPublicationPage is the publication type's create page under its
	// holder: one choice per contributed connector, its name, « Créer ».
	NewPublicationPage(ctx context.Context, in NewPublicationPageInput) (PublicationPageOutput, error)
	// PublicationNew creates a publication of its holder through a connector.
	PublicationNew(ctx context.Context, in PublicationNewInput) (PublicationNewOutput, error)
}
