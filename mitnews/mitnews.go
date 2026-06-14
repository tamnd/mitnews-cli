// Package mitnews is the library behind the mit command: the HTTP client,
// request shaping, and the typed data models for MIT News.
//
// Data comes from the public RSS feeds at news.mit.edu. No API key is required.
// The client sends a real User-Agent, paces requests, and retries 429/5xx with
// exponential backoff.
package mitnews

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrUnknownTopic is returned when the topic argument does not match any
// registered feed key.
var ErrUnknownTopic = errors.New("unknown topic")

// feedEntry maps a topic key to its feed path (relative to BaseURL).
type feedEntry struct {
	key   string
	path  string
	label string
}

var feeds = []feedEntry{
	{"ai", "/topic/mitartificial-intelligence2-rss.xml", "Artificial intelligence"},
	{"robotics", "/topic/mitrobotics-rss.xml", "Robotics"},
	{"space", "/topic/mitspace-rss.xml", "Space"},
	{"research", "/topic/mitresearch-rss.xml", "Research"},
	{"engineering", "/topic/mitengineering-rss.xml", "Engineering"},
	{"computers", "/topic/mitcomputers-rss.xml", "Computer Science and AI"},
	{"biology", "/topic/mitbiology-rss.xml", "Biology"},
	{"physics", "/topic/mitphysics-rss.xml", "Physics"},
	{"energy", "/topic/mitenergy-rss.xml", "Energy"},
	{"health", "/topic/mithealth-rss.xml", "Health"},
	{"education", "/topic/miteducation-rss.xml", "Education"},
	{"economics", "/topic/miteconomics-rss.xml", "Economics"},
	{"climate", "/topic/mitclimate-rss.xml", "Climate"},
	{"mathematics", "/topic/mitmathematics-rss.xml", "Mathematics"},
	{"business", "/topic/mitbusiness-rss.xml", "Business"},
	{"policy", "/topic/mitpolicy-rss.xml", "Policy"},
	{"politics", "/topic/mitpolitics-rss.xml", "Politics"},
	{"humanities", "/topic/mithumanities-rss.xml", "Humanities"},
	{"architecture", "/topic/mitarchitecture-rss.xml", "Architecture"},
	{"media", "/topic/mitmedia-rss.xml", "Media"},
	{"neuroscience", "/topic/mitneuroscience-rss.xml", "Neuroscience"},
	{"genetics", "/topic/mitgenetics-rss.xml", "Genetics"},
	{"mechanical", "/topic/mitmechanical-engineering-rss.xml", "Mechanical engineering"},
}

// Config holds constructor parameters for Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://news.mit.edu",
		UserAgent: "mit/dev (+https://github.com/tamnd/mitnews-cli)",
		Rate:      300 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client fetches MIT News RSS feeds.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	rate       time.Duration
	retries    int
	mu         sync.Mutex
	last       time.Time
}

// NewClient returns a Client configured by cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		userAgent:  cfg.UserAgent,
		rate:       cfg.Rate,
		retries:    cfg.Retries,
	}
}

// Article is the record emitted for MIT News articles.
type Article struct {
	Rank      int    `json:"rank"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Topic     string `json:"topic"`
	Published string `json:"published"`
	Summary   string `json:"summary"`
	URL       string `json:"url"`
}

// Topic is the record emitted by the topics command.
type Topic struct {
	Rank  int    `json:"rank"`
	Name  string `json:"name"`
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Latest fetches the main MIT News feed and returns up to limit articles.
// limit=0 returns all entries.
func (c *Client) Latest(ctx context.Context, limit int) ([]Article, error) {
	rawURL := c.baseURL + "/rss/feed"
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return parseArticles(body, rawURL, limit)
}

// Topic fetches the feed for the given topic key and returns up to limit articles.
// Returns ErrUnknownTopic if the topic key is not registered.
func (c *Client) TopicArticles(ctx context.Context, topic string, limit int) ([]Article, error) {
	fe, ok := feedByKey(topic)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownTopic, topic)
	}
	rawURL := c.baseURL + fe.path
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	arts, err := parseArticles(body, rawURL, limit)
	if err != nil {
		return nil, err
	}
	// tag each article with the topic key, overriding URL-derived segments
	// which are often year numbers (e.g. "2024") rather than topic names.
	for i := range arts {
		arts[i].Topic = fe.key
	}
	return arts, nil
}

// Search fetches the latest feed and returns articles whose title or summary
// contain query (case-insensitive). limit=0 returns all matches.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Article, error) {
	arts, err := c.Latest(ctx, 0)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(query)
	var out []Article
	for _, a := range arts {
		if strings.Contains(strings.ToLower(a.Title), q) ||
			strings.Contains(strings.ToLower(a.Summary), q) ||
			strings.Contains(strings.ToLower(a.Author), q) {
			out = append(out, a)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// Topics returns the static list of registered feed topics.
// No network request is made.
func (c *Client) Topics() []Topic {
	out := make([]Topic, len(feeds))
	for i, fe := range feeds {
		out[i] = Topic{
			Rank:  i + 1,
			Name:  fe.key,
			Label: fe.label,
			URL:   c.baseURL + fe.path,
		}
	}
	return out
}

// TopicNames returns just the list of valid topic key strings.
func TopicNames() []string {
	names := make([]string, len(feeds))
	for i, fe := range feeds {
		names[i] = fe.key
	}
	return names
}

// feedByKey looks up a feed entry by its topic key (case-insensitive).
func feedByKey(key string) (feedEntry, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, fe := range feeds {
		if fe.key == key {
			return fe, true
		}
	}
	return feedEntry{}, false
}

// parseArticles parses an RSS body into Articles, applying limit.
func parseArticles(body []byte, rawURL string, limit int) ([]Article, error) {
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse feed %s: %w", rawURL, err)
	}
	items := feed.Channel.Items
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	out := make([]Article, len(items))
	for i, it := range items {
		out[i] = itemToArticle(it, i+1)
	}
	return out, nil
}

// get fetches a URL with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/xml, application/rss+xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rate <= 0 {
		return
	}
	if wait := c.rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// ─── RSS 2.0 wire types ───────────────────────────────────────────────────────

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

// rssItem maps to each <item> in the feed.
// dc:creator maps to the local name "creator" (encoding/xml matches local name).
type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	PubDate     string   `xml:"pubDate"`
	Creator     string   `xml:"creator"`
	Description string   `xml:"description"`
	Categories  []string `xml:"category"`
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// parseDate parses an RSS pubDate and returns "2006-01-02". Falls back to the
// raw string on parse error.
func parseDate(s string) string {
	s = strings.TrimSpace(s)
	t, err := time.Parse(time.RFC1123Z, s)
	if err != nil {
		t, err = time.Parse(time.RFC1123, s)
		if err != nil {
			return s
		}
	}
	return t.UTC().Format("2006-01-02")
}

// topicFromCategories picks the first category slug from the item's categories,
// falling back to a URL-extracted segment.
func topicFromCategories(cats []string, u string) string {
	for _, c := range cats {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && !strings.Contains(c, " ") {
			return c
		}
	}
	return topicFromURL(u)
}

// topicFromURL extracts the first non-year path segment after the host.
func topicFromURL(u string) string {
	rest := u
	if idx := strings.Index(rest, "://"); idx >= 0 {
		rest = rest[idx+3:]
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[idx+1:]
	} else {
		return ""
	}
	seg := rest
	if idx := strings.Index(seg, "/"); idx >= 0 {
		seg = seg[:idx]
	}
	return strings.ToLower(seg)
}

// stripAndTruncate strips HTML tags, collapses common entities, and truncates
// to maxChars runes, appending "…" if truncated.
func stripAndTruncate(html string, maxChars int) string {
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", `"`)
	out = strings.ReplaceAll(out, "&#39;", "'")
	out = strings.ReplaceAll(out, "&apos;", "'")
	out = strings.TrimSpace(out)
	rs := []rune(out)
	if len(rs) > maxChars {
		return string(rs[:maxChars-1]) + "…"
	}
	return out
}

func itemToArticle(it rssItem, rank int) Article {
	topic := topicFromCategories(it.Categories, it.Link)
	return Article{
		Rank:      rank,
		Title:     strings.TrimSpace(it.Title),
		Author:    strings.TrimSpace(it.Creator),
		Topic:     topic,
		Published: parseDate(it.PubDate),
		Summary:   stripAndTruncate(it.Description, 150),
		URL:       strings.TrimSpace(it.Link),
	}
}
