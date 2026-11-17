// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"encoding/json"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
	"os"
	"path/filepath"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	docsrender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
)

// TestDemo_GenerateDrawioFiles is a one-shot end-to-end live runner —
// not part of the regular test suite (gated by env KTW_DRAWIO_DEMO=1).
// Produces a `.drawio` file under <repo-root>/tmp/diagram-demos/ for each
// realistic scenario, exercising the full v2 chain (catalog resolve →
// scanner → ELK auto-layout via bun → DrawIOEmitterV2 with decorations
// and templated labels). Open the generated files in drawio.app to see
// the result.
//
// Run with: KTW_DRAWIO_DEMO=1 go test -run TestDemo_GenerateDrawioFiles -v ./internal/extensions/tools/diagram_emit/
func TestDemo_GenerateDrawioFiles(t *testing.T) {
	if os.Getenv("KTW_DRAWIO_DEMO") != "1" {
		t.Skip("set KTW_DRAWIO_DEMO=1 to run the live drawio demo (writes files to tmp/diagram-demos/)")
	}

	// Walk up from this package to the repo root, then build absolute paths.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := wd
	for !fileExists(filepath.Join(repoRoot, "go.mod")) {
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatalf("could not find repo root from %s", wd)
		}
		repoRoot = parent
	}
	outDir := filepath.Join(repoRoot, "tmp", "diagram-demos")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))

	demos := []struct {
		name    string
		uri     string
		diagram any
	}{
		{
			name: "app-flux-network",
			uri:  "drawio-test:/viewpoint/app-flux",
			diagram: map[string]any{
				"title": "App flux — Order intake",
				"nodes": []any{
					map[string]any{"id": "billing", "label": "Billing", "kind": "Application", "attributes": map[string]any{
						"domain": "reference", "markers": []any{"xls"},
						"description":        "Invoices and prices",
						"external_url":       "https://catalog.example.com/applications/billing",
						"external_url_label": "Catalog",
					}},
					map[string]any{"id": "ledger", "label": "Ledger", "kind": "Application", "attributes": map[string]any{
						"domain":             "invoicing",
						"description":        "Comptabilité Belgique",
						"external_url":       "https://catalog.example.com/applications/ledger",
						"external_url_label": "Catalog",
					}},
					map[string]any{"id": "dwh", "label": "Data warehouse", "kind": "Application", "attributes": map[string]any{
						"domain":             "reporting",
						"description":        "Datawarehouse central",
						"external_url":       "https://catalog.example.com/applications/warehouse",
						"external_url_label": "Catalog",
					}},
					map[string]any{"id": "etl", "label": "ETL pipeline", "kind": "Application", "attributes": map[string]any{
						"domain":      "transverse",
						"description": "ETL billing → warehouse",
					}},
					map[string]any{"id": "note1", "label": "Phase 1 — pilote BE", "kind": "Note", "attributes": map[string]any{}},
				},
				"edges": []any{
					map[string]any{"id": "e1", "source": "billing", "target": "etl", "kind": "Flow",
						"attributes": map[string]any{
							"interface_codes": []any{map[string]any{"code": "IF-7349", "url": "https://catalog.example.com/interfaces/abc"}},
							"data_types":      []any{"Article", "Prix"},
						},
					},
					map[string]any{"id": "e2", "source": "etl", "target": "dwh", "kind": "Flow",
						"attributes": map[string]any{
							"interface_codes": []any{map[string]any{"code": "IF-7350", "url": "https://catalog.example.com/interfaces/def"}},
							"data_types":      []any{"Article", "Stock"},
						},
					},
					map[string]any{"id": "e3", "source": "dwh", "target": "ledger", "kind": "Flow",
						"attributes": map[string]any{
							"interface_codes": []any{map[string]any{"code": "IF-7351", "url": "https://catalog.example.com/interfaces/ghi"}},
							"data_types":      []any{"FactureLigne"},
						},
					},
				},
			},
		},

		{
			name: "archimate-business-flow",
			uri:  "ktw-docs:/viewpoint/archimate3-layered",
			diagram: map[string]any{
				"title": "ArchiMate — Order intake (Business + Application + Tech)",
				"nodes": []any{
					map[string]any{"id": "customer", "label": "Customer", "kind": "BusinessActor", "attributes": map[string]any{}},
					map[string]any{"id": "place-order", "label": "Place Order", "kind": "BusinessProcess", "attributes": map[string]any{}},
					map[string]any{"id": "order-svc", "label": "Order Service", "kind": "ApplicationService", "attributes": map[string]any{}},
					map[string]any{"id": "order-comp", "label": "OrderApp", "kind": "ApplicationComponent", "attributes": map[string]any{}},
					map[string]any{"id": "order-data", "label": "Order", "kind": "DataObject", "attributes": map[string]any{}},
					map[string]any{"id": "prod-node", "label": "ProductionServer", "kind": "TechnologyNode", "attributes": map[string]any{}},
				},
				"edges": []any{
					map[string]any{"id": "e1", "source": "customer", "target": "place-order", "kind": "triggering"},
					map[string]any{"id": "e2", "source": "place-order", "target": "order-svc", "kind": "serving"},
					map[string]any{"id": "e3", "source": "order-svc", "target": "order-comp", "kind": "realization"},
					map[string]any{"id": "e4", "source": "order-comp", "target": "order-data", "kind": "access"},
					map[string]any{"id": "e5", "source": "order-comp", "target": "prod-node", "kind": "assignment"},
				},
			},
		},

		{
			name: "uml-class-model",
			uri:  "ktw-docs:/viewpoint/uml2-class",
			diagram: map[string]any{
				"title": "UML — Order domain model",
				"nodes": []any{
					map[string]any{"id": "user", "label": "User", "kind": "Class", "attributes": map[string]any{
						"attributes": []any{"+id: int", "+name: string", "+email: string"},
						"operations": []any{"+login()", "+logout()", "+placeOrder(): Order"},
					}},
					map[string]any{"id": "order", "label": "Order", "kind": "Class", "attributes": map[string]any{
						"attributes": []any{"+id: int", "+total: decimal", "+createdAt: date"},
						"operations": []any{"+submit()", "+cancel()"},
					}},
					map[string]any{"id": "line", "label": "OrderLine", "kind": "Class", "attributes": map[string]any{
						"attributes": []any{"+quantity: int", "+unitPrice: decimal"},
						"operations": []any{"+subtotal(): decimal"},
					}},
					map[string]any{"id": "product", "label": "Product", "kind": "Class", "attributes": map[string]any{
						"attributes": []any{"+sku: string", "+name: string", "+price: decimal"},
						"operations": []any{"+restock(qty: int)"},
					}},
				},
				"edges": []any{
					map[string]any{"id": "e1", "source": "user", "target": "order", "kind": "association"},
					map[string]any{"id": "e2", "source": "order", "target": "line", "kind": "composition"},
					map[string]any{"id": "e3", "source": "line", "target": "product", "kind": "association"},
				},
			},
		},
	}

	for _, d := range demos {
		t.Run(d.name, func(t *testing.T) {
			m, ok := d.diagram.(map[string]any)
			if !ok {
				t.Fatalf("demo %s: diagram is not a map", d.name)
			}
			m["viewpoint"] = d.uri
			diagJSON, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("marshal diagram: %v", err)
			}
			var ld drawioapi.LogicalDiagram
			if err := json.Unmarshal(diagJSON, &ld); err != nil {
				t.Fatalf("decode diagram: %v", err)
			}

			out, err := docsrender.Emit(ctx, &ld, false) // full ELK auto-layout via bun
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if len(out.Errors) > 0 {
				t.Errorf("errors: %v", out.Errors)
			}
			if len(out.Warnings) > 0 {
				t.Logf("warnings: %v", out.Warnings)
			}

			outPath := filepath.Join(outDir, d.name+".drawio")
			if err := os.WriteFile(outPath, []byte(out.XML), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			t.Logf("✓ wrote %s (%d bytes)", outPath, len(out.XML))
		})
	}
}

// fileExists is a tiny helper for the repo-root walk.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
