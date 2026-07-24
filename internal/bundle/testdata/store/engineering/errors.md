---
id: eng.errors
title: Error wrapping
description: Wrap errors with context using %w so callers can match them.
domain: engineering
type: concept
status: active
provenance: decided
owner: platform
tags: [go]
---

# Error wrapping

## Rule

Wrap errors with %w and a short context clause naming the operation that failed.