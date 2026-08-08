---
id: billing.money-representation
title: Representing money
description: Money is an integer of minor units plus a currency. Never a float, never a bare number, and never compared across currencies.
domain: billing
type: policy
status: active
provenance: decided
owner: billing
tags: [money, correctness]
---

# Representing money

No scope, so this loads in every session of a repository that takes the
`billing` package — and in no session of a repository that does not.

## Integer minor units, always

Money is an integer count of minor units and a currency code. `1250` and `USD`,
never `12.50`. A float cannot hold a tenth of a cent and will not tell you when
it stops trying.

## Currency travels with the amount

A bare number is not money. If a function takes an amount without a currency, it
is wrong for someone, and that someone is usually in another country.