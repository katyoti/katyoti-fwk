---
name: marketplace-authoring
description: "Author a whole marketplace (sidecars, infra, config-sets, seeds, vaults, solutions) from its marketplace.yaml, through a marketplace-project node or `ktw-compose marketplace build`. Load to create or update a team's marketplace."
advertised: false
hashtags: [marketplace, publish, release, infra, config-set, seed, vault, solution, signing]
category: guide
---

# Authoring a marketplace

A marketplace is ONE folder that consumers subscribe to: its catalog
(`katyoti-marketplace.yaml`, signed), the assets of every item, the infra
channel (`infra/`, the control plane and the installers) and the public key.
What it publishes is defined by ONE file kept with its sources —
`marketplace.yaml` — and published by ONE verb from a build:

```
ktw-compose marketplace build <marketplace.yaml> --version <v> [--dist dist] [--reinit] [--dry-run]
```

A `marketplace-project` node holds where the definition is (`definition`), the
`version` and the build (`dist`); its actions **Check** (dry run), **Publish**
and **Start over** (reinit) run that verb.

## 1. The definition

```yaml
name: acme                                    # what subscribers call it
output: ${{env:OneDrive}}/Shared/marketplace-acme    # the published folder
mirrors: [D:/copies/marketplace-acme]         # copies of it (optional)
key: {folder: keys, prefix: acme}             # keys/acme.key — NEVER committed
keep: 3                                       # versions kept per item
platforms: [windows-amd64, darwin-arm64]      # the builds it publishes
config_sets: [{path: config-sets/base}]
seeds: [{path: projects/sample}]
vaults:                                       # keys sealed at publish
  - {name: team-keys, env_file: .env, keys: config-sets/base/keys.yaml, password_env: TEAM_VAULT_PASSWORD}
solutions:                                    # each a scenario case
  - {case: scenarios/demo, config_sets: [base], vaults: [team-keys]}
demo: demo                                    # what a fresh install opens on
scenario_root: D:/TestProjects                # where the cases run
```

Paths are relative to the file; `${{env:VAR}}` reads a variable (an unset one
refuses). No secret is ever written in it: the private key is a gitignored file,
a vault names the variable its password is read from.

## 2. What the verb does

1. **Checks everything first** and writes nothing while one fails: the key, the
   build holding exactly the definition's platforms, every folder, every case,
   every vault's `.env` and password (`--dry-run` stops here and lists what would
   be published).
2. `--reinit` wipes the published tree (subscribers' pinned versions stop
   resolving — bump the version first).
3. Publishes, at the version: the sidecars of the build, the infra channel
   (naming the demo), each config-set and seed, each vault (sealed from its
   `.env`), each solution (its case installed if needed, its binaries taken from
   this build so its members are pinned by content, its documents its `docs/`).
4. Verifies the tree as a consumer will, then copies it to each mirror.

`make release-reinit` in ktw-agents builds every platform into `dist/`, then runs
the verb for each definition `MARKETPLACES` lists, at `RELEASE_VERSION`.

## 3. What a consumer gets

The installer in `infra/` installs katyoti, which runs `katyoti setup`: the
control plane from the channel, the channel pinned to the key the pointer names
(`key:` in `infra/channel.yaml`), the catalog beside it subscribed under the
marketplace's own name, and the demo solution installed.

## 4. Rules

- A published version is immutable: cut a new one rather than republish.
- Keep the same key once published: subscribers pin it. Make one with
  `ktw-compose keygen --folder keys --prefix <name>` and gitignore
  `keys/*.key`.
- Remove a version with `remove-version <item>@<v>`, never by deleting files: it
  re-signs and refuses one a solution still pins.
