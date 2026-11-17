// SPDX-License-Identifier: Apache-2.0

package local

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// AutoKTSPContainer is an Extension that reads all publish.PublishedService entries
// with publish.KTSPContainerCap and builds a Endpoint automatically.
//
// For each service:
//  1. Resolves the service instance from ServiceManager
//  2. Discovers publishable methods via reflection
//  3. Builds a KTSP Slot with auto-generated CLIMeta
//  4. Adds the slot to a shared Endpoint
//
// The container is stored as *ktsp.Endpoint("default") in the SM.
// Must be loaded AFTER all PublishService extensions have contributed.
// AutoKTSPOption configures AutoKTSPContainer behavior.
type AutoKTSPOption func(*autoKTSPConfig)

type autoKTSPConfig struct {
	flatCLI bool
}

// WithFlatCLI makes methods appear as top-level commands instead of nested under the slot name.
// Use when there's a single slot and you want `ktw-docs render` instead of `ktw-docs docs render`.
func WithFlatCLI() AutoKTSPOption {
	return func(c *autoKTSPConfig) { c.flatCLI = true }
}

func AutoKTSPContainer(appName, desc, version string, opts ...AutoKTSPOption) application.Extension {
	cfg := &autoKTSPConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(init *application.ApplicationInit) {
		// The serving hooks: what a process starts once it serves (ktsp nats).
		init.AddRegisterPoints("auto.ktsp", ".serving", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[ktsapi.ServingHook]())
		})
		init.AddAppInit("auto.ktsp", "", func(app *application.Application) error {
			entries := application.UseAppExtensions[publish.PublishedService](app)

			// Default-instance slots from KTSPContainerCap.
			var slots []*ktsp.Slot
			// Named-instance bindings from KTSPNamedInstancesCap. Each entry
			// (slot, instance) is registered via AddSlot(slot, instance) after the
			// Endpoint is created so dispatch can route on the parsed
			// instance token from the bus subject.
			type namedBinding struct {
				slot     *ktsp.Slot
				instance string
			}
			var named []namedBinding

			for _, ps := range entries {
				if cap := ps.FindCapability("ktsp-container"); cap != nil {
					ktspCap := cap.(publish.KTSPContainerCap)

					// Resolve the service from SM
					impl, err := resolveServiceByType(app, ps.ServiceType)
					if err != nil {
						return fmt.Errorf("auto-ktsp: resolve %s: %w", ps.Name, err)
					}

					// Build slot from reflection
					tags := ktsp.SlotTags{}
					if cfg.flatCLI {
						tags[ktsp.TagSlotCLIFlat] = "true"
					}
					// Serialize declared CLI groups so the CLI generator can
					// build noun-verb subcommand trees from multi-segment
					// MethodDef.CLIName paths.
					if len(ktspCap.CLIGroups) > 0 {
						if b, err := json.Marshal(ktspCap.CLIGroups); err == nil {
							tags[ktsp.TagSlotCLIGroups] = string(b)
						}
					}
					// Merge per-service slot tags from the publish capability.
					// A service can request `cli=false` to opt out of auto-CLI
					// generation while still being exposed over KTSP/HTTP.
					for k, v := range ktspCap.SlotTags {
						tags[k] = v
					}
					// Lazy resolver: re-fetch the service on every request so a
					// later PutService (e.g., sidecar discovery wrapping the
					// local SkillManager / TaskService with a composite) is
					// honored. Without this, the impl captured at AppInit
					// time bypasses the composite and remote sidecar
					// contributions never surface over KTSP.
					className := ps.ServiceType.Name()
					resolve := func() (any, error) {
						return application.GetServiceByName(app, className, "default")
					}

					// SlotName (already kebab) IS the contract identifier —
					// the categorical SlotType dimension was collapsed into
					// typeName during the Phase-A++ cleanup.
					slot := BuildKTSPSlotLazy(
						ktspCap.SlotName,
						ktsp.DefaultInstance,
						ktspCap.SlotDesc,
						impl,
						tags,
						ktspCap.Methods,
						resolve,
					)
					slots = append(slots, slot)
				}

				if nc := ps.FindCapability("ktsp-named-instances"); nc != nil {
					ncap := nc.(publish.KTSPNamedInstancesCap)
					if ncap.Provider == nil {
						return fmt.Errorf("auto-ktsp: %s: KTSPNamedInstancesCap.Provider is nil", ps.Name)
					}
					instances, err := ncap.Provider(app)
					if err != nil {
						return fmt.Errorf("auto-ktsp: named instances %s: %w", ps.Name, err)
					}

					tags := ktsp.SlotTags{}
					if cfg.flatCLI {
						tags[ktsp.TagSlotCLIFlat] = "true"
					}
					for k, v := range ncap.SlotTags {
						tags[k] = v
					}
					for name, impl := range instances {
						if impl == nil {
							return fmt.Errorf("auto-ktsp: named instances %s: provider returned nil impl for instance %q", ps.Name, name)
						}
						// One slot per (name, impl). The slot's methods are
						// the same shape as the slot type — taken from
						// ncap.Methods — but bound to this impl's reflected
						// receiver. No lazy resolve here: the provider hands
						// us concrete impls at AppInit. If hot-swap of named
						// instances ever becomes a requirement, add a
						// per-instance resolver to KTSPNamedInstancesCap and
						// route through BuildKTSPSlotLazy.
						slot := BuildKTSPSlotWithTags(
							ncap.SlotName,
							name,
							ncap.SlotDesc,
							impl,
							tags,
							ncap.Methods,
						)
						named = append(named, namedBinding{slot: slot, instance: name})
					}
				}
			}

			if len(slots) == 0 && len(named) == 0 {
				return nil
			}

			container := ktsp.NewEndpoint(appName, desc, version)
			// Default-instance slots from KTSPContainerCap — bound
			// explicitly under DefaultInstance so the strict-explicit
			// contract is visible at the call site.
			for _, slot := range slots {
				container.AddSlot(slot)
			}
			for _, nb := range named {
				// nb.slot already carries its instance name (set at
				// BuildKTSPSlotWithTags time above); AddSlot is single-arg.
				container.AddSlot(nb.slot)
			}
			application.PutService[*ktsp.Endpoint](app, "default", container)

			// Phase D pending — the per-method Go-I/O data-type
			// contribution path moves to a typed Producer registered
			// against the existing ContributionProvidersPoint in the
			// manifest extension. The Phase C direct-contribute to a
			// global DataTypePoint is removed; the producer rewire
			// lands with Phase D-5 (modify publish.go + auto_ktsp.go).
			return nil
		})
	}
}

// resolveServiceByType resolves a service from SM using its interface type.
// Uses reflection to call services.UseService[T] dynamically.
func resolveServiceByType(app *application.Application, svcType reflect.Type) (any, error) {
	className := svcType.Name()
	if className == "" {
		return nil, fmt.Errorf("service type has no name")
	}
	return application.GetServiceByName(app, className, "default")
}
