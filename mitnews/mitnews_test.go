package mitnews_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/mitnews-cli/mitnews"
)

// rssXML returns a minimal valid RSS 2.0 feed with the given items injected.
func rssXML(items string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
` + items + `
  </channel>
</rss>`
}

func singleItem(title, link, pubDate, creator, category, description string) string {
	return `<item>
  <title>` + title + `</title>
  <link>` + link + `</link>
  <pubDate>` + pubDate + `</pubDate>
  <dc:creator><![CDATA[` + creator + `]]></dc:creator>
  <category><![CDATA[` + category + `]]></category>
  <description><![CDATA[` + description + `]]></description>
</item>`
}

func newTestClient(ts *httptest.Server) *mitnews.Client {
	cfg := mitnews.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return mitnews.NewClient(cfg)
}

func TestLatestParsesTitle(t *testing.T) {
	xml := rssXML(singleItem(
		"New Quantum Computing Breakthrough",
		"https://news.mit.edu/2024/01/quantum",
		"Mon, 15 Jan 2024 12:00:00 +0000",
		"Anne Trafton | MIT News",
		"research",
		"<p>MIT researchers have achieved a new milestone.</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("got %d articles, want 1", len(arts))
	}
	if arts[0].Title != "New Quantum Computing Breakthrough" {
		t.Errorf("Title = %q", arts[0].Title)
	}
}

func TestLatestParsesAuthor(t *testing.T) {
	xml := rssXML(singleItem(
		"Gene Editing Advances",
		"https://news.mit.edu/2024/01/gene",
		"Wed, 10 Jan 2024 09:00:00 +0000",
		"David Chandler | MIT News",
		"biology",
		"<p>Body text.</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if arts[0].Author != "David Chandler | MIT News" {
		t.Errorf("Author = %q", arts[0].Author)
	}
}

func TestLatestParsesURL(t *testing.T) {
	wantURL := "https://news.mit.edu/2024/01/new-chip-architecture"
	xml := rssXML(singleItem(
		"New Chip Architecture",
		wantURL,
		"Fri, 12 Jan 2024 15:30:00 +0000",
		"MIT News",
		"engineering",
		"<p>Summary here.</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if arts[0].URL != wantURL {
		t.Errorf("URL = %q, want %q", arts[0].URL, wantURL)
	}
}

func TestLatestParsesDate(t *testing.T) {
	xml := rssXML(singleItem(
		"Robot Learns to Walk",
		"https://news.mit.edu/2024/03/robot",
		"Thu, 07 Mar 2024 18:00:00 +0000",
		"Sarah McDonnell | MIT News",
		"robotics",
		"<p>Details.</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if arts[0].Published != "2024-03-07" {
		t.Errorf("Published = %q, want %q", arts[0].Published, "2024-03-07")
	}
}

func TestLatestStripsSummaryHTML(t *testing.T) {
	xml := rssXML(singleItem(
		"AI Safety Study",
		"https://news.mit.edu/2024/01/ai-safety",
		"Sat, 20 Jan 2024 10:00:00 +0000",
		"MIT News",
		"ai",
		"<p>This is the <b>summary</b> text.</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(arts[0].Summary, "<") || strings.Contains(arts[0].Summary, ">") {
		t.Errorf("Summary contains HTML tags: %q", arts[0].Summary)
	}
	if !strings.Contains(arts[0].Summary, "summary") {
		t.Errorf("Summary text missing: %q", arts[0].Summary)
	}
}

func TestLatestTruncatesSummary(t *testing.T) {
	long := strings.Repeat("x", 300)
	xml := rssXML(singleItem(
		"Long Article",
		"https://news.mit.edu/2024/01/long",
		"Mon, 01 Jan 2024 00:00:00 +0000",
		"Author Name",
		"research",
		"<p>"+long+"</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	runes := []rune(arts[0].Summary)
	if len(runes) > 150 {
		t.Errorf("Summary too long: %d runes", len(runes))
	}
	if !strings.HasSuffix(arts[0].Summary, "…") {
		t.Errorf("Summary missing ellipsis: %q", arts[0].Summary)
	}
}

func TestLatestRankOrder(t *testing.T) {
	items := singleItem("A", "https://news.mit.edu/2024/01/a", "Mon, 01 Jan 2024 00:00:00 +0000", "X", "research", "") +
		singleItem("B", "https://news.mit.edu/2024/01/b", "Tue, 02 Jan 2024 00:00:00 +0000", "Y", "research", "") +
		singleItem("C", "https://news.mit.edu/2024/01/c", "Wed, 03 Jan 2024 00:00:00 +0000", "Z", "research", "")
	xml := rssXML(items)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 3 {
		t.Fatalf("got %d articles, want 3", len(arts))
	}
	for i, a := range arts {
		if a.Rank != i+1 {
			t.Errorf("arts[%d].Rank = %d, want %d", i, a.Rank, i+1)
		}
	}
}

func TestLatestLimit(t *testing.T) {
	items := ""
	for i := 0; i < 5; i++ {
		items += singleItem("T", "https://news.mit.edu/2024/01/x", "Mon, 01 Jan 2024 00:00:00 +0000", "A", "research", "")
	}
	xml := rssXML(items)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Latest(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Errorf("got %d articles with limit=2, want 2", len(arts))
	}
}

func TestTopicArticlesUnknownTopic(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err := newTestClient(ts).TopicArticles(context.Background(), "nonexistent", 0)
	if !errors.Is(err, mitnews.ErrUnknownTopic) {
		t.Errorf("got %v, want ErrUnknownTopic", err)
	}
}

func TestTopicArticlesTagsTopic(t *testing.T) {
	// The feed returns items with multi-word categories; the topic key should be set from the feed key.
	xml := rssXML(singleItem(
		"New AI Model",
		"https://news.mit.edu/2024/01/ai-model",
		"Mon, 15 Jan 2024 12:00:00 +0000",
		"MIT News",
		"Artificial intelligence",
		"<p>Summary.</p>",
	))
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).TopicArticles(context.Background(), "ai", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) == 0 {
		t.Fatal("got no articles")
	}
	// Topic should be set to "ai" from the feed key because category "Artificial intelligence" has a space.
	if arts[0].Topic != "ai" {
		t.Errorf("Topic = %q, want %q", arts[0].Topic, "ai")
	}
}

func TestSearchFilters(t *testing.T) {
	items := singleItem("Quantum Computing Advance", "https://news.mit.edu/2024/01/quantum", "Mon, 01 Jan 2024 00:00:00 +0000", "MIT News", "research", "Qubits improve") +
		singleItem("Robot Vision Study", "https://news.mit.edu/2024/01/robot", "Tue, 02 Jan 2024 00:00:00 +0000", "MIT News", "robotics", "Visual recognition") +
		singleItem("Climate Model Update", "https://news.mit.edu/2024/01/climate", "Wed, 03 Jan 2024 00:00:00 +0000", "MIT News", "climate", "Warming trends")
	xml := rssXML(items)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Search(context.Background(), "quantum", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("got %d results for 'quantum', want 1", len(arts))
	}
	if !strings.Contains(strings.ToLower(arts[0].Title), "quantum") {
		t.Errorf("unexpected result: %q", arts[0].Title)
	}
}

func TestSearchLimit(t *testing.T) {
	items := ""
	for i := 0; i < 5; i++ {
		items += singleItem("AI Research Paper", "https://news.mit.edu/2024/01/ai", "Mon, 01 Jan 2024 00:00:00 +0000", "MIT News", "ai", "Machine learning study")
	}
	xml := rssXML(items)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(xml))
	}))
	defer ts.Close()

	arts, err := newTestClient(ts).Search(context.Background(), "ai", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 2 {
		t.Errorf("got %d results with limit=2, want 2", len(arts))
	}
}

func TestTopicsReturnsAll(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rssXML("")))
	}))
	defer ts.Close()

	topics := newTestClient(ts).Topics()
	if len(topics) == 0 {
		t.Fatal("Topics() returned no topics")
	}
	for i, tp := range topics {
		if tp.Rank != i+1 {
			t.Errorf("topics[%d].Rank = %d, want %d", i, tp.Rank, i+1)
		}
		if tp.Name == "" {
			t.Errorf("topics[%d].Name is empty", i)
		}
		if tp.Label == "" {
			t.Errorf("topics[%d].Label is empty", i)
		}
		if !strings.HasPrefix(tp.URL, ts.URL) {
			t.Errorf("topics[%d].URL = %q, should start with test server URL", i, tp.URL)
		}
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(rssXML("")))
	}))
	defer ts.Close()

	cfg := mitnews.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := mitnews.NewClient(cfg)

	start := time.Now()
	_, err := c.Latest(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetUserAgent(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(rssXML("")))
	}))
	defer ts.Close()

	cfg := mitnews.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	c := mitnews.NewClient(cfg)
	_, _ = c.Latest(context.Background(), 0)

	if gotUA == "" {
		t.Error("request carried no User-Agent")
	}
}

func TestTopicNamesNotEmpty(t *testing.T) {
	names := mitnews.TopicNames()
	if len(names) == 0 {
		t.Fatal("TopicNames() returned empty slice")
	}
	for _, n := range names {
		if n == "" {
			t.Error("TopicNames() contains empty string")
		}
	}
}
