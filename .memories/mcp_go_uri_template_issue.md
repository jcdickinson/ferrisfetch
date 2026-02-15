---
title: 'mcp-go URI Template :: Matching Issue'
description: 'Why rsdoc://{path} fails with :: characters'
tags:
  issue: true
  solution: true
created: 2026-02-14T23:10:13.710749343-08:00
modified: 2026-02-14T23:10:13.710749343-08:00
---

# mcp-go URI Template Matching Issue: Why `::` Fails

## Root Cause
Template `rsdoc://{crate}/{version}/{path}` uses **simple expressions** (`{path}` = no operator prefix).

RFC 6570 simple expressions only match **unreserved characters**:
- ✓ Allowed: `- . 0-9 A-Z _ a-z ~`
- ✗ Not allowed: `:` (colon is a reserved character)

The uritemplate library correctly implements RFC 6570:
- Generated regex for `{path}`: `(?:(?:[,\x2d\x2e\x30-\x39\x41-\x5a\x5f\x61-\x7a\x7e]|%[[:xdigit:]][[:xdigit:]])*)` 
- Colon (0x3A) is NOT included in the character class
- `jj_lib::git::GitFetch` fails at first `:`

## Code Path
1. `handleReadResource()` @ `/server/server.go:948`
2. `matchesTemplate()` @ line 1017, 1039
3. `template.URITemplate.Regexp().MatchString(uri)` → uritemplate regex match fails

## Solution: Use Reserved Expansion `{+path}`
```go
mcp.NewResourceTemplate(
    "rsdoc://{crate}/{version}/{+path}",
    "Rust documentation item",
)
```
The `+` operator allows both reserved and unreserved chars without percent-encoding.

## Why It's Not a Bug
- RFC 6570 Section 3.2: simple expressions exclude reserved chars
- `+` operator (Section 3.2.2) allows reserved chars
- Library correctly implements the standard
