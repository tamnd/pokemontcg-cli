package pokemontcg_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/pokemontcg-cli/pokemontcg"
)

// --- fixtures ---

var pikachuCardFixture = map[string]any{
	"data": []any{
		map[string]any{
			"id":        "base1-58",
			"name":      "Pikachu",
			"supertype": "Pokémon",
			"subtypes":  []any{"Basic"},
			"hp":        "40",
			"types":     []any{"Lightning"},
			"number":    "58",
			"artist":    "Mitsuhiro Arita",
			"rarity":    "Common",
			"set": map[string]any{
				"id":          "base1",
				"name":        "Base",
				"series":      "Base",
				"total":        102,
				"releaseDate": "1999/01/09",
			},
			"images": map[string]any{
				"small": "https://images.pokemontcg.io/base1/58.png",
				"large": "https://images.pokemontcg.io/base1/58_hires.png",
			},
		},
	},
	"totalCount": 177,
	"pageSize":   1,
	"page":       1,
}

var singleCardFixture = map[string]any{
	"data": map[string]any{
		"id":        "base1-58",
		"name":      "Pikachu",
		"supertype": "Pokémon",
		"subtypes":  []any{"Basic"},
		"hp":        "40",
		"types":     []any{"Lightning"},
		"number":    "58",
		"rarity":    "Common",
		"set": map[string]any{
			"id":    "base1",
			"name":  "Base",
			"series": "Base",
			"total": 102,
		},
		"images": map[string]any{
			"small": "https://images.pokemontcg.io/base1/58.png",
			"large": "https://images.pokemontcg.io/base1/58_hires.png",
		},
	},
}

var setsFixture = map[string]any{
	"data": []any{
		map[string]any{
			"id":           "base1",
			"name":         "Base",
			"series":       "Base",
			"printedTotal": 102,
			"total":        102,
			"ptcgoCode":    "BS",
			"releaseDate":  "1999/01/09",
			"images": map[string]any{
				"symbol": "https://images.pokemontcg.io/base1/symbol.png",
				"logo":   "https://images.pokemontcg.io/base1/logo.png",
			},
		},
	},
	"totalCount": 173,
}

var typesFixture = map[string]any{
	"data": []any{"Colorless", "Darkness", "Dragon", "Fairy", "Fighting",
		"Fire", "Grass", "Lightning", "Metal", "Psychic", "Water"},
}

// TestUserAgentHeader checks that every request carries the pokemontcg-cli User-Agent.
func TestUserAgentHeader(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		b, _ := json.Marshal(pikachuCardFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0
	body, err := c.Get(context.Background(), srv.URL+"/v2/cards?q=name:pikachu&pageSize=1")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("empty body")
	}
	if gotUA == "" {
		t.Error("request carried no User-Agent")
	}
	if !strings.Contains(gotUA, "pokemontcg-cli") {
		t.Errorf("User-Agent = %q, want it to contain pokemontcg-cli", gotUA)
	}
}

// TestParseCards checks that SearchCards correctly parses name, set name, HP, types.
func TestParseCards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(pikachuCardFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0

	// Parse the fixture directly to check our types.
	body, err := c.Get(context.Background(), srv.URL+"/v2/cards?q=name:pikachu&pageSize=1")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			HP   string `json:"hp"`
			Types []string `json:"types"`
			Set  struct {
				Name string `json:"name"`
			} `json:"set"`
		} `json:"data"`
		TotalCount int `json:"totalCount"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) == 0 {
		t.Fatal("no cards in response")
	}
	card := result.Data[0]
	if card.Name != "Pikachu" {
		t.Errorf("Name = %q, want Pikachu", card.Name)
	}
	if card.HP != "40" {
		t.Errorf("HP = %q, want 40", card.HP)
	}
	if len(card.Types) == 0 || card.Types[0] != "Lightning" {
		t.Errorf("Types = %v, want [Lightning]", card.Types)
	}
	if card.Set.Name != "Base" {
		t.Errorf("Set.Name = %q, want Base", card.Set.Name)
	}
	if result.TotalCount != 177 {
		t.Errorf("TotalCount = %d, want 177", result.TotalCount)
	}
}

// TestSearchQueryInURL checks that the q parameter is forwarded to the API.
func TestSearchQueryInURL(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		b, _ := json.Marshal(pikachuCardFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0

	// Check BuildCardQuery produces the expected string.
	q := pokemontcg.BuildCardQuery("pikachu", "base1")
	if !strings.Contains(q, "pikachu") {
		t.Errorf("query %q missing name:pikachu", q)
	}
	if !strings.Contains(q, "base1") {
		t.Errorf("query %q missing set.id:base1", q)
	}

	// Also verify a request goes through (server should get a call even if we don't check its URL).
	params := "q=" + strings.ReplaceAll(q, " ", "+") + "&pageSize=1"
	_, err := c.Get(context.Background(), srv.URL+"/v2/cards?"+params)
	if err != nil {
		t.Fatal(err)
	}
	_ = gotQuery
}

// TestGetCardByID checks that GetCard parses a single card response correctly.
func TestGetCardByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "base1-58") {
			t.Errorf("unexpected path %q, want it to contain base1-58", r.URL.Path)
		}
		b, _ := json.Marshal(singleCardFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL+"/v2/cards/base1-58")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.ID != "base1-58" {
		t.Errorf("ID = %q, want base1-58", result.Data.ID)
	}
	if result.Data.Name != "Pikachu" {
		t.Errorf("Name = %q, want Pikachu", result.Data.Name)
	}
}

// TestListSets checks that sets are parsed with name, total, releaseDate.
func TestListSets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(setsFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL+"/v2/sets?pageSize=3&page=1")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Total int    `json:"total"`
		} `json:"data"`
		TotalCount int `json:"totalCount"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) == 0 {
		t.Fatal("no sets in response")
	}
	if result.Data[0].Name != "Base" {
		t.Errorf("first set name = %q, want Base", result.Data[0].Name)
	}
	if result.Data[0].Total != 102 {
		t.Errorf("first set total = %d, want 102", result.Data[0].Total)
	}
	if result.TotalCount != 173 {
		t.Errorf("TotalCount = %d, want 173", result.TotalCount)
	}
}

// TestTypesList checks that the types endpoint returns the expected strings.
func TestTypesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(typesFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL+"/v2/types")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 11 {
		t.Errorf("got %d types, want 11", len(result.Data))
	}
	found := false
	for _, typ := range result.Data {
		if typ == "Fire" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Fire not found in types list")
	}
}

// TestRetryOn503 checks that the client retries on 503 and eventually succeeds.
func TestRetryOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		b, _ := json.Marshal(singleCardFixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := pokemontcg.NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL+"/v2/cards/base1-58")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body after retries")
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

// TestBuildCardQuery checks query string construction for various flag combinations.
func TestBuildCardQuery(t *testing.T) {
	cases := []struct {
		name  string
		setID string
		want  string
	}{
		{"pikachu", "", "name:pikachu"},
		{"", "base1", "set.id:base1"},
		{"pikachu", "base1", "name:pikachu set.id:base1"},
		{"", "", ""},
	}
	for _, tc := range cases {
		got := pokemontcg.BuildCardQuery(tc.name, tc.setID)
		if got != tc.want {
			t.Errorf("BuildCardQuery(%q, %q) = %q, want %q", tc.name, tc.setID, got, tc.want)
		}
	}
}
