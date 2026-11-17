package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Made-up blocked words only: this repo is public and the real list comes from
// SSM at runtime. Blank entries must be ignored.
var blocked = []string{"badword", "Ofensivo", "zorgle", "  ", ""}

func TestNormalizeEmail(t *testing.T) {
	local64 := strings.Repeat("l", 64)
	domain189 := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 57) + ".com"
	label63 := strings.Repeat("d", 63)

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "jane@example.com", want: "jane@example.com"},
		{in: "  Jane.Doe@Example.COM \n", want: "jane.doe@example.com"},
		{in: "a+tag@sub.example.mx", want: "a+tag@sub.example.mx"},
		{in: "ja.ne+news.2026@sub-domain.example.com", want: "ja.ne+news.2026@sub-domain.example.com"},
		{in: "jane@" + label63 + ".com", want: "jane@" + label63 + ".com"},
		{in: local64 + "@" + domain189, want: local64 + "@" + domain189}, // 254 total
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
		{in: "jane @example.com", wantErr: true},
		{in: "ja\x00ne@example.com", wantErr: true}, // control character
		{in: "ja\x7fne@example.com", wantErr: true},
		{in: "jan\xffe@example.com", wantErr: true}, // invalid UTF-8
		{in: "jane\u200b@gmail.com", wantErr: true}, // zero-width space (Cf)
		{in: "ja\u00adne@gmail.com", wantErr: true}, // soft hyphen (Cf)
		{in: "jane@gmail.com\ufeff", wantErr: true}, // BOM (Cf)
		{in: "ñandú@example.mx", wantErr: true},     // non-ASCII local part
		{in: "jané@example.com", wantErr: true},
		{in: "<jane>@example.com", wantErr: true},
		{in: "jane(x)@example.com", wantErr: true},
		{in: "jane[x]@example.com", wantErr: true},
		{in: `ja\ne@example.com`, wantErr: true},
		{in: "jane,x@example.com", wantErr: true},
		{in: "jane;x@example.com", wantErr: true},
		{in: "jane:x@example.com", wantErr: true},
		{in: `"jane"@example.com`, wantErr: true},
		{in: local64 + "l@example.com", wantErr: true},                                                      // local 65
		{in: local64 + "@" + domain189[:len(domain189)-4] + "c.com", wantErr: true},                         // 255 total
		{in: ".jane@example.com", wantErr: true},                                                            // leading dot
		{in: "jane.@example.com", wantErr: true},                                                            // trailing dot
		{in: "ja..ne@example.com", wantErr: true},                                                           // doubled dot
		{in: "jane@" + label63 + "d.com", wantErr: true},                                                    // label 64
		{in: "jane@-example.com", wantErr: true},                                                            // label starts with -
		{in: "jane@example-.com", wantErr: true},                                                            // label ends with -
		{in: "jane@ex_ample.com", wantErr: true},                                                            // _ in domain
		{in: "jane@exámple.com", wantErr: true},                                                             // non-ASCII domain
		{in: "jane@example.c", wantErr: true},                                                               // 1-letter TLD
		{in: "jane@example.c0m", wantErr: true},                                                             // digit in TLD
		{in: "jane@example.123", wantErr: true},                                                             // numeric TLD
		{in: "jane@" + strings.Repeat("e", 10) + "." + strings.Repeat("m", 3), want: "jane@eeeeeeeeee.mmm"}, // control: valid
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
	a := func(n int) string { return strings.Repeat("a", n) }
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
		{"control and format chars stripped", "An\x00a\x07 Ma​ría\x1b", "Ana María"},
		{"invalid utf-8 dropped", "An\xffa Mar\xc3ía", "Ana María"},

		// HTML: each tag becomes one space; only <letter, </ and <! start a tag.
		{"html tags become spaces", "<b>Ana</b> <script>alert(1)</script>María", "Ana alert(1) María"},
		{"tag between words", "Ana<br>María", "Ana María"},
		{"comment tag", "Ana<!-- x -->Luis", "Ana Luis"},
		{"unterminated tag runs to end", "Ana <b onclick=x", "Ana"},
		{"lone angle brackets kept", "Ana <3 y Luis >", "Ana <3 y Luis >"},
		{"lt then space kept", "a < b", "a < b"},
		{"html then redaction", "<i>badword</i>", "[redacted]"},

		// Redaction: whole words, ignoring case and accents.
		{"blocked whole word", "badword", "[redacted]"},
		{"blocked case insensitive", "Ana BADWORD", "Ana [redacted]"},
		{"blocked list entry casing ignored", "ofensivo", "[redacted]"},
		{"blocked accent insensitive", "Zórglé López", "[redacted] López"},
		{"blocked accent and case", "ZÓRGLE", "[redacted]"},
		{"blocked decomposed accent", "Zórgle", "[redacted]"},
		{"blocked fullwidth", "Ana ｂａｄｗｏｒｄ", "Ana [redacted]"},
		{"accents kept in clean words", "José Núñez", "José Núñez"},

		// Word = run of letters and marks; every other rune separates and is kept.
		{"hyphen separator kept", "Ana-badword", "Ana-[redacted]"},
		{"apostrophe separator kept", "O'badword", "O'[redacted]"},
		{"typographic apostrophe separator kept", "O’badword", "O’[redacted]"},
		{"modifier letter apostrophe separator kept", "O\u02bczorgle", "O\u02bc[redacted]"},
		{"enclosing mark folded", "zorgle\u20dd", "[redacted]"},
		{"spacing mark folded", "Ana zorgle\u0903", "Ana [redacted]"},
		{"trailing punctuation", "badword!", "[redacted]!"},
		{"parentheses", "(badword)", "([redacted])"},
		{"slash", "Ana/badword", "Ana/[redacted]"},
		{"digit", "badword1", "[redacted]1"},
		{"emoji", "badword😀", "[redacted]😀"},
		{"period", "Ana badword.", "Ana [redacted]."},
		{"several words", "badword ofensivo-Ana", "[redacted] [redacted]-Ana"},
		{"substring not redacted (prefix)", "Badwordson", "Badwordson"},
		{"substring not redacted (suffix)", "Mybadword", "Mybadword"},
		{"substring not redacted (accent)", "Zorglón", "Zorglón"},
		// Out of scope: leetspeak and splits inside a word.
		{"leetspeak not redacted", "b4dword", "b4dword"},
		{"underscore split not redacted", "bad_word", "bad_word"},

		// Cap: 50 runes after redaction, on a word boundary.
		{"cap 50 ascii hard cut", a(60), a(50)},
		{"cap 50 multibyte hard cut", strings.Repeat("ñ", 60), strings.Repeat("ñ", 50)},
		{"cap 50 runes not bytes", strings.Repeat("é", 50), strings.Repeat("é", 50)},
		{"cap on word boundary", a(49) + " b", a(49)},
		{"cap drops a partial word", "Ana " + a(60), "Ana"},
		{"cap drops long tail word", a(42) + " badwordson", a(42)},
		{"redacted word that no longer fits is dropped", a(42) + " badword", a(42)},
		{"redacted word that fits is kept", a(39) + " badword", a(39) + " [redacted]"},
		{"many blocked words", "badword badword badword badword badword", "[redacted] [redacted] [redacted] [redacted]"},
		{"hard cut never splits redacted", strings.Repeat("!", 45) + "badword", strings.Repeat("!", 45)},
		{"exactly 50 untouched", a(20) + " " + a(29), a(20) + " " + a(29)},

		// Raw input is bounded to 200 runes before anything else.
		// The bound never leaves a partial word: if the cut lands inside a word,
		// that word is dropped too.
		{"raw bound drops partial word", strings.Repeat(" ", 195) + "Ana Luisa", "Ana"},
		{"raw bound counts tags", strings.Repeat("<b>", 66) + "Ana Luisa", ""},
		{"raw bound no partial blocked word", strings.Repeat(" ", 197) + "badword", ""},
		{"raw bound no partial word after tags", strings.Repeat("<b>", 66) + "zorgle!!", ""},
		{"raw bound no partial word after zero-width", strings.Repeat("\u200b", 196) + "zorgleta", ""},
		{"raw bound cut word is not a blocked word", strings.Repeat(" ", 194) + "zorgleta", ""},
		{"raw bound cut on a separator keeps the word", strings.Repeat(" ", 194) + "Ana Lu ruiz", "Ana Lu"},
		{"raw bound exactly 200 untouched", strings.Repeat(" ", 191) + "Ana Luisa", "Ana Luisa"},
		{"raw bound word ending at the cut kept", strings.Repeat(" ", 191) + "Ana Luisa!!", "Ana Luisa"},

		// No trailing unmatched opener.
		{"trailing paren dropped", "Ana (", "Ana"},
		{"trailing bracket dropped", "Ana María [", "Ana María"},
		{"trailing ascii quote dropped", `Ana "`, "Ana"},
		{"trailing curly quote dropped", "Ana \u201c", "Ana"},
		{"trailing guillemet dropped", "Ana \u00ab", "Ana"},
		{"trailing opener with separators before it", "Ana, - (", "Ana"},
		{"trailing opener left by a tag", "Ana (<b>", "Ana"},
		{"matched ascii quotes kept", `"Ana"`, `"Ana"`},
		{"matched curly quotes kept", "\u201cAna\u201d", "\u201cAna\u201d"},
		{"unmatched opener mid-string kept", "Ana (Luis", "Ana (Luis"},
		{"unmatched curly quote mid-string kept", "Ana \u201cLuis", "Ana \u201cLuis"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CleanName(c.in, blocked); got != c.want {
				t.Errorf("CleanName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestCleanNameKeepsEnyeDistinct(t *testing.T) {
	c := NewNameCleaner([]string{"ano", "pena"})
	cases := []struct{ in, want string }{
		{"Año Peña", "Año Peña"},
		{"Ano Pena", "[redacted] [redacted]"},
		{"ANO", "[redacted]"},
	}
	for _, tc := range cases {
		if got := c.Clean(tc.in); got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	c = NewNameCleaner([]string{"Peña"})
	if got := c.Clean("PEÑA Pena"); got != "[redacted] Pena" {
		t.Errorf("got %q, want %q", got, "[redacted] Pena")
	}
}

func TestCleanNameNeverExceedsCap(t *testing.T) {
	c := NewNameCleaner(blocked)
	inputs := []string{
		strings.Repeat("badword ", 30),
		strings.Repeat("a-badword ", 30),
		strings.Repeat("😀", 300),
		strings.Repeat("x", 5000),
		strings.Repeat("ab ", 1000),
		strings.Repeat("<b>badword</b>", 50),
	}
	for _, in := range inputs {
		got := c.Clean(in)
		if n := utf8.RuneCountInString(got); n > MaxNameRunes {
			t.Errorf("Clean(%.20q...) has %d runes, want <= %d", in, n, MaxNameRunes)
		}
		if !utf8.ValidString(got) {
			t.Errorf("Clean(%.20q...) is not valid UTF-8", in)
		}
		if strings.Contains(strings.ReplaceAll(got, Redacted, ""), "[redac") {
			t.Errorf("Clean(%.20q...) = %q splits %s", in, got, Redacted)
		}
	}
}

func TestCleanNameNilBlockedList(t *testing.T) {
	if got := CleanName(" badword! ", nil); got != "badword!" {
		t.Errorf("got %q, want %q", got, "badword!")
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

func TestNameCleanerSkipsMultiTokenEntries(t *testing.T) {
	c := NewNameCleaner([]string{"badword", " bad word ", "bad-word", "x1", "o'zorgle", "", "   ", "Zórgle"})
	want := []string{"bad word", "bad-word", "x1", "o'zorgle"}
	if got := c.Skipped(); !reflect.DeepEqual(got, want) {
		t.Errorf("Skipped() = %q, want %q", got, want)
	}
	if got := c.Clean("bad word bad-word zorgle"); got != "bad word bad-word [redacted]" {
		t.Errorf("Clean = %q", got)
	}
	// The returned slice is a copy.
	c.Skipped()[0] = "mutated"
	if c.Skipped()[0] != "bad word" {
		t.Error("Skipped() must return a copy")
	}
	if got := NewNameCleaner(blocked).Skipped(); len(got) != 0 {
		t.Errorf("Skipped() = %q, want none", got)
	}
}

func TestNormalizeCampaign(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "none"},
		{"   ", "none"},
		{"xmas-2026", "xmas-2026"},
		{"  XMAS-2026 ", "xmas-2026"},
		{"a", "a"},
		{strings.Repeat("a", 32), strings.Repeat("a", 32)},
		{strings.Repeat("a", 33), "invalid"},
		{strings.Repeat("AB", 16), strings.Repeat("ab", 16)},           // folds before the length check
		{" " + strings.Repeat("a", 32) + " ", strings.Repeat("a", 32)}, // trims before the length check
		{"xmas_2026", "invalid"},
		{"xmas 2026", "invalid"},
		{"navidad-ñ", "invalid"},
		{"<script>", "invalid"},
		{"none", "none"},
		{"invalid", "invalid"},
		{"x\n", "x"},
		{"---", "---"},
		{"utm_source=ig", "invalid"},
	}
	for _, c := range cases {
		if got := NormalizeCampaign(c.in); got != c.want {
			t.Errorf("NormalizeCampaign(%q) = %q, want %q", c.in, got, c.want)
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
		// Fails closed.
		{"dob in the future", dob(2030, 1, 1), at(2026, 11, 20, 12), false},
		{"dob far in the future", dob(2100, 1, 1), at(2026, 11, 20, 12), false},
		{"zero dob", time.Time{}, at(2026, 11, 20, 12), false},
		// Feb 29 birthdays: in a non-leap year the birthday is reached on Mar 1.
		{"feb29 on feb28 non-leap", dob(2008, 2, 29), at(2026, 2, 28, 23), false},
		{"feb29 on mar1 non-leap", dob(2008, 2, 29), at(2026, 3, 1, 0), true},
		{"feb29 turns 18 in non-leap 2030, not on feb28", dob(2012, 2, 29), at(2030, 2, 28, 23), false},
		{"feb29 turns 18 in non-leap 2030, on mar1", dob(2012, 2, 29), at(2030, 3, 1, 0), true},
		{"feb29 adult on a leap feb29", dob(2008, 2, 29), at(2028, 2, 29, 12), true},
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

func TestMaskEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"jane@example.com", "j***@example.com"},
		{"j@example.com", "j***@example.com"},
		{"ñandú@example.mx", "ñ***@example.mx"},
		{" jane@example.com ", "j***@example.com"},
		{"@example.com", "***"},
		{"no-at-sign", "***"},
		{"", "***"},
	}
	for _, c := range cases {
		if got := MaskEmail(c.in); got != c.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", c.in, got, c.want)
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
