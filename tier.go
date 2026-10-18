// Package loyalty holds the Mysstika Rewards program rules shared by the
// Lambdas, so each service renders the same tier for the same cycle.
package loyalty

// FirstCycle is the cycle every customer starts in.
const FirstCycle = 1

type tierGroup struct {
	MinCycle int
	Name     string
}

// tierGroups is ascending by MinCycle and starts at FirstCycle. The names are
// temporary (LOY-06). The tier is computed on read and never stored in
// DynamoDB, but Google objects store the rendered name, so renaming a tier
// means re-patching them.
var tierGroups = []tierGroup{
	{MinCycle: 1, Name: "Nivel 1"},
	{MinCycle: 2, Name: "Nivel 2"},
	{MinCycle: 4, Name: "Nivel 3"},
}

// TierFor returns the tier name for a cycle. Cycles below FirstCycle (a legacy
// pass with no currentCycle reads as 0) get the first tier.
func TierFor(cycle int) string {
	name := tierGroups[0].Name
	for _, g := range tierGroups {
		if cycle >= g.MinCycle {
			name = g.Name
		}
	}
	return name
}
