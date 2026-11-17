// SPDX-License-Identifier: Apache-2.0

// Package core implements the PublishersService: it lists the contributed
// publication connectors and publishes a document tree through one, by its
// method URI, after checking the tree and the instance's settings. It knows
// no connector: every one is a contribution.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/internal/scan"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// Service is the PublishersService.
type Service struct{}

// New is the service.
func New() *Service { return &Service{} }

var _ api.PublishersService = (*Service)(nil)

// Connectors lists the publication-connector entries of the universe — of
// this binary alone when no catalog aggregates the universe (a test, an
// offline command).
func (s *Service) Connectors(ctx context.Context, _ api.ConnectorsInput) (api.ConnectorsOutput, error) {
	entries, err := mbapi.QueryTyped[api.ConnectorDecl](ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Kind: api.ContributionTypePublicationConnector})
	if err != nil {
		entries, err = mbapi.QueryTyped[api.ConnectorDecl](ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: api.ContributionTypePublicationConnector})
	}
	if err != nil {
		return api.ConnectorsOutput{}, fmt.Errorf("publications: list the publication connectors: %w", err)
	}
	out := api.ConnectorsOutput{Connectors: []api.ConnectorDecl{}}
	for _, e := range entries {
		out.Connectors = append(out.Connectors, e.Body)
	}
	sort.SliceStable(out.Connectors, func(i, j int) bool { return out.Connectors[i].Name < out.Connectors[j].Name })
	return out, nil
}

// Publish checks the tree and the settings, then runs the connector's publish
// method with the tree, the settings and the state it kept.
func (s *Service) Publish(ctx context.Context, in api.PublishRequest) (api.PublishOutput, error) {
	c, err := s.connector(ctx, in.Connector)
	if err != nil {
		return api.PublishOutput{}, err
	}
	if err := in.Tree.Validate(); err != nil {
		return api.PublishOutput{}, fmt.Errorf("%w: %v", cwmapi.ErrInvalidInput, err)
	}
	if err := checkSettings(ctx, c, in.Settings); err != nil {
		return api.PublishOutput{}, err
	}
	// The scan before it leaves: a finding holds the
	// publication for a person - the publisher never runs - unless a person
	// approved this very publication's entry.
	findings := scan.Scan(in.Tree, scanRules(ctx, c, in.Settings))
	if len(findings) > 0 && !in.Preview {
		held, err := hold(ctx, c, in, findings)
		if err != nil || held.Held {
			return held, err
		}
	}
	// The receipt: a publish that writes names where it
	// may write; the connector refuses every write outside it.
	if !in.Preview && (in.Receipt == nil || len(in.Receipt.Targets) == 0) {
		return api.PublishOutput{}, fmt.Errorf("%w: %w (connector %s)", cwmapi.ErrInvalidInput, api.ErrNoReceipt, c.Name)
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return api.PublishOutput{}, fmt.Errorf("publications: resolve ActionService: %w", err)
	}
	raw, err := json.Marshal(in.PublishInput)
	if err != nil {
		return api.PublishOutput{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.PublishTimeout())
	defer cancel()
	res, err := actions.Run(ctx, actionsapi.RunInput{URI: c.Publish, Input: raw})
	if err != nil {
		return api.PublishOutput{}, fmt.Errorf("publications: %s publish: %w", c.Name, err)
	}
	var out api.PublishOutput
	if len(res.Value) == 0 {
		return out, fmt.Errorf("publications: %s answered no value", c.Publish)
	}
	if err := json.Unmarshal(res.Value, &out); err != nil {
		return out, fmt.Errorf("publications: %s answered %s: %w", c.Publish, res.Value, err)
	}
	if in.Preview && out.State == nil {
		out.State = in.State
	}
	if in.Preview {
		out.Findings = findings
	}
	return out, nil
}

// scanRules are the scan's rules: PublicationScan.yaml's allowed hosts and
// tenants' terms, the publishing tenant's own terms left out, and the hosts
// the connector names for its target (its hosts method, over the instance's
// settings). No file: only the connector's hosts are allowed and no term is
// looked for. A hosts method that fails allows nothing more: the scan holds
// more, never less.
func scanRules(ctx context.Context, c api.ConnectorDecl, settings map[string]any) scan.Rules {
	cfg, _ := application.UseConfig[api.PublicationScan](ctx)
	tenant, _ := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
	hosts := append([]string(nil), cfg.AllowedHosts...)
	hosts = append(hosts, connectorHosts(ctx, c, settings)...)
	return scan.Rules{AllowedHosts: hosts, Tenant: tenant, TenantTerms: cfg.TenantTerms}
}

// connectorHosts are the hosts a connector names for its target through its
// hosts method: none when it declares none or the method fails.
func connectorHosts(ctx context.Context, c api.ConnectorDecl, settings map[string]any) []string {
	if c.Hosts == "" {
		return nil
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return nil
	}
	raw, err := json.Marshal(api.HostsInput{Settings: settings})
	if err != nil {
		return nil
	}
	res, err := actions.Run(ctx, actionsapi.RunInput{URI: c.Hosts, Input: raw})
	if err != nil || len(res.Value) == 0 {
		return nil
	}
	var out api.HostsOutput
	if json.Unmarshal(res.Value, &out) != nil {
		return nil
	}
	return out.Hosts
}

// hold is a publication with findings held for a person: a verdict entry in
// the inbox (« À faire ») listing them, and nothing written. An entry the
// person approved lets it go (Held false); one pending or rejected holds it
// again, without filing another.
func hold(ctx context.Context, c api.ConnectorDecl, in api.PublishRequest, findings []api.ScanFinding) (api.PublishOutput, error) {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return api.PublishOutput{}, fmt.Errorf("publications: the scan found %d thing(s) to see and no inbox can hold the publication: %w", len(findings), err)
	}
	out := api.PublishOutput{Held: true, Findings: findings}
	requestedBy := "publications:" + c.Name
	fp := scanPrint(findings)
	if in.ApprovedScan != "" {
		got, err := inbox.Get(ctx, inboxapi.GetInput{ID: in.ApprovedScan})
		if err != nil {
			return api.PublishOutput{}, fmt.Errorf("publications: read the inbox entry %s: %w", in.ApprovedScan, err)
		}
		// An entry holds only the publication it was filed for: by this
		// scan, for this connector, listing these very findings. Any other
		// is not an approval of what leaves now - a new entry is filed.
		mine := got.Entry.RequestedBy == requestedBy && strings.Contains(got.Entry.Description, fp)
		switch {
		case !mine:
		case got.Entry.Status == inboxapi.StatusApproved:
			return api.PublishOutput{}, nil
		case got.Entry.Status == inboxapi.StatusPending:
			out.Inbox = in.ApprovedScan
			out.Message = "La publication attend toujours votre décision dans « À faire »."
			return out, nil
		case got.Entry.Status == inboxapi.StatusRejected:
			out.Inbox = in.ApprovedScan
			out.Message = "La publication a été rejetée dans « À faire » : rien n'a été écrit."
			return out, nil
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Avant d'écrire « %s » avec %s, l'analyse a trouvé ce qui suit. Rien n'a été écrit : approuvez pour publier tel quel, ou rejetez.\n\n", in.Tree.Meta.Name, firstNonEmpty(c.Label, c.Name))
	for _, f := range findings {
		fmt.Fprintf(&b, "- %s (%s) : %s - « %s »\n", f.Key, f.Kind, f.Why, f.Excerpt)
	}
	fmt.Fprintf(&b, "\n%s\n", fp)
	filed, err := inbox.File(ctx, inboxapi.FileInput{
		Title:       fmt.Sprintf("Publication retenue : %s (%d point(s) à voir)", firstNonEmpty(in.Tree.Meta.Name, c.Name), len(findings)),
		Description: b.String(),
		RequestedBy: requestedBy,
		About:       in.About,
		Urgency:     "high",
		Expects:     "verdict",
	})
	if err != nil {
		return api.PublishOutput{}, fmt.Errorf("publications: hold the publication in the inbox: %w", err)
	}
	out.Inbox = filed.Request.ID
	out.Message = fmt.Sprintf("Rien n'a été écrit : %d point(s) à voir attendent votre décision dans « À faire ».", len(findings))
	return out, nil
}

// scanPrint names a scan's findings in one line its inbox entry carries: an
// approval holds for these findings only, never for a tree changed since.
func scanPrint(findings []api.ScanFinding) string {
	h := sha256.New()
	for _, f := range findings {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00", f.Key, f.Kind, f.Excerpt)
	}
	return "Empreinte de l'analyse : " + hex.EncodeToString(h.Sum(nil))[:16]
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

// connector is the contributed connector of that name.
func (s *Service) connector(ctx context.Context, name string) (api.ConnectorDecl, error) {
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.ConnectorDecl{}, err
	}
	for _, c := range list.Connectors {
		if c.Name == name {
			if c.Publish == "" {
				return api.ConnectorDecl{}, fmt.Errorf("%w: the publication connector %q declares no publish method", cwmapi.ErrInvalidInput, name)
			}
			return c, nil
		}
	}
	return api.ConnectorDecl{}, fmt.Errorf("%w: no publication connector %q is contributed in this universe", cwmapi.ErrNotFound, name)
}

// checkSettings validates an instance's settings against the data type its
// connector declares, through the universe's SchemaService: settings that do
// not match never reach the connector.
func checkSettings(ctx context.Context, c api.ConnectorDecl, settings map[string]any) error {
	if c.Settings == "" {
		if len(settings) > 0 {
			return fmt.Errorf("%w: the connector %s takes no settings", api.ErrInvalidSettings, c.Name)
		}
		return nil
	}
	schemas, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil || schemas == nil {
		return fmt.Errorf("publications: the settings of %s cannot be checked against %s: no SchemaService (%v)", c.Name, c.Settings, err)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	body, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("%w: %v", api.ErrInvalidSettings, err)
	}
	res, err := schemas.ValidateAgainst(ctx, schemaapi.ValidateAgainstInput{URI: c.Settings, Body: body})
	if err != nil {
		return fmt.Errorf("publications: check the settings of %s against %s: %w", c.Name, c.Settings, err)
	}
	if res.Valid {
		return nil
	}
	var why []string
	for _, d := range res.Diagnostics {
		if string(d.Severity) == "warning" {
			continue
		}
		if d.Path != "" {
			why = append(why, d.Path+": "+d.Message)
		} else {
			why = append(why, d.Message)
		}
	}
	return fmt.Errorf("%w for %s (%s): %s", api.ErrInvalidSettings, c.Name, c.Settings, strings.Join(why, "; "))
}
