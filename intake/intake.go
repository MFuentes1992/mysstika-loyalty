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
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ErrInvalidEmail is returned by NormalizeEmail for input it does not accept.
var ErrInvalidEmail = errors.New("intake: invalid email")

const (
	maxEmailLen   = 254
	maxLocalLen   = 64
	maxLabelLen   = 63
	emailSpecials = `<>()[]\,;:"`
)

// NormalizeEmail trims and lowercases s and accepts it only when it is shaped
// like local@domain.tld. It is deliberately stricter and simpler than RFC 5322:
//
//   - valid UTF-8, at most 254 bytes, exactly one @
//   - ASCII only (local part and domain), so no Unicode format characters
//     such as a zero-width space; no whitespace, control characters or any of
//     <>()[]\,;:"
//   - local part of 1–64 bytes with no leading, trailing or doubled dot
//     (+ tags and inner dots are kept)
//   - two or more domain labels of 1–63 ASCII letters, digits or hyphens, not
//     starting or ending with a hyphen; the TLD is 2+ letters
func NormalizeEmail(s string) (string, error) {
	if !utf8.ValidString(s) { // before ToLower, which turns invalid bytes into U+FFFD
		return "", ErrInvalidEmail
	}
	e := strings.ToLower(strings.TrimSpace(s))
	if e == "" || len(e) > maxEmailLen || strings.Count(e, "@") != 1 {
		return "", ErrInvalidEmail
	}
	for _, r := range e {
		if r > unicode.MaxASCII || unicode.IsSpace(r) || unicode.IsControl(r) ||
			unicode.Is(unicode.Cf, r) || strings.ContainsRune(emailSpecials, r) {
			return "", ErrInvalidEmail
		}
	}
	local, domain, _ := strings.Cut(e, "@")
	if local == "" || len(local) > maxLocalLen ||
		local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return "", ErrInvalidEmail
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", ErrInvalidEmail
	}
	for _, l := range labels {
		if !validLabel(l) {
			return "", ErrInvalidEmail
		}
	}
	tld := labels[len(labels)-1]
	if len(tld) < 2 {
		return "", ErrInvalidEmail
	}
	for i := 0; i < len(tld); i++ {
		if tld[i] < 'a' || tld[i] > 'z' {
			return "", ErrInvalidEmail
		}
	}
	return e, nil
}

func validLabel(l string) bool {
	if l == "" || len(l) > maxLabelLen || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

const (
	// MaxRawNameRunes bounds the raw input before any processing.
	MaxRawNameRunes = 200
	// MaxNameRunes caps the cleaned name, after redaction.
	MaxNameRunes = 50
	// Redacted replaces each blocked word in a name.
	Redacted = "[redacted]"
)

// htmlTag is a tag start (< followed by a letter, / or !) up to the next > or,
// for an unterminated tag, the end of the input.
var htmlTag = regexp.MustCompile(`<[\p{L}/!][^>]*(?:>|$)`)

// NameCleaner cleans names against a blocked-word list folded once.
type NameCleaner struct {
	blocked map[string]struct{}
	skipped []string
}

// NewNameCleaner folds the blocked words (case and accents) once. Blank
// entries are ignored and a nil list blocks nothing. Only single-word entries
// are used: an entry containing a separator (a space, hyphen, apostrophe,
// digit or any other non-letter) can never match a word, so it is set aside
// and reported by Skipped for the caller to log.
func NewNameCleaner(blocked []string) *NameCleaner {
	c := &NameCleaner{blocked: make(map[string]struct{}, len(blocked))}
	for _, w := range blocked {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		f := fold(w)
		if strings.IndexFunc(f, isSeparator) >= 0 {
			c.skipped = append(c.skipped, w)
			continue
		}
		c.blocked[f] = struct{}{}
	}
	return c
}

// Skipped returns the trimmed blocked-list entries that were not used because
// they are not a single word.
func (c *NameCleaner) Skipped() []string {
	return append([]string(nil), c.skipped...)
}

// CleanName is NewNameCleaner(blocked).Clean(s), for one-off calls.
func CleanName(s string, blocked []string) string {
	return NewNameCleaner(blocked).Clean(s)
}

// Clean returns a display name of at most MaxNameRunes runes:
//
//  1. drop invalid UTF-8 and control and format characters (NUL, zero-width
//     space, soft hyphen...); turn whitespace into spaces;
//  2. keep the first MaxRawNameRunes runes; when that cut lands inside a
//     word, the kept part of that word is dropped too;
//  3. replace each HTML tag with a space (a tag starts at < followed by a
//     letter, / or !, and an unterminated one runs to the end), so a lone < or
//     > is kept; collapse spaces and trim;
//  4. replace every whole word that matches a blocked word with Redacted;
//  5. cap at MaxNameRunes on a word boundary;
//  6. drop a trailing unmatched opener ( [ " “ « and the separators before
//     it, so a name never ends in "Ana (". An unmatched opener mid-string
//     ("Ana (Luis") is left alone: that is cosmetic and accepted.
//
// A word is a run of letters and combining marks. Every other rune
// (punctuation, digits, emoji, _, spaces, hyphens, apostrophes including ʼ) separates
// words and is kept, so "Ana-badword" is "Ana-[redacted]" and "badword!" is
// "[redacted]!". Matching ignores case, compatibility forms (fullwidth
// letters) and accents, but ñ stays distinct from n. Substrings never match.
// Leetspeak ("b4dword") and splits inside a word ("bad_word") are out of
// scope.
//
// The cap runs after redaction so the result always fits MaxNameRunes and a
// Redacted marker is never split. It keeps whole words up to the last word
// that fits. When not even the first word fits, it hard-cuts: separators may
// be cut anywhere, but a word is cut only when it is the first token and the
// piece is neither a blocked word nor a blocked-word prefix; otherwise the
// output stops before that word.
//
// The result is not HTML-safe: escape it wherever it is rendered.
func (c *NameCleaner) Clean(s string) string {
	s = strings.ToValidUTF8(s, "")
	// Strip control and format characters (zero-width space, soft hyphen,
	// NUL...) before the raw cut, so they neither count toward it nor hide a
	// word from it. Whitespace becomes a plain space.
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > MaxRawNameRunes {
		kept := r[:MaxRawNameRunes]
		if !isSeparator(r[MaxRawNameRunes]) {
			// The cut is inside a word: drop its kept part too, so a prefix
			// of a blocked word ("bad" of "badword") never survives.
			for len(kept) > 0 && !isSeparator(kept[len(kept)-1]) {
				kept = kept[:len(kept)-1]
			}
		}
		s = string(kept)
	}
	s = htmlTag.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")

	toks := tokenize(s)
	for i := range toks {
		if toks[i].word {
			if _, ok := c.blocked[fold(toks[i].text)]; ok {
				toks[i].text, toks[i].redacted = Redacted, true
			}
		}
	}
	return dropTrailingOpener(c.capTokens(toks))
}

const (
	openers = "([\"\u201c\u00ab" // ( [ " “ «
	closers = ")]\"\u201d\u00bb" // ) ] " ” »
)

// dropTrailingOpener removes an unmatched opener at the end of s, with the
// separators before it, so a name never ends in "Ana (". A closing bracket or
// quote before it is kept. An unmatched opener mid-string is left alone.
func dropTrailingOpener(s string) string {
	for {
		r, size := utf8.DecodeLastRuneInString(s)
		if size == 0 || !strings.ContainsRune(openers, r) {
			return s
		}
		if r == '"' && strings.Count(s, `"`)%2 == 0 {
			return s // closes an earlier "
		}
		s = strings.TrimRightFunc(s[:len(s)-size], func(r rune) bool {
			return isSeparator(r) && !strings.ContainsRune(closers, r)
		})
	}
}

type token struct {
	text           string
	word, redacted bool
}

// tokenize splits s into alternating runs of word runes and separators.
func tokenize(s string) []token {
	var toks []token
	start := 0
	for i, r := range s {
		w := !isSeparator(r)
		if i > start && w != toks[len(toks)-1].word {
			toks[len(toks)-1].text = s[start:i]
			start = i
		}
		if i == start {
			toks = append(toks, token{word: w})
		}
	}
	if len(toks) > 0 {
		toks[len(toks)-1].text = s[start:]
	}
	return toks
}

func (c *NameCleaner) capTokens(toks []token) string {
	total, lastWordEnd := 0, -1
	for i, t := range toks {
		total += utf8.RuneCountInString(t.text)
		if t.word && total <= MaxNameRunes {
			lastWordEnd = i
		}
	}
	var b strings.Builder
	if total <= MaxNameRunes {
		for _, t := range toks {
			b.WriteString(t.text)
		}
		return b.String()
	}
	if lastWordEnd >= 0 {
		for _, t := range toks[:lastWordEnd+1] {
			b.WriteString(t.text)
		}
		return b.String()
	}
	// Not even the first word fits: hard-cut. Separators may be cut anywhere.
	// A word is cut only when it is the first token and the piece is neither a
	// blocked word nor a blocked-word prefix; otherwise stop before it.
	left := MaxNameRunes
	for i, t := range toks {
		n := utf8.RuneCountInString(t.text)
		if n <= left {
			b.WriteString(t.text)
			left -= n
			continue
		}
		piece := string([]rune(t.text)[:left])
		if !t.word || (i == 0 && !t.redacted && !c.blockedPrefix(piece)) {
			b.WriteString(piece)
		}
		break
	}
	return strings.TrimSpace(b.String())
}

// isSeparator reports whether r separates words. U+02BC (ʼ) is a modifier
// letter but is typed as an apostrophe, so it separates too.
// blockedPrefix reports whether w, folded, is a blocked word or the start of
// one.
func (c *NameCleaner) blockedPrefix(w string) bool {
	f := fold(w)
	for b := range c.blocked {
		if strings.HasPrefix(b, f) {
			return true
		}
	}
	return false
}

func isSeparator(r rune) bool {
	return r == '\u02bc' || !unicode.IsLetter(r) && !unicode.IsMark(r)
}

// fold lowercases w and, after NFKD, drops every combining mark (Mn, Mc, Me)
// except the tilde of ñ, so "Zórglé" folds to "zorgle", "ｂａｄ" to "bad" and "Peña" to "peña"
// (not "pena").
func fold(w string) string {
	var b strings.Builder
	var prev rune
	for _, r := range norm.NFKD.String(w) {
		if unicode.IsMark(r) && !(r == '\u0303' && (prev == 'n' || prev == 'N')) {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return norm.NFC.String(strings.ToLower(b.String()))
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
// date in America/Mexico_City. A Feb 29 birthday is reached on Mar 1 in
// non-leap years.
//
// Pass dob as a date-only value (e.g. time.Parse(time.DateOnly, s), which is
// midnight UTC): its year, month and day are read as is, never converted to
// Mexico City time. Only now is converted.
//
// Adult fails closed: a zero dob or a dob after now's date is not adult.
// Callers only call it when a dob was given; a missing dob is the caller's
// decision, not an implicit pass.
func Adult(dob, now time.Time) bool {
	if dob.IsZero() {
		return false
	}
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
	first, _ := utf8.DecodeRuneInString(e)
	return string(first) + "***" + e[at:]
}

// EmailHash is a stable log key for an email: the first 12 hex characters of
// the SHA-256 of NormalizeEmail's output, or of the trimmed, lowercased input
// when it does not normalize.
//
// It is a pseudonym, not anonymization: it is unsalted, so anyone holding a
// candidate email can hash it and match the key. Treat it as personal data.
func EmailHash(email string) string {
	e, err := NormalizeEmail(email)
	if err != nil {
		e = strings.ToLower(strings.TrimSpace(email))
	}
	sum := sha256.Sum256([]byte(e))
	return hex.EncodeToString(sum[:])[:12]
}
