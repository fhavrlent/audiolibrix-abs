package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func loadDoc(t *testing.T, name string) *goquery.Document {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestParseSearch(t *testing.T) {
	cands := parseSearch(loadDoc(t, "search_zaklinac.html"))
	if len(cands) != 16 {
		t.Fatalf("got %d candidates, want 16", len(cands))
	}
	want := candidate{
		ID:      "16088",
		Title:   "Zaklínač I: Posledné želanie",
		URL:     "https://www.audiolibrix.com/cs/Directory/Book/16088/Audiokniha-Zaklinac-I-Posledne-zelanie-Andrzej-Sapkowski",
		Authors: []string{"Andrzej Sapkowski"},
	}
	if !reflect.DeepEqual(cands[0], want) {
		t.Errorf("first candidate = %+v, want %+v", cands[0], want)
	}
}

func TestParseBook(t *testing.T) {
	tests := []struct {
		file string
		want Book
	}{
		{"book_zaklinac2.html", Book{
			Subtitle:      "Nech sa snažíš akokoľvek, osudu neunikneš",
			Author:        "Andrzej Sapkowski",
			Narrator:      "Milo Kráľ",
			Publisher:     "Voxi",
			PublishedYear: "2025",
			Genres:        []string{"Fantasy"},
			Series:        []Series{{Series: "Zaklínač", Sequence: "2"}},
			Language:      "slovak",
			Duration:      16*60 + 8,
			langCode:      "sk",
		}},
		{"book_hobit_sk.html", Book{
			Subtitle:      "...alebo cesta tam a späť",
			Author:        "John Ronald Reuel Tolkien",
			Narrator:      "Dušan Cinkota",
			Publisher:     "SLOVART, 582",
			PublishedYear: "2023",
			Genres:        []string{"Fantasy"},
			Language:      "slovak",
			Duration:      11*60 + 1,
			langCode:      "sk",
		}},
		{"book_babicka.html", Book{
			Author:        "Božena Němcová",
			Narrator:      "Magdaléna Borová",
			Publisher:     "Témbr",
			PublishedYear: "2021",
			Genres:        []string{"Děti a mládež", "Klasika", "Společenská próza"},
			Language:      "czech",
			Duration:      9*60 + 40,
			langCode:      "cs",
		}},
		{"book_babicka_zkracena.html", Book{
			Author:        "Božena Němcová",
			Narrator:      "Libuše Šafránková",
			Publisher:     "Popron Music",
			PublishedYear: "2014",
			Genres:        []string{"Děti a mládež", "Klasika"},
			Tags:          []string{"zkrácená verze"},
			Language:      "czech",
			Duration:      3*60 + 47,
			langCode:      "cs",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := parseBook(loadDoc(t, tt.file), candidate{Title: "x"})

			if !strings.HasPrefix(got.Cover, "https://d2emjept89nv7b.cloudfront.net/covers/") {
				t.Errorf("cover = %q", got.Cover)
			}
			if !strings.HasPrefix(got.Description, "<p>") || strings.Contains(got.Description, "itemprop") {
				t.Errorf("description does not look like the annotation: %.120q", got.Description)
			}
			got.Title, got.Cover, got.Description = "", "", ""
			// Subtitle is not asserted for the Babička fixtures.
			if tt.want.Subtitle == "" {
				got.Subtitle = ""
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseBookTitleFromPage(t *testing.T) {
	// Direct lookups have no search result to take the title from.
	if got := parseBook(loadDoc(t, "book_zaklinac2.html"), candidate{}).Title; got != "Zaklínač II: Meč osudu" {
		t.Errorf("title = %q", got)
	}
}

func TestParseSeries(t *testing.T) {
	got := parseSeries([]string{"Harry Potter (DE) #6", "Zaklínač #2", "Bez čísla"})
	want := []Series{{"Harry Potter", "6"}, {"Zaklínač", "2"}, {"Bez čísla", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]int{"11:01 h": 661, "0:45 h": 45, "45 min": 45, "": 0} {
		if got := parseDuration(in); got != want {
			t.Errorf("parseDuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRank(t *testing.T) {
	cands := []candidate{
		{ID: "1", Title: "10 Rare Beatrix Potter Tales", Authors: []string{"Beatrix Potter"}},
		{ID: "2", Title: "Hodina duchov", Authors: []string{"Michael Connelly"}},
		{ID: "3", Title: "Harry Potter a Kámen mudrců", Authors: []string{"J.K. Rowling"}},
		{ID: "4", Title: "Harry Potter und der Feuerkelch", Authors: []string{"J.K. Rowling"}},
		{ID: "5", Title: "Náhradník", Authors: []string{"Princ Harry"}},
	}
	ids := func(cs []candidate) []string {
		out := []string{}
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	if got, want := ids(rank(cands, "Harry Potter a kamen mudrcu", "J.K. Rowling")), []string{"3", "4", "1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("title query: ranked ids = %v, want %v", got, want)
	}
	if got, want := ids(rank(cands, "rowling", "")), []string{"3", "4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("author-only query: ranked ids = %v, want %v", got, want)
	}
}

func TestBookRef(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.audiolibrix.com/cs/Directory/Book/596/Audiokniha-Druhy-gong-Agatha-Christie": "596",
		"https://www.audiolibrix.com/sk/Directory/Book/596":                                       "596",
		"www.audiolibrix.com/cs/Directory/Book/596/x?utm_source=google":                           "596",
		"audiolibrix:596":  "596",
		"Audiolibrix: 596": "596",
		"Druhý gong":       "",
		"1984":             "",
		"https://example.com/cs/Directory/Book/596": "",
	} {
		got := ""
		if m := bookRefRe.FindStringSubmatch(in); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("bookRefRe(%q) = %q, want %q", in, got, want)
		}
	}
}
