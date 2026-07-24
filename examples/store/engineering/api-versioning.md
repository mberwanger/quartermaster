---
id: eng.api-versioning
title: Versioning a public API
description: Add fields, never repurpose them. A breaking change needs a new version, and the old one stays until its consumers are gone.
domain: engineering
type: policy
status: active
provenance: decided
owner: platform
tags: [api, compatibility]
---

# Versioning a public API

This document declares no scope, so a ruleset that selects it makes it load in
every session. That is the right choice here: it applies to any change that
touches a contract, whatever language the repository is written in.

## Adding to an API

Add fields. Never repurpose an existing one, and never tighten what a field
accepts, because a consumer you cannot see is already sending the old shape.

## Breaking an API

A breaking change gets a new version. The previous version keeps working until
its consumers are gone, and "gone" means measured, not assumed.