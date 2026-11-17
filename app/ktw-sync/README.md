# ktw-sync

Multi-provider cloud sync sidecar — Google Drive, S3, OneDrive, SFTP,
WebDAV. Job-based + workspace-scoped + rclone-backed.

## Composes modules

| Module | Role |
|---|---|
| [sync](../../modules/sync/) | Domain logic |
| [framework](../../modules/framework/) | Bootstrap |
| [catalog](../../modules/contributions/) | Manifest |
| [skills](../../modules/skills/) | Sync-related skills |
| [scripts](../../modules/scripts/) | Skill scripts |

## External dependencies

- `rclone` (required) — must be on PATH.

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go).
