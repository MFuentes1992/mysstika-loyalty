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

- `NormalizeEmail(s)`: trim + lowercase, `ErrInvalidEmail` unless it is a plain `local@domain.tld`: at most 254 bytes, local part 1–64 with no leading/trailing/doubled dot (`+` tags kept), ASCII domain labels of 1–63 letters/digits/hyphens, TLD of 2+ letters, and no whitespace, control characters or ``<>()[]\,;:"``. Not RFC 5322.
- `CleanName(s, blocked)` / `NewNameCleaner(blocked).Clean(s)`: bound the raw input to 200 runes, drop invalid UTF-8, replace HTML tags with a space, drop control characters, collapse whitespace, replace whole blocked words with `[redacted]`, then cap at 50 runes on a word boundary (never splitting `[redacted]`).
  - A word is a run of letters and marks; every other rune is a separator and is kept (`Ana-badword` → `Ana-[redacted]`, `badword!` → `[redacted]!`). Substrings never match (`Badwordson` stays).
  - Matching ignores case, fullwidth forms and accents, but `ñ` stays distinct from `n`. Leetspeak (`b4dword`) and in-word splits (`bad_word`) are out of scope.
  - Blocked entries must be single words; entries with a separator are ignored and listed by `Skipped()` so the service can log them.
  - **The output is not HTML-safe.** Escape it wherever it is rendered.
- `NormalizeCampaign(s)`: `none` when empty, the tag when it matches `^[a-z0-9-]{1,32}$`, otherwise `invalid`.
- `Adult(dob, now)`: 18+ on `now`'s date in `America/Mexico_City` (embedded tzdata). Pass `dob` as a date-only value; Feb 29 birthdays are reached on Mar 1 in non-leap years. Fails closed: a zero or future `dob` is not adult, so callers only call it when a dob was given.
- `MaskEmail(e)` and `EmailHash(e)`: log helpers (`j***@example.com`; 12 hex chars of an unsalted SHA-256, a pseudonym anyone with a candidate email can match, so treat it as personal data).

The blocked-word list is never committed here: services load it from SSM and pass it in. Tests use made-up words only.

Versioned with git tags (`vX.Y.Z`); consumers pin a tag in `go.mod`.

Test fixtures are synthetic. This repo is public, so never commit real customer data.
