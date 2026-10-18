package loyalty

import "testing"

func TestTierFor(t *testing.T) {
	cases := map[int]string{
		0:  "Nivel 1", // absent currentCycle on legacy passes
		1:  "Nivel 1",
		2:  "Nivel 2",
		3:  "Nivel 2",
		4:  "Nivel 3",
		57: "Nivel 3",
	}
	for cycle, want := range cases {
		if got := TierFor(cycle); got != want {
			t.Errorf("TierFor(%d) = %q, want %q", cycle, got, want)
		}
	}
}

func TestTierGroupsAscendingFromFirstCycle(t *testing.T) {
	if len(tierGroups) == 0 || tierGroups[0].MinCycle != FirstCycle {
		t.Fatalf("tierGroups must start at FirstCycle so every cycle has a tier")
	}
	for i := 1; i < len(tierGroups); i++ {
		if tierGroups[i].MinCycle <= tierGroups[i-1].MinCycle {
			t.Errorf("tierGroups[%d].MinCycle %d is not above %d", i, tierGroups[i].MinCycle, tierGroups[i-1].MinCycle)
		}
	}
}
