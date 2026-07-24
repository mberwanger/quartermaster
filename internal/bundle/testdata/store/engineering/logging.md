---
id: eng.logging
title: Structured logging
description: Log as structured key-value pairs, never interpolated strings.
domain: engineering
type: concept
status: active
provenance: verified
owner: platform
scope: ["**/*.go"]
tags: [go, observability]
---

# Structured logging

## Rule

Emit logs as structured key-value pairs; never build a message by string interpolation.
