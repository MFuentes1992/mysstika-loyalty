# mysstika-loyalty

Program rules for Mysstika Rewards, shared by the `mysstika-rewards` Lambdas so every service computes the same numbers.

- `TierFor(cycle)`: the tier name for a cycle (cycle 1 → Nivel 1, 2–3 → Nivel 2, 4+ → Nivel 3; names are temporary).

```go
import "github.com/MFuentes1992/mysstika-loyalty"

loyalty.TierFor(loyalty.FirstCycle) // "Nivel 1"
```

Versioned with git tags (`vX.Y.Z`); consumers pin a tag in `go.mod`.
