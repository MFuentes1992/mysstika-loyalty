// Package segments computes the Mysstika marketing segments. It is a port of
// the `segments` command in mysstika-ops customers.sh and must stay
// output-compatible with it (see the golden test).
package segments

import (
	"sort"
	"time"
)

// Segment thresholds. The stamp thresholds are the reward codes (5 = 10%,
// 8 = 15%, 10 = 20%).
const (
	RewardReadyStamps = 10
	NearRewardStamps  = 8
	MidCycleStamps    = 5

	NewCustomerWindow = 30 * 24 * time.Hour
	StaleWindow       = 45 * 24 * time.Hour
)

// timestampLayout is how customers.sh formats its cutoffs. Timestamps are
// compared as strings, exactly like the jq filter does.
const timestampLayout = "2006-01-02T15:04:05Z"

// Customer is a customer joined with their pass: stamps and the last punch
// time (empty when the customer has never been punched).
type Customer struct {
	Email        string `json:"email"`
	FirstName    string `json:"firstName"`
	BirthDay     string `json:"birthDay"`
	CreatedAt    string `json:"createdAt"`
	WelcomeEmail bool   `json:"welcomeEmail"`
	Stamps       int    `json:"stamps"`
	LastPunch    string `json:"lastPunch"`
}

// StampEntry is a customer in a stamp- or punch-based segment.
type StampEntry struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	Stamps    int    `json:"stamps"`
	LastPunch string `json:"lastPunch"`
}

// BirthdayEntry is a customer in birthdays_this_month.
type BirthdayEntry struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	BirthDay  string `json:"birthDay"`
}

// SignupEntry is a customer in new_last_30d or never_punched.
type SignupEntry struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	CreatedAt string `json:"createdAt"`
}

// ContactEntry is a customer in missing_welcome.
type ContactEntry struct {
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
}

// Report holds every segment. Each list is ordered by createdAt.
type Report struct {
	Total              int             `json:"total"`
	RewardReady        []StampEntry    `json:"reward_ready"`
	NearReward         []StampEntry    `json:"near_reward"`
	MidCycle           []StampEntry    `json:"mid_cycle"`
	BirthdaysThisMonth []BirthdayEntry `json:"birthdays_this_month"`
	NewLast30d         []SignupEntry   `json:"new_last_30d"`
	Stale45d           []StampEntry    `json:"stale_45d"`
	NeverPunched       []SignupEntry   `json:"never_punched"`
	MissingWelcome     []ContactEntry  `json:"missing_welcome"`
}

// Build computes every segment for customers as of now. A customer can be in
// several segments.
func Build(customers []Customer, now time.Time) Report {
	sorted := append([]Customer(nil), customers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt < sorted[j].CreatedAt })

	now = now.UTC()
	month := now.Format("01")
	newCutoff := now.Add(-NewCustomerWindow).Format(timestampLayout)
	staleCutoff := now.Add(-StaleWindow).Format(timestampLayout)

	r := Report{
		Total:              len(sorted),
		RewardReady:        []StampEntry{},
		NearReward:         []StampEntry{},
		MidCycle:           []StampEntry{},
		BirthdaysThisMonth: []BirthdayEntry{},
		NewLast30d:         []SignupEntry{},
		Stale45d:           []StampEntry{},
		NeverPunched:       []SignupEntry{},
		MissingWelcome:     []ContactEntry{},
	}
	for _, c := range sorted {
		stamp := StampEntry{Email: c.Email, FirstName: c.FirstName, Stamps: c.Stamps, LastPunch: c.LastPunch}
		signup := SignupEntry{Email: c.Email, FirstName: c.FirstName, CreatedAt: c.CreatedAt}

		switch {
		case c.Stamps >= RewardReadyStamps:
			r.RewardReady = append(r.RewardReady, stamp)
		case c.Stamps >= NearRewardStamps:
			r.NearReward = append(r.NearReward, stamp)
		case c.Stamps >= MidCycleStamps:
			r.MidCycle = append(r.MidCycle, stamp)
		}
		if birthMonth(c.BirthDay) == month {
			r.BirthdaysThisMonth = append(r.BirthdaysThisMonth, BirthdayEntry{Email: c.Email, FirstName: c.FirstName, BirthDay: c.BirthDay})
		}
		if c.CreatedAt > newCutoff {
			r.NewLast30d = append(r.NewLast30d, signup)
		}
		if c.LastPunch == "" {
			r.NeverPunched = append(r.NeverPunched, signup)
		} else if c.LastPunch < staleCutoff {
			r.Stale45d = append(r.Stale45d, stamp)
		}
		if !c.WelcomeEmail {
			r.MissingWelcome = append(r.MissingWelcome, ContactEntry{Email: c.Email, FirstName: c.FirstName})
		}
	}
	return r
}

// birthMonth returns characters 5-7 of a YYYY-MM-DD birthday, like jq's
// .birthDay[5:7], clamped for short values.
func birthMonth(birthDay string) string {
	if len(birthDay) <= 5 {
		return ""
	}
	return birthDay[5:min(7, len(birthDay))]
}
