package pokemontcg

import (
	"context"
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes pokemontcg as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/pokemontcg-cli/pokemontcg"
//
// The init below registers it; the host dereferences pokemontcg:// URIs by
// routing to the operations Register installs. The same Domain also builds the
// standalone pokemontcg binary (see cli.NewApp), so the binary and a host share
// one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the pokemontcg driver.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "pokemontcg",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "pokemontcg",
			Short:  "A command line for the Pokemon TCG API.",
			Long: `A command line for the Pokemon Trading Card Game API.

pokemontcg reads public data over HTTPS, shapes it into clean records,
and prints output that pipes into the rest of your tools. No API key required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/pokemontcg-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// cards: search or list cards.
	kit.Handle(app, kit.OpMeta{Name: "cards", Group: "read", List: true,
		Summary: "Search or list Pokemon cards (--name, --set flags)",
		URIType: "card"}, listCards)

	// card: get a single card by ID.
	kit.Handle(app, kit.OpMeta{Name: "card", Group: "read", Single: true,
		Summary: "Get a card by ID (e.g. base1-58)",
		URIType: "card", Resolver: true,
		Args: []kit.Arg{{Name: "id", Help: "card ID"}}}, getCard)

	// sets: list all expansion sets.
	kit.Handle(app, kit.OpMeta{Name: "sets", Group: "read", List: true,
		Summary: "List all expansion sets",
		URIType: "set"}, listSets)

	// set: get a single set by ID.
	kit.Handle(app, kit.OpMeta{Name: "set", Group: "read", Single: true,
		Summary: "Get a set by ID (e.g. base1)",
		URIType: "set",
		Args:    []kit.Arg{{Name: "id", Help: "set ID"}}}, getSet)

	// types: list all card types.
	kit.Handle(app, kit.OpMeta{Name: "types", Group: "read", List: true,
		Summary: "List all card types",
		URIType: "type"}, listTypes)

	// rarities: list all card rarities.
	kit.Handle(app, kit.OpMeta{Name: "rarities", Group: "read", List: true,
		Summary: "List all card rarities",
		URIType: "rarity"}, listRarities)

	// page resolver (for ant get pokemontcg://page/<path>)
	kit.Handle(app, kit.OpMeta{Name: "page", Group: "read", Single: true,
		Summary: "Fetch a page by path or URL", URIType: "page", Resolver: true,
		Args: []kit.Arg{{Name: "ref", Help: "page path or URL"}}}, getPage)

	// links list (for ant ls)
	kit.Handle(app, kit.OpMeta{Name: "links", Group: "read", List: true,
		Summary: "List the pages a page links to", URIType: "page",
		Args: []kit.Arg{{Name: "ref", Help: "page path or URL"}}}, listLinks)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- input structs ---

type cardsInput struct {
	Name   string  `kit:"flag" help:"filter by card name (Lucene: name:NAME)"`
	Set    string  `kit:"flag" help:"filter by set ID (Lucene: set.id:SET)"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type cardInput struct {
	ID     string  `kit:"arg" help:"card ID (e.g. base1-58)"`
	Client *Client `kit:"inject"`
}

type setsInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type setInput struct {
	ID     string  `kit:"arg" help:"set ID (e.g. base1)"`
	Client *Client `kit:"inject"`
}

type noInput struct {
	Client *Client `kit:"inject"`
}

type pageRef struct {
	Ref    string  `kit:"arg" help:"page path or URL"`
	Client *Client `kit:"inject"`
}

type listRef struct {
	Ref    string  `kit:"arg" help:"page path or URL"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// StringRecord wraps a single string value for list endpoints that return []string.
type StringRecord struct {
	Value string `json:"value" kit:"id"`
}

// --- handlers ---

func listCards(ctx context.Context, in cardsInput, emit func(*Card) error) error {
	q := BuildCardQuery(in.Name, in.Set)
	cards, err := in.Client.SearchCards(ctx, q, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range cards {
		if err := emit(&cards[i]); err != nil {
			return err
		}
	}
	return nil
}

func getCard(ctx context.Context, in cardInput, emit func(*Card) error) error {
	card, err := in.Client.GetCard(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(card)
}

func listSets(ctx context.Context, in setsInput, emit func(*Set) error) error {
	sets, err := in.Client.ListSets(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range sets {
		if err := emit(&sets[i]); err != nil {
			return err
		}
	}
	return nil
}

func getSet(ctx context.Context, in setInput, emit func(*Set) error) error {
	s, err := in.Client.GetSet(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(s)
}

func listTypes(ctx context.Context, in noInput, emit func(*StringRecord) error) error {
	types, err := in.Client.Types(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, t := range types {
		if err := emit(&StringRecord{Value: t}); err != nil {
			return err
		}
	}
	return nil
}

func listRarities(ctx context.Context, in noInput, emit func(*StringRecord) error) error {
	rarities, err := in.Client.Rarities(ctx)
	if err != nil {
		return mapErr(err)
	}
	for _, r := range rarities {
		if err := emit(&StringRecord{Value: r}); err != nil {
			return err
		}
	}
	return nil
}

func getPage(ctx context.Context, in pageRef, emit func(*Page) error) error {
	p, err := in.Client.GetPage(ctx, pagePath(in.Ref))
	if err != nil {
		return mapErr(err)
	}
	return emit(p)
}

func listLinks(ctx context.Context, in listRef, emit func(*Page) error) error {
	pages, err := in.Client.PageLinks(ctx, pagePath(in.Ref), in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, p := range pages {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

// Classify turns any accepted input into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = pagePath(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized pokemontcg reference: %q", input)
	}
	return "page", id, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "page", "card", "set", "type", "rarity":
		return "https://" + Host + "/" + strings.Trim(id, "/"), nil
	default:
		return "", errs.Usage("pokemontcg has no resource type %q", uriType)
	}
}

// --- helpers ---

func pagePath(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return strings.Trim(u.Path, "/")
	}
	return strings.Trim(input, "/")
}

func mapErr(err error) error {
	return err
}
