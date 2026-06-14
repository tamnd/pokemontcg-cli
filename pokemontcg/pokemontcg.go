// Package pokemontcg is the library behind the pokemontcg command line:
// the HTTP client, request shaping, and typed data models for the
// Pokemon TCG API (https://api.pokemontcg.io/v2).
//
// The API requires no key for basic access but rate-limits unauthenticated
// requests. The Client paces requests, sets a real User-Agent, and retries
// transient failures (429 and 5xx).
package pokemontcg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to the Pokemon TCG API.
const DefaultUserAgent = "pokemontcg-cli/0.1.0 (github.com/tamnd/pokemontcg-cli)"

// Host is the API hostname.
const Host = "api.pokemontcg.io"

// BaseURL is the root every API request is built from.
const BaseURL = "https://" + Host + "/v2"

// Card holds the flattened information about a Pokemon TCG card.
type Card struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Supertype   string   `json:"supertype"`
	Subtypes    []string `json:"subtypes,omitempty"`
	HP          string   `json:"hp,omitempty"`
	Types       []string `json:"types,omitempty"`
	EvolvesFrom string   `json:"evolvesFrom,omitempty"`
	Rarity      string   `json:"rarity,omitempty"`
	Number      string   `json:"number,omitempty"`
	Artist      string   `json:"artist,omitempty"`
	Set         CardSet  `json:"set"`
	ImageSmall  string   `json:"imageSmall,omitempty"`
	ImageLarge  string   `json:"imageLarge,omitempty"`
}

// CardSet is the set a card belongs to (embedded in Card).
type CardSet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Series      string `json:"series"`
	Total       int    `json:"total"`
	ReleaseDate string `json:"releaseDate,omitempty"`
}

// Set holds metadata for a Pokemon TCG expansion set.
type Set struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Series       string `json:"series"`
	PrintedTotal int    `json:"printedTotal"`
	Total        int    `json:"total"`
	PtcgoCode    string `json:"ptcgoCode,omitempty"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	SymbolURL    string `json:"symbolUrl,omitempty"`
	LogoURL      string `json:"logoUrl,omitempty"`
}

// --- wire types (API nesting) ---

type wireCard struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Supertype   string   `json:"supertype"`
	Subtypes    []string `json:"subtypes"`
	HP          string   `json:"hp"`
	Types       []string `json:"types"`
	EvolvesFrom string   `json:"evolvesFrom"`
	Rarity      string   `json:"rarity"`
	Number      string   `json:"number"`
	Artist      string   `json:"artist"`
	Set         struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Series      string `json:"series"`
		Total       int    `json:"total"`
		ReleaseDate string `json:"releaseDate"`
	} `json:"set"`
	Images struct {
		Small string `json:"small"`
		Large string `json:"large"`
	} `json:"images"`
}

type wireSet struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Series       string `json:"series"`
	PrintedTotal int    `json:"printedTotal"`
	Total        int    `json:"total"`
	PtcgoCode    string `json:"ptcgoCode"`
	ReleaseDate  string `json:"releaseDate"`
	Images       struct {
		Symbol string `json:"symbol"`
		Logo   string `json:"logo"`
	} `json:"images"`
}

type wireCardsResp struct {
	Data       []wireCard `json:"data"`
	TotalCount int        `json:"totalCount"`
	PageSize   int        `json:"pageSize"`
	Page       int        `json:"page"`
}

type wireSetsResp struct {
	Data       []wireSet `json:"data"`
	TotalCount int       `json:"totalCount"`
}

type wireStringsResp struct {
	Data []string `json:"data"`
}

// Client talks to the Pokemon TCG API over HTTPS.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   3,
	}
}

// SearchCards queries /v2/cards with an optional Lucene query string.
// If limit <= 0, up to 250 cards are returned (API max pageSize).
func (c *Client) SearchCards(ctx context.Context, query string, limit int) ([]Card, error) {
	pageSize := limit
	if pageSize <= 0 || pageSize > 250 {
		pageSize = 250
	}
	params := url.Values{}
	if query != "" {
		params.Set("q", query)
	}
	params.Set("pageSize", fmt.Sprintf("%d", pageSize))
	params.Set("page", "1")

	u := BaseURL + "/cards?" + params.Encode()
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w wireCardsResp
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse cards: %w", err)
	}
	out := make([]Card, 0, len(w.Data))
	for _, wc := range w.Data {
		out = append(out, flattenCard(wc))
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// GetCard fetches a single card by its ID (e.g. "base1-58").
func (c *Client) GetCard(ctx context.Context, id string) (*Card, error) {
	u := BaseURL + "/cards/" + url.PathEscape(id)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w struct {
		Data wireCard `json:"data"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse card %s: %w", id, err)
	}
	card := flattenCard(w.Data)
	return &card, nil
}

// ListSets returns all expansion sets. If limit <= 0 up to 250 are returned.
func (c *Client) ListSets(ctx context.Context, limit int) ([]Set, error) {
	pageSize := limit
	if pageSize <= 0 || pageSize > 250 {
		pageSize = 250
	}
	u := BaseURL + fmt.Sprintf("/sets?pageSize=%d&page=1", pageSize)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w wireSetsResp
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse sets: %w", err)
	}
	out := make([]Set, 0, len(w.Data))
	for _, ws := range w.Data {
		out = append(out, flattenSet(ws))
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// GetSet fetches a single expansion set by its ID (e.g. "base1").
func (c *Client) GetSet(ctx context.Context, id string) (*Set, error) {
	u := BaseURL + "/sets/" + url.PathEscape(id)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w struct {
		Data wireSet `json:"data"`
	}
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse set %s: %w", id, err)
	}
	s := flattenSet(w.Data)
	return &s, nil
}

// Types returns all card types (e.g. "Fire", "Water", "Grass").
func (c *Client) Types(ctx context.Context) ([]string, error) {
	return c.stringList(ctx, "/types")
}

// Rarities returns all card rarities (e.g. "Common", "Uncommon", "Rare Holo").
func (c *Client) Rarities(ctx context.Context) ([]string, error) {
	return c.stringList(ctx, "/rarities")
}

// Supertypes returns all card supertypes (e.g. "Pokémon", "Trainer", "Energy").
func (c *Client) Supertypes(ctx context.Context) ([]string, error) {
	return c.stringList(ctx, "/supertypes")
}

func (c *Client) stringList(ctx context.Context, path string) ([]string, error) {
	body, err := c.Get(ctx, BaseURL+path)
	if err != nil {
		return nil, err
	}
	var w wireStringsResp
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return w.Data, nil
}

// BuildCardQuery constructs a Lucene query string from name and set filters.
// For example: name=pikachu and set=base1 returns "name:pikachu set.id:base1".
func BuildCardQuery(name, setID string) string {
	var parts []string
	if name != "" {
		parts = append(parts, "name:"+name)
	}
	if setID != "" {
		parts = append(parts, "set.id:"+setID)
	}
	return strings.Join(parts, " ")
}

// Get fetches a URL and returns the body, pacing and retrying as configured.
func (c *Client) Get(ctx context.Context, u string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, u)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", u, lastErr)
}

func (c *Client) do(ctx context.Context, u string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
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

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
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

// --- flatten helpers ---

func flattenCard(wc wireCard) Card {
	return Card{
		ID:          wc.ID,
		Name:        wc.Name,
		Supertype:   wc.Supertype,
		Subtypes:    wc.Subtypes,
		HP:          wc.HP,
		Types:       wc.Types,
		EvolvesFrom: wc.EvolvesFrom,
		Rarity:      wc.Rarity,
		Number:      wc.Number,
		Artist:      wc.Artist,
		Set: CardSet{
			ID:          wc.Set.ID,
			Name:        wc.Set.Name,
			Series:      wc.Set.Series,
			Total:       wc.Set.Total,
			ReleaseDate: wc.Set.ReleaseDate,
		},
		ImageSmall: wc.Images.Small,
		ImageLarge: wc.Images.Large,
	}
}

func flattenSet(ws wireSet) Set {
	return Set{
		ID:           ws.ID,
		Name:         ws.Name,
		Series:       ws.Series,
		PrintedTotal: ws.PrintedTotal,
		Total:        ws.Total,
		PtcgoCode:    ws.PtcgoCode,
		ReleaseDate:  ws.ReleaseDate,
		SymbolURL:    ws.Images.Symbol,
		LogoURL:      ws.Images.Logo,
	}
}

// Page is kept for the kit domain resolver (ant get pokemontcg://page/...).
type Page struct {
	ID    string `json:"id" kit:"id"`
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty" kit:"body"`
}

// GetPage is a stub for the kit page resolver.
func (c *Client) GetPage(ctx context.Context, path string) (*Page, error) {
	u := "https://" + Host + "/" + path
	return &Page{ID: path, URL: u, Title: path}, nil
}

// PageLinks is a stub for the kit list op.
func (c *Client) PageLinks(ctx context.Context, path string, limit int) ([]*Page, error) {
	return nil, nil
}
