---
name: test-ktsp-stdio
description: "Test the KTSP wire protocol itself: JSON-RPC over stdio against a sidecar serving 'sc ktsp stdio'. Load when the question is framing, slot routing, or what a remote consumer receives on the wire."
advertised: false
hashtags: [testing, ktsp, protocol, wire]
category: guide
---

# Stdio method-call probe

The lowest-level truth: what a bus/remote consumer actually receives over the
stdio method-call transport.

## Drive it

Start the binary serving the protocol and pipe JSON-RPC requests, one per
line in, one NDJSON response per line out:

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}\n' \
  | $BIN sc ktsp stdio 2>/dev/null | head -5
```

Method names are the slot methods (`resources/list`, the resources read method,
`<slot>/<method>` for published services).

## What it answers

- the envelope shape a polyglot/remote consumer must parse
- slot routing: does `<slot>/<method>` reach the right handler
- error shapes: invalid params must return a JSON-RPC error, not a hang
- regression after protocol-layer changes (encode/decode, ctx threading)

## Rules

- Responses must arrive even for errors — a silent hang is always a defect.
- Compare against the CLI twin (call the same `<uri>` tool with JSON args): same
  method, same JSON result. Divergence = wire bug.
- This level is for protocol questions only — for surface checks use
  `ktw-compose:/skill/test-cli-probe`; it is the same data with less effort.
