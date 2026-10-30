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
	cands := parseSearch(loadDoc(t, "search_strazca.html"))
	// 6 book entries, one listed twice, plus a podcast without a book link.
	if len(cands) != 5 {
		t.Fatalf("got %d candidates, want 5", len(cands))
	}
	want := candidate{
		ID:      "900101",
		Title:   "Strážca svetiel I: Prvý plameň",
		URL:     "https://www.audiolibrix.com/cs/Directory/Book/900101/Audiokniha-Strazca-svetiel-I-Prvy-plamen-Tomas-Vrbiak",
		Authors: []string{"Tomáš Vrbiak"},
	}
	if !reflect.DeepEqual(cands[0], want) {
		t.Errorf("first candidate = %+v, want %+v", cands[0], want)
	}
	if got, want := cands[3].Authors, []string{"Zuzana Brezová", "Peter Javor"}; !reflect.DeepEqual(got, want) {
		t.Errorf("multi-author candidate authors = %v, want %v", got, want)
	}
}

func TestParseBook(t *testing.T) {
	tests := []struct {
		file string
		want Book
	}{
		{"book_strazca2.html", Book{
			Subtitle:      "Keď zhasne posledná lampa, začína jeho služba",
			Author:        "Tomáš Vrbiak",
			Narrator:      "Ondrej Halaš",
			Publisher:     "Ukážkové audio",
			PublishedYear: "2024",
			Cover:         "https://cdn.example.com/covers/900102/1000/strazca-svetiel-ii.jpg",
			Genres:        []string{"Fantasy"},
			Series:        []Series{{Series: "Strážca svetiel", Sequence: "2"}},
			Language:      "slovak",
			Duration:      12*60 + 34,
			langCode:      "sk",
		}},
		{"book_kamen_sk.html", Book{
			Subtitle:      "...a čo všetko voda neodnesie",
			Author:        "Zuzana Brezová, Peter Javor",
			Narrator:      "Lucia Sladká, Marek Dub",
			Publisher:     "Prvé vydavateľstvo, Druhé vydavateľstvo",
			PublishedYear: "2023",
			Cover:         "https://cdn.example.com/covers/900201/1000/kamen-pod-mostom.jpg",
			Genres:        []string{"Dobrodružné"},
			Language:      "slovak",
			Duration:      8*60 + 2,
			langCode:      "sk",
		}},
		{"book_mlynar.html", Book{
			Subtitle:      "Příběh jednoho mlýna a tří generací",
			Author:        "Hedvika Pokorná",
			Narrator:      "Jan Kos",
			Publisher:     "Vzorový zvuk",
			PublishedYear: "2020",
			Cover:         "https://cdn.example.com/covers/900301/1000/mlynarova-dcera.jpg",
			Genres:        []string{"Rodinné ságy", "Klasika", "Venkov"},
			Language:      "czech",
			Duration:      7*60 + 5,
			langCode:      "cs",
		}},
		{"book_mlynar_zkracena.html", Book{
			Subtitle:      "Zkrácená četba pro rozhlas",
			Author:        "Hedvika Pokorná",
			Narrator:      "Eva Malá",
			Publisher:     "Ukázková nahrávka",
			PublishedYear: "2015",
			Cover:         "https://cdn.example.com/covers/900302/1000/mlynarova-dcera-zkracena.jpg",
			Genres:        []string{"Klasika", "Venkov"},
			Tags:          []string{"zkrácená verze"},
			Language:      "czech",
			Duration:      2*60 + 15,
			langCode:      "cs",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := parseBook(loadDoc(t, tt.file), candidate{Title: "x"})

			if !strings.HasPrefix(got.Description, "<p>") || strings.Contains(got.Description, "itemprop") {
				t.Errorf("description does not look like the annotation: %.120q", got.Description)
			}
			got.Title, got.Description = "", ""
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestParseBookTitleFromPage(t *testing.T) {
	// Direct lookups have no search result to take the title from.
	if got := parseBook(loadDoc(t, "book_strazca2.html"), candidate{}).Title; got != "Strážca svetiel II: Noc bez hviezd" {
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
		"https://www.audiolibrix.com/cs/Directory/Book/596/Audiokniha-Lampar-Hedvika-Pokorna": "596",
		"https://www.audiolibrix.com/sk/Directory/Book/596":                                   "596",
		"www.audiolibrix.com/cs/Directory/Book/596/x?utm_source=google":                       "596",
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
