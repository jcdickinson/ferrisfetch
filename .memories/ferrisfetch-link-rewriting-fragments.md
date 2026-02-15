---
title: Link Rewriting & Fragment Documents Implementation
description: Details of the on-the-fly markdown link rewriting and fragment sub-document generation feature
tags:
  feature: link-rewriting
  project: ferrisfetch
created: 2026-02-14T18:10:23.52886796-08:00
modified: 2026-02-14T18:48:24.384277862-08:00
---

# Link Rewriting & Fragment Documents

## What Was Built

On-the-fly rewriting of rustdoc intra-doc links to `rsdoc://` URIs, plus generation of fragment sub-documents (#fields, #variants, #impl, #implementations).

## Key Design Decisions

- **CAS stores original unchanged markdown** — hash stability preserved
- **Items store `doc_links TEXT`** — JSON map: markdown target → `rsdoc://` URI
- **Rewriting is on-the-fly**: same CAS content can have different link targets across crate versions
- **Fragment types**: struct→#fields, enum→#variants, trait→#impl, all→#implementations

## URI Format

- Local items (crate_id==0): `rsdoc://crateName/version/full::path`
- External deps: looks up `crate.ExternalCrates[strconv.Itoa(crateID)]`, uses `rsdoc://depName/latest/full::path`

## Version Resolution

- `addCrate` resolves "latest" to real versions via rustdoc JSON's `CrateVersion` field
- Real version stored in DB, used in generated URIs
- Version cache (10min TTL) avoids repeated docs.rs lookups
- Singleflight deduplicates concurrent fetches for same crate@version

## Auto-Fetch

- `handleGetDoc` auto-fetches unindexed crates via `resolveOrFetchCrate`
- Falls back to `addCrate` if crate not in DB
- 404s are cached to avoid spamming docs.rs