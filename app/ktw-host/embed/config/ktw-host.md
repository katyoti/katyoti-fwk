# ktw-host

The generic host for **files-only sidecars** (ADR §12). Run it against a
folder that contains a `ktw-host.yaml` (identity + directory layout) plus
contribution sub-folders; the host adopts the manifest's identity at runtime
and serves the folder's contributions under that name — no Go build.

    ktw-host --folder <path>  sidecar resources list
    ktw-host --folder <path>  sidecar skills get-instructions <provider>:/skill/<name>

`ktw-host.yaml`:

    name: acme-flows             # runtime identity (provider / KTSP slot / config name)
    displayName: "Acme Flows"
    description: "…"
    dirs:                        # optional: remap a type to a custom subdir
      references: guides

Conventional sub-folders are served automatically when present:
`skills/ tasks/ references/ personas/ data-types/ scripts/ cwm/`. Use `dirs:`
to point a type at a non-conventional directory, or set a type to `""` to
disable it.

Declare the sidecar's own config needs so `sidecar config check` (and
`katyoti configure`) drive it like any compiled sidecar:

    config:
      apiKeys: [ { envName: ACME_TOKEN, label: "Acme token", required: true } ]
      auth:    [ { name: example, loginHint: "ktw-example auth login" } ]
      tools:   [ { name: Graphviz, commands: [dot] } ]

Follow-up work: freeze-to-exe (embed a folder into a copy of `ktw-host`).
