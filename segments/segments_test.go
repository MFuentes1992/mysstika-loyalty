package segments

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"
)

// goldenNow matches the fixed "now" in testdata/gen-golden.sh.
var goldenNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// The Go port must produce exactly what customers.sh `segments` produces for the
// same input; testdata/gen-golden.sh regenerates the golden file with that filter.
func TestBuildMatchesCustomersShGolden(t *testing.T) {
	var customers []Customer
	readJSON(t, "testdata/joined.json", &customers)

	got, err := json.Marshal(Build(customers, goldenNow))
	if err != nil {
		t.Fatal(err)
	}
	var gotDoc, wantDoc any
	if err := json.Unmarshal(got, &gotDoc); err != nil {
		t.Fatal(err)
	}
	readJSON(t, "testdata/segments.golden.json", &wantDoc)

	if !reflect.DeepEqual(gotDoc, wantDoc) {
		want, _ := json.MarshalIndent(wantDoc, "", "  ")
		gotIndented, _ := json.MarshalIndent(gotDoc, "", "  ")
		t.Errorf("segments differ from customers.sh golden\n--- got\n%s\n--- want\n%s", gotIndented, want)
	}
}

func TestBuildEmptyHasEmptyListsNotNull(t *testing.T) {
	got, err := json.Marshal(Build(nil, goldenNow))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"total":0,"reward_ready":[],"near_reward":[],"mid_cycle":[],"birthdays_this_month":[],"new_last_30d":[],"stale_45d":[],"never_punched":[],"missing_welcome":[]}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestBuildDoesNotReorderInput(t *testing.T) {
	in := []Customer{
		{Email: "b@example.com", CreatedAt: "2026-02-01T00:00:00Z"},
		{Email: "a@example.com", CreatedAt: "2026-01-01T00:00:00Z"},
	}
	Build(in, goldenNow)
	if in[0].Email != "b@example.com" {
		t.Error("Build must not mutate the caller's slice")
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
