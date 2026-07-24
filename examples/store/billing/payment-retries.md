---
id: billing.payment-retries
title: Retrying a payment
description: Retry only what the processor called retryable, with an idempotency key that survives the retry. Never retry on a timeout without checking first.
domain: billing
type: runbook
status: active
provenance: verified
owner: billing
scope: ["internal/billing/**", "**/payment*.go"]
tags: [money, resilience]
sources:
  - repo: example-billing
    path: internal/billing/retry.go
---

# Retrying a payment

Scoped to the billing package and anything payment-shaped, so it loads when the
work is actually adjacent to it.

## Retry only what is retryable

A processor tells you whether a decline is permanent. A permanent decline
retried is a second decline plus a second fee.

## Carry the idempotency key through the retry

The key is generated once per payment attempt, not once per request. Generating
a fresh key on retry is how one payment becomes two charges.

## A timeout is not a failure

A timed-out charge may well have settled. Read the payment's state before
deciding anything; retrying blind is the other way one payment becomes two.
