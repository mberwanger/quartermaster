---
id: eligibility.coverage-effective-dates
title: Coverage effective dates
description: Coverage is a half-open interval in the member's local date, and a claim is judged against the date of service rather than the date of submission.
domain: eligibility
type: policy
status: active
provenance: decided
owner: eligibility
tags: [dates, correctness]
---

# Coverage effective dates

This document exists to be *absent* from the billing example. A repository that
filters `knowledge.domain` to engineering and billing never sees this file at
all, and never takes the `eligibility` package that would make it a rule.

That is the point: one store can serve an organization without every repository
carrying all of it.

## Half-open intervals

Coverage runs from the effective date inclusive to the termination date
exclusive. Two adjacent spans must not both claim the boundary day.

## Judge on the date of service

A claim is judged against the date the care happened, not the date the claim
arrived. Those differ by weeks, and the coverage often differs with them.