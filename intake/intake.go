// Package intake holds the enrollment intake rules shared by the
// self-enrollment portal and teller enrollment, so both normalize and accept
// the same input. The blocked-word list is never part of this public repo: it
// is passed in at runtime.
package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // America/Mexico_City on Lambda provided.al2023, which has no zoneinfo
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ErrInvalidEmail is returned by NormalizeEmail for input that is not shaped
// like local@domain.tld.
var ErrInvalidEmail = errors.New("intake: invalid email")

// NormalizeEmail trims and lowercases s and checks it has the basic shape
// local@domain.tld: one @, a non-empty local part, a domain of at least two
// non-empty dot-separated labels, and no whitespace. It is not RFC 5322.
func NormalizeEmail(s string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(s))
	if strings.IndexFunc(e, unicode.IsSpace) >= 0 || strings.Count(e, "@") != 1 {
		return "", ErrInvalidEmail
	}
	local, domain, _ := strings.Cut(e, "@")
	if local == "" {
		return "", ErrInvalidEmail
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", ErrInvalidEmail
	}
	for _, l := range labels {
		if l == "" {
			return "", ErrInvalidEmail
		}
	}
	return e, nil
}

// MaxNameRunes caps a cleaned name, counted in runes before redaction.
const MaxNameRunes = 50

// Redacted replaces each blocked word in a name.
const Redacted = "[redacted]"

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// NameCleaner cleans names against a blocked-word list folded once.
type NameCleaner struct {
	blocked map[string]struct{}
}

// NewNameCleaner folds the blocked words (case and accents) once. Blank
// entries are ignored; a nil list blocks nothing.
func NewNameCleaner(blocked []string) *NameCleaner {
	c := &NameCleaner{blocked: make(map[string]struct{}, len(blocked))}
	for _, w := range blocked {
		if f := fold(strings.TrimSpace(w)); f != "" {
			c.blocked[f] = struct{}{}
		}
	}
	return c
}

// CleanName is NewNameCleaner(blocked).Clean(s), for one-off calls.
func CleanName(s string, blocked []string) string {
	return NewNameCleaner(blocked).Clean(s)
}

// Clean strips HTML tags and control characters, collapses whitespace into
// single spaces, trims, caps the result at MaxNameRunes, and then replaces
// every whole word matching a blocked word with Redacted.
//
// Words are split on spaces, hyphens and apostrophes (' and ’), and the
// separators are kept. Matching ignores case and accents; substrings never
// match.
//
// The cap applies before redaction so the stored length depends only on what
// the customer typed, not on the runtime list, and a Redacted marker is never
// cut in half. A redacted name can therefore exceed MaxNameRunes.
func (c *NameCleaner) Clean(s string) string {
	s = htmlTag.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > MaxNameRunes {
		s = strings.TrimSpace(string(r[:MaxNameRunes]))
	}
	if len(c.blocked) == 0 {
		return s
	}

	var b strings.Builder
	start := 0
	flush := func(end int) {
		w := s[start:end]
		if _, ok := c.blocked[fold(w)]; ok && w != "" {
			w = Redacted
		}
		b.WriteString(w)
	}
	for i, r := range s {
		if isWordSeparator(r) {
			flush(i)
			b.WriteRune(r)
			start = i + len(string(r))
		}
	}
	flush(len(s))
	return b.String()
}

func isWordSeparator(r rune) bool {
	return r == ' ' || r == '-' || r == '\'' || r == '’'
}

// fold lowercases w and drops combining marks after NFD, so "Pendéjo" folds
// to "pendejo" (and "ñ" to "n").
func fold(w string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, norm.NFD.String(w)))
}

var campaignPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// NormalizeCampaign trims and lowercases a campaign tag. Empty input is
// "none", a tag matching ^[a-z0-9-]{1,32}$ is itself, anything else "invalid".
func NormalizeCampaign(s string) string {
	c := strings.ToLower(strings.TrimSpace(s))
	switch {
	case c == "":
		return "none"
	case campaignPattern.MatchString(c):
		return c
	}
	return "invalid"
}

// AdultAge is the age of majority in Mexico.
const AdultAge = 18

var mexicoCity = mustLoadLocation("America/Mexico_City")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Adult reports whether someone born on dob is AdultAge or older on now's
// date in America/Mexico_City. dob's calendar date is read as is (in its own
// location; a date-only value parsed as UTC is the usual input). A Feb 29
// birthday is reached on Mar 1 in non-leap years. A future dob is not adult.
func Adult(dob, now time.Time) bool {
	by, bm, bd := dob.Date()
	ny, nm, nd := now.In(mexicoCity).Date()
	age := ny - by
	if nm < bm || (nm == bm && nd < bd) {
		age--
	}
	return age >= AdultAge
}

// MaskEmail hides the local part of an email for logs: jane@example.com is
// j***@example.com. Input with no @, or with @ first, is "***".
func MaskEmail(email string) string {
	e := strings.TrimSpace(email)
	at := strings.LastIndex(e, "@")
	if at <= 0 {
		return "***"
	}
	first := []rune(e[:at])[0]
	return string(first) + "***" + e[at:]
}

// EmailHash is a stable log key for an email: the first 12 hex characters of
// the SHA-256 of NormalizeEmail's output, or of the trimmed, lowercased input
// when it does not normalize.
func EmailHash(email string) string {
	e, err := NormalizeEmail(email)
	if err != nil {
		e = strings.ToLower(strings.TrimSpace(email))
	}
	sum := sha256.Sum256([]byte(e))
	return hex.EncodeToString(sum[:])[:12]
}
