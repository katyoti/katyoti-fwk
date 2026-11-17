# KaTyoti

KaTyoti is a platform for running teams of AI agents on your own machine: a hub, the
sidecar programs that give the agents their tools and services, a task engine, and a
desktop application.

> **0.x preview.** The interfaces change between versions until 1.0.0. This
> repository is a **read-only mirror**: it receives the source of each published
> release. Contributions open with the `katyoti/katyoti` repository at version 0.11.0
> (see [CONTRIBUTING.md](CONTRIBUTING.md)); pull requests here are closed
> automatically. Report a vulnerability to security@katyoti.ai
> ([SECURITY.md](SECURITY.md)).

## Build

Needs Go (the version in `go.mod`), bash (Git Bash on Windows), bun for the
TypeScript parts, and the Wails CLI for the desktop application. On Linux the build
uses cgo for the voice player: it needs gcc and the ALSA headers
(`sudo apt-get install gcc libasound2-dev` on Debian and Ubuntu).

```sh
make build          # the Go programs and ktw-display, into bin/
make ktw-gui        # the desktop application (Wails)
```

## Test

```sh
make vet            # go vet ./...
make test-quick     # go test -short, the quick lane
make test           # every Go test
```

## Licence

Apache License 2.0 - see [LICENSE](LICENSE) and [NOTICE](NOTICE).
