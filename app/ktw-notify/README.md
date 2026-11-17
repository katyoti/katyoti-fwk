# ktw-notify

Notification sidecar — routes a notification by kind to voice (TTS) and desktop-toast channels.

## Composes modules

| Module | Role |
|---|---|
| [notify](../../modules/notify/) | Domain logic |
| [framework](../../modules/framework/) | Bootstrap |
| [catalog](../../modules/contributions/) | Manifest |
| [skills](../../modules/skills/) | Notification-related skills |
| [scripts](../../modules/scripts/) | Skill scripts |

## Configuration

`NotifyService.yaml` maps kinds to profiles and profiles to channels;
`TTSClient.yaml`, `TTSVoices.yaml` and `TTSPronunciations.yaml` configure
the voice engine (seeded from [embed/config/](embed/config/)).

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go).
