---
id: eng.error-handling
title: Wrapping and inspecting errors
description: Wrap with %w and a clause naming the operation. Match with errors.Is and errors.As, never on message text.
domain: engineering
type: reference
status: stable
owner: platform
scope: ["**/*.go"]
tags: [go, errors]
sources:
  - id: store
    resource: example-service/internal/store/store.go
    title: Error wrapping at the store boundary
---

# Wrapping and inspecting errors

This document declares `scope: ["**/*.go"]`, so it loads only when a Go file is
open. A repository full of Terraform pays nothing for it.

## Wrapping

Wrap with `%w` and a short clause naming the operation that failed:

```go
if err != nil {
    return fmt.Errorf("load account %s: %w", id, err)
}
```

The clause is the part a reader needs. `failed to do thing: failed to do thing`
tells them nothing twice.

## Inspecting

Match with `errors.Is` and `errors.As`. Never match on message text: the message
is for a human and will be reworded the moment it is unclear.