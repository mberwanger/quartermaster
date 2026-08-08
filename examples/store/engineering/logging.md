---
id: eng.logging
title: Structured logging
description: Log key-value pairs, never interpolated prose. Log the identifiers that let someone find the record, never the record itself.
domain: engineering
type: reference
status: active
provenance: verified
owner: platform
scope: ["**/*.go"]
tags: [go, observability]
sources:
  - repo: example-service
    path: internal/obs/log.go
---

# Structured logging

Scoped to Go here, but the `incident-review` package overrides that scope to
nothing, so a repository that reviews incidents loads it in every session. Same
document, same text, different residency — the consumer decides.

## Log pairs, not prose

```go
slog.Info("charge settled", "account", id, "amount_cents", cents)
```

Interpolating the values into a sentence makes the log unsearchable, which is
the only thing a log is for.

## Log identifiers, not records

Log the account id, not the account. Someone with the id can fetch the record
and is allowed to; the log is read by people who are not.