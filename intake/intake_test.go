package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Synthetic blocked words only: this repo is public and the real list comes
// from SSM at runtime.
var blocked = []string{"badword", "Ofensivo", "pendejo", "  ", ""}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "jane@example.com", want: "jane@example.com"},
		{in: "  Jane.Doe@Example.COM \n", want: "jane.doe@example.com"},
		{in: "a+tag@sub.example.mx", want: "a+tag@sub.example.mx"},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "jane.example.com", wantErr: true},     // no @
		{in: "jane@@example.com", wantErr: true},    // two @
		{in: "jane@ex@ample.com", wantErr: true},    // two @
		{in: "@example.com", wantErr: true},         // empty local part
		{in: "jane@", wantErr: true},                // empty domain
		{in: "jane@localhost", wantErr: true},       // domain without a dot
		{in: "jane@example.", wantErr: true},        // empty TLD
		{in: "jane@.com", wantErr: true},            // empty label
		{in: "jane@example..com", wantErr: true},    // empty label
		{in: "jane doe@example.com", wantErr: true}, // inner space
		{in: "jane@exa mple.com", wantErr: true},    // inner space
		{in: "jane\t@example.com", wantErr: true},   // inner tab
	}
	for _, c := range cases {
		got, err := NormalizeEmail(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizeEmail(%q) = %q, want error", c.in, got)
			} else if !errors.Is(err, ErrInvalidEmail) {
				t.Errorf("NormalizeEmail(%q) error = %v, want ErrInvalidEmail", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q, nil", c.in, got, err, c.want)
		}
	}
}

func TestCleanName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Ana", "Ana"},
		{"trim", "  Ana María  ", "Ana María"},
		{"empty", "", ""},
		{"only spaces", "   \t\n ", ""},
		{"collapse whitespace", "Ana \t\n  María", "Ana María"},
		{"control chars stripped", "An\x00a\x07 Ma​ría\x1b", "Ana María"},
		{"html tags stripped", "<b>Ana</b> <script>alert(1)</script>María", "Ana alert(1)María"},
		{"lone angle bracket kept", "Ana <3", "Ana <3"},
		{"blocked whole word", "badword", "[redacted]"},
		{"blocked case insensitive", "Ana BADWORD", "Ana [redacted]"},
		{"blocked list entry casing ignored", "ofensivo", "[redacted]"},
		{"blocked accent insensitive", "Pendéjo López", "[redacted] López"},
		{"blocked accent and case", "PENDÉJO", "[redacted]"},
		{"hyphen separator kept", "Ana-badword", "Ana-[redacted]"},
		{"apostrophe separator kept", "O'badword", "O'[redacted]"},
		{"typographic apostrophe separator kept", "O’badword", "O’[redacted]"},
		{"several words", "badword ofensivo-Ana", "[redacted] [redacted]-Ana"},
		{"substring not redacted (prefix)", "Badwordson", "Badwordson"},
		{"substring not redacted (suffix)", "Mybadword", "Mybadword"},
		{"substring not redacted (scunthorpe)", "Pendejón", "Pendejón"},
		{"accents kept in clean words", "José Núñez", "José Núñez"},
		{"html then redaction", "<i>badword</i>", "[redacted]"},
		{"cap 50 ascii", strings.Repeat("a", 60), strings.Repeat("a", 50)},
		{"cap 50 multibyte", strings.Repeat("ñ", 60), strings.Repeat("ñ", 50)},
		{"cap 50 runes not bytes", strings.Repeat("é", 50), strings.Repeat("é", 50)},
		{"cap leaves no trailing space", strings.Repeat("a", 49) + " b", strings.Repeat("a", 49)},
		// The cap applies before redaction: a blocked word that fits is still
		// redacted and the output may exceed 50 runes.
		{"redaction after cap", strings.Repeat("a", 42) + " badword", strings.Repeat("a", 42) + " [redacted]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CleanName(c.in, blocked); got != c.want {
				t.Errorf("CleanName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCleanNameNilBlockedList(t *testing.T) {
	if got := CleanName(" badword ", nil); got != "badword" {
		t.Errorf("got %q, want %q", got, "badword")
	}
}

func TestNameCleanerReusable(t *testing.T) {
	c := NewNameCleaner(blocked)
	for i := 0; i < 2; i++ {
		if got := c.Clean("Ana Ofénsivo"); got != "Ana [redacted]" {
			t.Errorf("run %d: got %q", i, got)
		}
	}
}

func TestCleanNameCapIsRunes(t *testing.T) {
	got := CleanName(strings.Repeat("ü", 80), nil)
	if n := utf8.RuneCountInString(got); n != MaxNameRunes {
		t.Errorf("rune count = %d, want %d", n, MaxNameRunes)
	}
	if !utf8.ValidString(got) {
		t.Error("cap split a multibyte rune")
	}
}

func TestNormalizeCampaign(t *testing.T) {
	cases := map[string]string{
		"":                                  "none",
		"   ":                               "none",
		"xmas-2026":                         "xmas-2026",
		"  XMAS-2026 ":                      "xmas-2026",
		"a":                                 "a",
		strings.Repeat("a", 32):             strings.Repeat("a", 32),
		strings.Repeat("a", 33):             "invalid",
		"xmas_2026":                         "invalid",
		"xmas 2026":                         "invalid",
		"navidad-ñ":                         "invalid",
		"<script>":                          "invalid",
		"none":                              "none",
		"invalid":                           "invalid",
		"x\n":                               "x",
		"---":                               "---",
		"utm_source=ig":                     "invalid",
		strings.Repeat("AB", 16):            strings.Repeat("ab", 16), // folds before the length check
		" " + strings.Repeat("a", 32) + " ": strings.Repeat("a", 32),  // trim before the length check
	}
	for in, want := range cases {
		if got := NormalizeCampaign(in); got != want {
			t.Errorf("NormalizeCampaign(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdult(t *testing.T) {
	mx, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	dob := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, mx) }

	cases := []struct {
		name string
		dob  time.Time
		now  time.Time
		want bool
	}{
		{"18th birthday itself", dob(2008, 11, 20), at(2026, 11, 20, 0), true},
		{"day before 18th birthday", dob(2008, 11, 20), at(2026, 11, 19, 23), false},
		{"day after 18th birthday", dob(2008, 11, 20), at(2026, 11, 21, 12), true},
		{"well over 18", dob(1980, 1, 1), at(2026, 11, 20, 12), true},
		{"17", dob(2009, 6, 1), at(2026, 11, 20, 12), false},
		{"newborn", dob(2026, 11, 20), at(2026, 11, 20, 12), false},
		{"dob in the future", dob(2030, 1, 1), at(2026, 11, 20, 12), false},
		{"dob far in the future", dob(2100, 1, 1), at(2026, 11, 20, 12), false},
		{"zero dob is old", time.Time{}, at(2026, 11, 20, 12), true},
		// Feb 29 birthdays: in a non-leap year the birthday is reached on Mar 1.
		{"feb29 on feb28 non-leap", dob(2008, 2, 29), at(2026, 2, 28, 23), false},
		{"feb29 on mar1 non-leap", dob(2008, 2, 29), at(2026, 3, 1, 0), true},
		// now is read on the Mexico City calendar, not UTC.
		{"utc already birthday, CDMX day before", dob(2008, 11, 20), time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC), false},
		{"utc birthday, CDMX birthday", dob(2008, 11, 20), time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Adult(c.dob, c.now); got != c.want {
				t.Errorf("Adult(%s, %s) = %v, want %v", c.dob.Format(time.DateOnly), c.now.Format(time.RFC3339), got, c.want)
			}
		})
	}
}

func TestAdultLeapDayBirthdayInLeapYear(t *testing.T) {
	mx, _ := time.LoadLocation("America/Mexico_City")
	dob := time.Date(2008, 2, 29, 0, 0, 0, 0, time.UTC)
	if Adult(dob, time.Date(2026, 2, 28, 12, 0, 0, 0, mx)) {
		t.Error("2008-02-29 is not 18 on 2026-02-28")
	}
	// 2028 is a leap year: 20th birthday on Feb 29 itself; 18 turned in 2026.
	if !Adult(dob, time.Date(2028, 2, 29, 12, 0, 0, 0, mx)) {
		t.Error("2008-02-29 is an adult on 2028-02-29")
	}
	// Born 2012-02-29 turns 18 in non-leap 2030, on Mar 1.
	dob = time.Date(2012, 2, 29, 0, 0, 0, 0, time.UTC)
	if Adult(dob, time.Date(2030, 2, 28, 23, 0, 0, 0, mx)) {
		t.Error("2012-02-29 is not 18 on 2030-02-28")
	}
	if !Adult(dob, time.Date(2030, 3, 1, 0, 0, 0, 0, mx)) {
		t.Error("2012-02-29 is 18 on 2030-03-01")
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"jane@example.com":   "j***@example.com",
		"j@example.com":      "j***@example.com",
		"ñandú@example.mx":   "ñ***@example.mx",
		" jane@example.com ": "j***@example.com",
		"@example.com":       "***",
		"no-at-sign":         "***",
		"":                   "***",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmailHash(t *testing.T) {
	sum := sha256.Sum256([]byte("jane@example.com"))
	want := hex.EncodeToString(sum[:])[:12]

	for _, in := range []string{"jane@example.com", "  JANE@Example.com "} {
		if got := EmailHash(in); got != want {
			t.Errorf("EmailHash(%q) = %q, want %q", in, got, want)
		}
	}

	// Invalid emails hash the trimmed, lowercased input.
	sum = sha256.Sum256([]byte("not an email"))
	if got, want := EmailHash("  Not An Email "), hex.EncodeToString(sum[:])[:12]; got != want {
		t.Errorf("EmailHash(invalid) = %q, want %q", got, want)
	}
	if got := EmailHash("x"); len(got) != 12 {
		t.Errorf("len = %d, want 12", len(got))
	}
}
