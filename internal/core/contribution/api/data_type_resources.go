// SPDX-License-Identifier: Apache-2.0

package contribapi

// ResourceTypeDataType is the discriminator for canonical data-type
// Contributions. A `data-type` Contribution's data layer is a DSL Schema
// (`internal/extensions/schema`); the hub's SchemaService parses each
// one at hub boot (Phase 1 of the bootstrap protocol) and serves them
// universe-wide via GetDataType / GetJSONSchema / ValidateAgainst.
//
// Sidecars ship data-type Contributions by placing `*.dsl.yaml` files
// under `embed/data-types/` and wiring `ContributeDataTypeResources`
// in bootstrap. Other Contribution kinds reference their canonical
// data-type via the envelope's `Schema` field — `<provider>:/data-type/<name>`.
//
// Canonical URI: `<provider>:/data-type/<name>` where `<name>` is the
// file's basename without extension (e.g. `notation`).
const ResourceTypeDataType = "data-type"

// ResourceTypeContributionType is the meta-kind: a contribution of this kind
// DECLARES another kind — its schema, its validation severity, and whether its
// content is read or run. Kinds are contributions like anything else, so
// listing them is an ordinary query rather than a separate registry.
const ResourceTypeContributionType = "contribution-type"
