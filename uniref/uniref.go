// Package uniref is the library behind the uniref command line:
// the HTTP client, request shaping, and the typed data models for the
// UniRef REST API (UniProt Reference Clusters, 381M protein cluster records).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package uniref

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// DefaultUserAgent identifies the client to UniRef servers.
const DefaultUserAgent = "uniref-cli/0.1.0 (github.com/tamnd/uniref-cli)"

// Host is the site this client talks to.
const Host = "rest.uniprot.org"

// BaseURL is the root every UniRef request is built from.
const BaseURL = "https://" + Host + "/uniref"

// Config holds all tunable client parameters.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Client talks to the UniRef REST API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// NewClientFromConfig returns a Client configured from cfg.
func NewClientFromConfig(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	retries := cfg.Retries
	if retries == 0 {
		retries = 3
	}
	return &Client{
		HTTP:      &http.Client{Timeout: timeout},
		UserAgent: cfg.UserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   retries,
	}
}

// --- wire types (UniRef REST JSON structure) ---

type wireCluster struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Updated   string `json:"updated"`
	EntryType string `json:"entryType"`
	CommonTaxon struct {
		ScientificName string `json:"scientificName"`
		TaxonID        int    `json:"taxonId"`
	} `json:"commonTaxon"`
	MemberCount          int `json:"memberCount"`
	RepresentativeMember struct {
		MemberID   string   `json:"memberId"`
		MemberType string   `json:"memberIdType"`
		OrgName    string   `json:"organismName"`
		ProtName   string   `json:"proteinName"`
		Accessions []string `json:"accessions"`
		Sequence   struct {
			Length    int `json:"length"`
			MolWeight int `json:"molWeight"`
		} `json:"sequence"`
	} `json:"representativeMember"`
	SeedID    string   `json:"seedId"`
	Members   []string `json:"members"`
	Organisms []struct {
		ScientificName string `json:"scientificName"`
		TaxonID        int    `json:"taxonId"`
	} `json:"organisms"`
	GoTerms []struct {
		GoID   string `json:"goId"`
		Aspect string `json:"aspect"`
	} `json:"goTerms"`
}

type wireMember struct {
	MemberID    string   `json:"memberId"`
	MemberType  string   `json:"memberIdType"`
	OrgName     string   `json:"organismName"`
	TaxonID     int      `json:"taxonId"`
	ProtName    string   `json:"proteinName"`
	Accessions  []string `json:"accessions"`
	UniRef50ID  string   `json:"uniref50Id"`
	UniRef90ID  string   `json:"uniref90Id"`
	UniRef100ID string   `json:"uniref100Id"`
}

type wireSearchResponse struct {
	Results []wireCluster `json:"results"`
}

type wireMembersResponse struct {
	Results []wireMember `json:"results"`
}

// --- public output models ---

// Cluster is a flattened, output-friendly UniRef cluster record.
type Cluster struct {
	ID           string `json:"id"            kit:"id"`
	Name         string `json:"name"`
	EntryType    string `json:"entry_type"`
	MemberCount  int    `json:"member_count"`
	TaxonName    string `json:"taxon_name,omitempty"`
	TaxonID      int    `json:"taxon_id,omitempty"`
	Updated      string `json:"updated,omitempty"`
	RepName      string `json:"representative_name,omitempty"`
	RepProtein   string `json:"representative_protein,omitempty"`
	RepAccession string `json:"representative_accession,omitempty"`
	SeqLength    int    `json:"seq_length,omitempty"`
	MolWeight    int    `json:"mol_weight,omitempty"`
}

// Member is a single member entry within a UniRef cluster.
type Member struct {
	ID        string `json:"id"         kit:"id"` // memberId
	Type      string `json:"type"`
	Organism  string `json:"organism,omitempty"`
	Protein   string `json:"protein,omitempty"`
	Accession string `json:"accession,omitempty"`
	UniRef50  string `json:"uniref50,omitempty"`
	UniRef90  string `json:"uniref90,omitempty"`
	UniRef100 string `json:"uniref100,omitempty"`
}

// --- conversion helpers ---

func flattenCluster(w wireCluster) *Cluster {
	c := &Cluster{
		ID:          w.ID,
		Name:        w.Name,
		EntryType:   w.EntryType,
		MemberCount: w.MemberCount,
		TaxonName:   w.CommonTaxon.ScientificName,
		TaxonID:     w.CommonTaxon.TaxonID,
		Updated:     w.Updated,
		RepName:     w.RepresentativeMember.MemberID,
		RepProtein:  w.RepresentativeMember.ProtName,
		SeqLength:   w.RepresentativeMember.Sequence.Length,
		MolWeight:   w.RepresentativeMember.Sequence.MolWeight,
	}
	if len(w.RepresentativeMember.Accessions) > 0 {
		c.RepAccession = w.RepresentativeMember.Accessions[0]
	}
	return c
}

func flattenMember(w wireMember) *Member {
	m := &Member{
		ID:        w.MemberID,
		Type:      w.MemberType,
		Organism:  w.OrgName,
		Protein:   w.ProtName,
		UniRef50:  w.UniRef50ID,
		UniRef90:  w.UniRef90ID,
		UniRef100: w.UniRef100ID,
	}
	if len(w.Accessions) > 0 {
		m.Accession = w.Accessions[0]
	}
	return m
}

// --- client methods ---

// Search searches UniRef clusters by query string.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]*Cluster, error) {
	if limit <= 0 {
		limit = 10
	}
	u := c.BaseURL + "/search?query=" + url.QueryEscape(query) +
		"&format=json&size=" + fmt.Sprintf("%d", limit)

	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var result wireSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse search results: %w", err)
	}
	out := make([]*Cluster, 0, len(result.Results))
	for _, w := range result.Results {
		out = append(out, flattenCluster(w))
	}
	return out, nil
}

// GetCluster fetches a single cluster by ID (e.g. "UniRef50_P04637").
func (c *Client) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	u := c.BaseURL + "/" + url.PathEscape(id) + "?format=json"
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var w wireCluster
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, fmt.Errorf("parse cluster %s: %w", id, err)
	}
	return flattenCluster(w), nil
}

// GetMembers fetches members of a cluster by ID.
func (c *Client) GetMembers(ctx context.Context, id string, limit int) ([]*Member, error) {
	if limit <= 0 {
		limit = 10
	}
	u := c.BaseURL + "/" + url.PathEscape(id) + "/members?format=json&size=" + fmt.Sprintf("%d", limit)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var result wireMembersResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse members for %s: %w", id, err)
	}
	out := make([]*Member, 0, len(result.Results))
	for _, w := range result.Results {
		out = append(out, flattenMember(w))
	}
	return out, nil
}

// Get fetches the URL and returns the response body. It paces and retries
// according to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
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

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
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
