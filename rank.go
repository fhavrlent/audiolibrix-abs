package main

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// rank orders search results by relevance. The site search is loose ("harry potter"
// also returns Beatrix Potter and unrelated thrillers), so weak matches are dropped
// and the rest sorted by how well they match.
func rank(cands []candidate, query, author string) []candidate {
	q := tokens(query)
	a := tokens(author)
	type scored struct {
		c     candidate
		score float64
	}
	var list []scored
	for _, c := range cands {
		title := tokens(c.Title)
		authors := tokens(strings.Join(c.Authors, " "))
		titleMatch := overlap(q, title)
		queryInAuthors := overlap(q, authors)
		// Require a title word in common, or a query that is just the author's name.
		// A shared author word alone ("Princ Harry" for "harry potter") is noise.
		if titleMatch == 0 && queryInAuthors < 1 {
			continue
		}
		// Audiobookshelf often puts the author into the query too.
		score := titleMatch + 0.5*overlap(a, authors) + 0.25*queryInAuthors
		list = append(list, scored{c, score})
	}
	// Stable sort keeps the site's order among equal scores.
	slices.SortStableFunc(list, func(x, y scored) int {
		switch {
		case x.score > y.score:
			return -1
		case x.score < y.score:
			return 1
		}
		return 0
	})
	out := make([]candidate, len(list))
	for i, s := range list {
		out[i] = s.c
	}
	return out
}

// overlap returns the fraction of want's words found in have.
func overlap(want, have []string) float64 {
	if len(want) == 0 {
		return 0
	}
	n := 0
	for _, w := range want {
		if slices.Contains(have, w) {
			n++
		}
	}
	return float64(n) / float64(len(want))
}

var stripDiacritics = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// tokens lowercases s, strips diacritics and splits it into words.
func tokens(s string) []string {
	s, _, _ = transform.String(stripDiacritics, strings.ToLower(s))
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
