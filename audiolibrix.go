package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Always scrape the Czech locale: the metadata labels ("Interpret:", "Délka:")
// are localized, so the parser depends on them being Czech.
const siteURL = "https://www.audiolibrix.com"
const localePath = "/cs"

// Book is the BookMetadata object of the Audiobookshelf custom metadata provider spec.
type Book struct {
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle,omitempty"`
	Author        string   `json:"author,omitempty"`
	Narrator      string   `json:"narrator,omitempty"`
	Publisher     string   `json:"publisher,omitempty"`
	PublishedYear string   `json:"publishedYear,omitempty"`
	Description   string   `json:"description,omitempty"`
	Cover         string   `json:"cover,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Series        []Series `json:"series,omitempty"`
	Language      string   `json:"language,omitempty"`
	Duration      int      `json:"duration,omitempty"` // minutes

	langCode string // e.g. "cs", used for filtering only
}

type Series struct {
	Series   string `json:"series"`
	Sequence string `json:"sequence,omitempty"`
}

// candidate is a single item from the search results page.
type candidate struct {
	ID      string
	Title   string
	URL     string
	Authors []string
}

// Language names as shown on the Czech site, mapped to a code and the name Audiobookshelf gets.
var languages = map[string]struct{ code, name string }{
	"Česky":       {"cs", "czech"},
	"Slovensky":   {"sk", "slovak"},
	"Anglicky":    {"en", "english"},
	"Německy":     {"de", "german"},
	"Francouzsky": {"fr", "french"},
	"Italsky":     {"it", "italian"},
	"Rusky":       {"ru", "russian"},
	"Španělsky":   {"es", "spanish"},
	"Ukrajinský":  {"ua", "ukrainian"},
}

var (
	durationRe   = regexp.MustCompile(`(\d+):(\d{2})\s*h`)
	minutesRe    = regexp.MustCompile(`(\d+)\s*min`)
	yearRe       = regexp.MustCompile(`\((\d{4})\)`)
	seriesLangRe = regexp.MustCompile(`\s*\([A-Z]{2}\)$`)
	// "https://www.audiolibrix.com/cs/Directory/Book/596/Audiokniha-..." or "audiolibrix:596"
	bookRefRe = regexp.MustCompile(`(?i)^(?:audiolibrix:\s*|(?:https?://)?(?:www\.)?audiolibrix\.com/[^?#]*?/Directory/Book/)(\d+)(?:[/?#].*)?$`)
)

type Client struct {
	http      *http.Client
	userAgent string
}

func NewClient(userAgent string) *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}, userAgent: userAgent}
}

func (c *Client) fetch(ctx context.Context, u string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept-Language", "cs-CZ,cs;q=0.9")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", u, resp.StatusCode)
	}
	return goquery.NewDocumentFromReader(resp.Body)
}

type SearchOptions struct {
	Languages   map[string]bool // language codes to keep; nil keeps all
	MaxResults  int
	MaxFetch    int // upper bound on detail pages fetched per search
	Concurrency int
	AddLink     bool
}

// Search runs a site search, ranks the results and fetches detail pages in ranked
// order until MaxResults books in an allowed language are found.
// A query that is an Audiolibrix book link or "audiolibrix:<id>" skips the search and
// returns that book, which also covers books no longer listed in search results.
func (c *Client) Search(ctx context.Context, query, author string, opts SearchOptions) ([]Book, error) {
	if m := bookRefRe.FindStringSubmatch(strings.TrimSpace(query)); m != nil {
		return c.lookup(ctx, m[1], opts)
	}
	doc, err := c.fetch(ctx, siteURL+localePath+"/Search/Results?query="+url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	cands := rank(parseSearch(doc), query, author)
	if len(cands) > opts.MaxFetch {
		cands = cands[:opts.MaxFetch]
	}
	slog.Info("search", "query", query, "author", author, "candidates", len(cands))

	books := []Book{} // encodes as [] rather than null when nothing matches
	for start := 0; start < len(cands) && len(books) < opts.MaxResults; start += opts.Concurrency {
		batch := cands[start:min(start+opts.Concurrency, len(cands))]
		results := make([]*Book, len(batch))
		var wg sync.WaitGroup
		for i, cand := range batch {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d, err := c.fetch(ctx, cand.URL)
				if err != nil {
					slog.Warn("fetching book failed", "url", cand.URL, "err", err)
					return
				}
				b := parseBook(d, cand)
				results[i] = &b
			}()
		}
		wg.Wait()

		for i, b := range results {
			if b == nil {
				continue
			}
			if opts.Languages != nil && !opts.Languages[b.langCode] {
				slog.Debug("skipping book in other language", "title", b.Title, "language", b.langCode)
				continue
			}
			if opts.AddLink {
				addLink(b, batch[i].URL)
			}
			books = append(books, *b)
		}
	}
	if len(books) > opts.MaxResults {
		books = books[:opts.MaxResults]
	}
	return books, nil
}

// lookup fetches a single book by its Audiolibrix ID. The language filter is not
// applied, since the book was asked for explicitly.
func (c *Client) lookup(ctx context.Context, id string, opts SearchOptions) ([]Book, error) {
	// The site redirects this to the full book URL, or to the catalog for unknown IDs.
	u := siteURL + localePath + "/Directory/Book/" + id
	doc, err := c.fetch(ctx, u)
	if err != nil {
		return nil, err
	}
	b := parseBook(doc, candidate{ID: id, URL: u})
	if b.Title == "" {
		slog.Info("book not found", "id", id)
		return []Book{}, nil
	}
	if opts.AddLink {
		addLink(&b, u)
	}
	return []Book{b}, nil
}

func addLink(b *Book, u string) {
	b.Description = fmt.Sprintf(`<a href="%s">Audiolibrix</a><br><br>%s`, u, b.Description)
}

func parseSearch(doc *goquery.Document) []candidate {
	var cands []candidate
	seen := map[string]bool{}
	doc.Find("article.alx-audiobook-list-item").Each(func(_ int, s *goquery.Selection) {
		a := s.Find("a.audiobook-link").First()
		id, _ := a.Attr("data-book-id")
		href, _ := a.Attr("href")
		if id == "" || href == "" || seen[id] {
			return
		}
		seen[id] = true
		title, _ := a.Attr("data-book-name")
		cands = append(cands, candidate{
			ID:      id,
			Title:   strings.TrimSpace(title),
			URL:     siteURL + href,
			Authors: linkTexts(s.Find("dd.alx-author")),
		})
	})
	return cands
}

func parseBook(doc *goquery.Document, cand candidate) Book {
	meta := map[string]*goquery.Selection{}
	doc.Find("dl.alx-metadata > dt").Each(func(_ int, dt *goquery.Selection) {
		label := strings.TrimSuffix(strings.TrimSpace(dt.Text()), ":")
		if _, ok := meta[label]; !ok {
			meta[label] = dt.NextFiltered("dd")
		}
	})
	// Labels switch to plural with multiple values (Žánr/Žánry, Vydavatel/Vydavatelé).
	// Matching is exact: the rating rows below include "Interpretace", which a prefix
	// match on "Interpret" would pick up.
	field := func(labels ...string) *goquery.Selection {
		for _, l := range labels {
			if dd, ok := meta[l]; ok {
				return dd
			}
		}
		return nil
	}

	b := Book{Title: cand.Title}
	if b.Title == "" {
		b.Title = strings.TrimPrefix(strings.TrimSpace(doc.Find(`h1[itemprop="name"]`).Text()), "Audiokniha ")
	}
	b.Subtitle = strings.TrimSpace(doc.Find("p.lead").First().Text())

	authors := cand.Authors
	if dd := field("Autor", "Autoři"); dd != nil {
		authors = linkTexts(dd)
	}
	b.Author = strings.Join(authors, ", ")
	if dd := field("Interpret", "Interpreti"); dd != nil {
		b.Narrator = strings.Join(linkTexts(dd), ", ")
	}
	if dd := field("Vydavatel", "Vydavatelé"); dd != nil {
		b.Publisher = strings.Join(linkTexts(dd), ", ")
		if m := yearRe.FindStringSubmatch(dd.Text()); m != nil {
			b.PublishedYear = m[1]
		}
	}
	if dd := field("Žánr", "Žánry"); dd != nil {
		b.Genres = linkTexts(dd)
	}
	if dd := field("Série"); dd != nil {
		b.Series = parseSeries(linkTexts(dd))
	}
	if dd := field("Délka"); dd != nil {
		b.Duration = parseDuration(dd.Text())
	}
	if dd := field("Jazyk"); dd != nil {
		if l, ok := languages[strings.TrimSpace(dd.Text())]; ok {
			b.langCode, b.Language = l.code, l.name
		}
	}
	if dd := field("Obsah"); dd != nil && strings.HasPrefix(strings.TrimSpace(dd.Text()), "Zkrácen") {
		b.Tags = append(b.Tags, "zkrácená verze")
	}

	// The id really is "#alx-publisher-summary", hash included.
	if html, err := doc.Find(`[id="#alx-publisher-summary"]`).Html(); err == nil {
		b.Description = strings.TrimSpace(html)
	}
	b.Cover, _ = doc.Find(`meta[property="og:image"]`).Attr("content")
	return b
}

func parseSeries(names []string) []Series {
	var series []Series
	for _, text := range names {
		name, seq := text, ""
		if i := strings.LastIndex(text, " #"); i >= 0 {
			name, seq = text[:i], text[i+2:]
		}
		// "Harry Potter (DE)" -> "Harry Potter"
		series = append(series, Series{Series: seriesLangRe.ReplaceAllString(name, ""), Sequence: seq})
	}
	return series
}

// parseDuration converts "11:01 h" (or "45 min") to minutes.
func parseDuration(s string) int {
	if m := durationRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		return h*60 + mins
	}
	if m := minutesRe.FindStringSubmatch(s); m != nil {
		mins, _ := strconv.Atoi(m[1])
		return mins
	}
	return 0
}

func linkTexts(s *goquery.Selection) []string {
	var out []string
	s.Find("a").Each(func(_ int, a *goquery.Selection) {
		if t := strings.TrimSpace(a.Text()); t != "" {
			out = append(out, t)
		}
	})
	return out
}
