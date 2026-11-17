# mysstika-loyalty

Program rules for Mysstika Rewards, shared by the `mysstika-rewards` Lambdas so every service computes the same numbers.

- `TierFor(cycle)`: the tier name for a cycle (cycle 1 → Nivel 1, 2–3 → Nivel 2, 4+ → Nivel 3; names are temporary).
- `segments.Build(customers, now)`: the 8 marketing segments, a port of `customers.sh segments` that must stay output-compatible with it. `segments/testdata/gen-golden.sh` regenerates the golden file with the original jq filter.

```go
import "github.com/MFuentes1992/mysstika-loyalty"

loyalty.TierFor(loyalty.FirstCycle) // "Nivel 1"
```

## intake

Enrollment intake rules, shared by the self-enrollment portal and teller enrollment so both accept the same input (`github.com/MFuentes1992/mysstika-loyalty/intake`):

- `NormalizeEmail(s)`: trim + lowercase, `ErrInvalidEmail` unless shaped `local@domain.tld` (not RFC 5322).
- `CleanName(s, blocked)` / `NewNameCleaner(blocked).Clean(s)`: strip HTML tags and control characters, collapse whitespace, cap at 50 runes, then replace whole blocked words with `[redacted]`. Matching ignores case and accents; words split on spaces, hyphens and apostrophes, which are kept (`Ana-badword` → `Ana-[redacted]`); substrings never match. The cap runs before redaction, so a redacted name can exceed 50 runes.
- `NormalizeCampaign(s)`: `none` when empty, the tag when it matches `^[a-z0-9-]{1,32}$`, otherwise `invalid`.
- `Adult(dob, now)`: 18+ on `now`'s date in `America/Mexico_City` (embedded tzdata; Feb 29 birthdays reach the age on Mar 1 in non-leap years).
- `MaskEmail(e)` and `EmailHash(e)`: log helpers (`j***@example.com`, 12 hex chars of SHA-256).

The blocked-word list is never committed here: services load it from SSM and pass it in. Tests use synthetic words only.

Versioned with git tags (`vX.Y.Z`); consumers pin a tag in `go.mod`.

Test fixtures are synthetic. This repo is public, so never commit real customer data.
