# hotkey-server

`hotkey-server` is the Go backend for the HotKey personal creator AI hotspot monitoring product.

The repository is being rebooted around a standard Go service structure and Symphony-driven Linear workflow. The server is the future OpenAPI source of truth for Web and miniapp clients.

## Current Scope

- Email-first account system.
- Optional WeChat login when configuration is present.
- User keywords and preferences.
- System sources plus user RSS or public links.
- Content normalization, deduplication, similarity, hotspot scoring, AI summaries, and daily reports.

The first foundation phase only provides:

- Symphony `WORKFLOW.md`.
- Standard Go API skeleton.
- `GET /healthz`.
- Migration directory foundation.
- Test and run commands.

## Commands

```bash
make test
make run
HOTKEY_HTTP_ADDR=127.0.0.1:18080 make run
curl http://127.0.0.1:18080/healthz
```

## Workflow

All implementation work is tracked in Linear and orchestrated by Symphony. `WORKFLOW.md` is the repository-owned workflow contract.
