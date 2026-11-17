# ktw-hub

The KTSP bus host. Embeds a NATS server (desktop / dev) or connects to an
external NATS cluster (cloud); provides ConnectionManager, Catalog, and
shared-state services (rooms / sessions / journal / presence) for ONE
active universe.

Not a user-facing day-to-day binary. Spawned by ktw-gui / ktw-agents via
the hubclient library. Direct invocation is for headless / systemd / k8s
deploys.

## Composes modules

| Module | Role |
|---|---|
| [bus](../../modules/bus/) | Hub + NATS + ConnMgr + lazy spawn |
| [catalog](../../modules/contributions/) | CatalogService + tier-1 cache |
| [framework](../../modules/framework/) | Bootstrap |

## See also

- [Which binary hosts the bus](../../modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md)
- [What carries the messages](../../modules/bus/decisions/OD-001-what-carries-the-messages-between-the-hub-the.cwm.md) — NATS
- [How subjects are scoped](../../modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md)
