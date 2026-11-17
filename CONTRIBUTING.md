# Contributing

Thank you for your interest in KaTyoti.

## The repository is read-only until katyoti 0.11.0

`katyoti/katyoti-fwk` is a published mirror: it receives the source of a KaTyoti
release, made by the release export and nothing else. Until the `katyoti/katyoti`
repository opens at version 0.11.0, it takes no contribution:

- issues, discussions, the wiki and projects are off;
- a pull request is closed automatically, with a message saying that contributions
  open with the `katyoti` repository at 0.11.0;
- only the maintainer merges, in every KaTyoti repository.

## Reporting a vulnerability

Send vulnerability reports to **security@katyoti.ai**, never as a public issue. See
[SECURITY.md](SECURITY.md).

## When contributions open

From the `katyoti` repository on, contributions are made under the
[Developer Certificate of Origin](https://developercertificate.org/) (DCO): each
commit carries a `Signed-off-by:` line (`git commit -s`) certifying that you wrote the
change or have the right to submit it under the project's licence. They are accepted
under the [Apache License 2.0](LICENSE), the licence of the project.

## Building and testing

The build needs Go (the version in `go.mod`) and, for the TypeScript parts, bun.

```sh
make build        # the Go programs and ktw-display, into bin/
make test-quick   # the quick test lane (go test -short)
make vet          # go vet ./...
```

The tests and scripts run in bash (Git Bash on Windows).
