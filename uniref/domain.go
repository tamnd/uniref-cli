package uniref

import (
	"context"
	"regexp"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes uniref as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/uniref-cli/uniref"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// uniref:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone uniref binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the uniref driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "uniref",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "uniref",
			Short:  "A command line for UniRef protein cluster database.",
			Long: `A command line for UniRef protein cluster database.

uniref reads public UniRef data over HTTPS, shapes it into clean records,
and prints output that pipes into the rest of your tools. No API key,
nothing to run alongside it. UniRef groups UniProt sequences into clusters
at 50%, 90%, and 100% identity thresholds across 381M records.`,
			Site: Host,
			Repo: "https://github.com/tamnd/uniref-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: full-text search over UniRef clusters.
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		List:    true,
		Summary: "Search UniRef clusters by gene name, keyword, or query",
		URIType: "cluster",
		Args:    []kit.Arg{{Name: "query", Help: "search query (e.g. TP53, BRCA1 AND organism_id:9606)"}},
	}, searchClusters)

	// cluster: fetch one cluster record by UniRef ID.
	kit.Handle(app, kit.OpMeta{
		Name:     "cluster",
		Group:    "read",
		Single:   true,
		Summary:  "Fetch a UniRef cluster by ID (e.g. UniRef50_P04637)",
		URIType:  "cluster",
		Resolver: true,
		Args:     []kit.Arg{{Name: "id", Help: "UniRef cluster ID (e.g. UniRef50_P04637)"}},
	}, getCluster)

	// members: list members of a cluster.
	kit.Handle(app, kit.OpMeta{
		Name:    "members",
		Group:   "read",
		List:    true,
		Summary: "List members of a UniRef cluster",
		URIType: "member",
		Args:    []kit.Arg{{Name: "id", Help: "UniRef cluster ID (e.g. UniRef50_P04637)"}},
	}, getMembers)
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

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"        help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results to return"`
	Client *Client `kit:"inject"`
}

type clusterRef struct {
	ID     string  `kit:"arg"   help:"UniRef cluster ID (e.g. UniRef50_P04637)"`
	Client *Client `kit:"inject"`
}

type membersInput struct {
	ID     string  `kit:"arg"         help:"UniRef cluster ID"`
	Limit  int     `kit:"flag,inherit" help:"max members to return"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchClusters(ctx context.Context, in searchInput, emit func(*Cluster) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	results, err := in.Client.Search(ctx, in.Query, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, c := range results {
		if err := emit(c); err != nil {
			return err
		}
	}
	return nil
}

func getCluster(ctx context.Context, in clusterRef, emit func(*Cluster) error) error {
	c, err := in.Client.GetCluster(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(c)
}

func getMembers(ctx context.Context, in membersInput, emit func(*Member) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	members, err := in.Client.GetMembers(ctx, in.ID, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, m := range members {
		if err := emit(m); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI string functions, pure and network-free ---

// unirefIDRE matches a UniRef cluster ID: UniRef50_*, UniRef90_*, UniRef100_*.
var unirefIDRE = regexp.MustCompile(`^UniRef(50|90|100)_\S+$`)

// Classify turns any accepted input into (type, id).
// Inputs matching UniRef(50|90|100)_... pattern map to type "cluster".
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if unirefIDRE.MatchString(input) {
		return "cluster", input, nil
	}
	return "", "", errs.Usage("unrecognized UniRef reference: %q (expected cluster ID like UniRef50_P04637)", input)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "cluster" {
		return "", errs.Usage("uniref has no resource type %q", uriType)
	}
	return "https://www.uniprot.org/uniref/" + id, nil
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
