package uniref

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.BaseURL = srv.URL
	c.Rate = 0
	c.Retries = 0
	c.HTTP = &http.Client{Timeout: 5 * time.Second}
	return c
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearch(t *testing.T) {
	payload := wireSearchResponse{
		Results: []wireCluster{
			{
				ID:          "UniRef50_P04637",
				Name:        "Cluster: Cellular tumor antigen p53",
				EntryType:   "UniRef50",
				MemberCount: 23,
				Updated:     "2024-01-17",
			},
		},
	}
	payload.Results[0].CommonTaxon.ScientificName = "Homo sapiens"
	payload.Results[0].CommonTaxon.TaxonID = 9606
	payload.Results[0].RepresentativeMember.MemberID = "P53_HUMAN"
	payload.Results[0].RepresentativeMember.ProtName = "Cellular tumor antigen p53"
	payload.Results[0].RepresentativeMember.Accessions = []string{"P04637"}
	payload.Results[0].RepresentativeMember.Sequence.Length = 393
	payload.Results[0].RepresentativeMember.Sequence.MolWeight = 43653

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("query") == "" {
			t.Error("missing query param")
		}
		if q.Get("format") != "json" {
			t.Errorf("format = %q, want json", q.Get("format"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.Search(context.Background(), "TP53", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	got := results[0]
	if got.ID != "UniRef50_P04637" {
		t.Errorf("ID = %q, want UniRef50_P04637", got.ID)
	}
	if got.MemberCount != 23 {
		t.Errorf("MemberCount = %d, want 23", got.MemberCount)
	}
	if got.TaxonName != "Homo sapiens" {
		t.Errorf("TaxonName = %q, want Homo sapiens", got.TaxonName)
	}
	if got.RepAccession != "P04637" {
		t.Errorf("RepAccession = %q, want P04637", got.RepAccession)
	}
	if got.SeqLength != 393 {
		t.Errorf("SeqLength = %d, want 393", got.SeqLength)
	}
}

func TestGetCluster(t *testing.T) {
	payload := wireCluster{
		ID:          "UniRef50_A0A8X6L4D0",
		Name:        "Cluster: Tp53",
		EntryType:   "UniRef50",
		MemberCount: 1,
		Updated:     "2022-12-14",
	}
	payload.CommonTaxon.ScientificName = "Mus musculus"
	payload.CommonTaxon.TaxonID = 10090
	payload.RepresentativeMember.MemberID = "A0A8X6L4D0_TRICU"
	payload.RepresentativeMember.ProtName = "Tp53"
	payload.RepresentativeMember.Accessions = []string{"A0A8X6L4D0"}
	payload.RepresentativeMember.Sequence.Length = 390
	payload.RepresentativeMember.Sequence.MolWeight = 43200

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/UniRef50_A0A8X6L4D0" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("format = %q, want json", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.GetCluster(context.Background(), "UniRef50_A0A8X6L4D0")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "UniRef50_A0A8X6L4D0" {
		t.Errorf("ID = %q, want UniRef50_A0A8X6L4D0", got.ID)
	}
	if got.EntryType != "UniRef50" {
		t.Errorf("EntryType = %q, want UniRef50", got.EntryType)
	}
	if got.TaxonName != "Mus musculus" {
		t.Errorf("TaxonName = %q, want Mus musculus", got.TaxonName)
	}
	if got.MemberCount != 1 {
		t.Errorf("MemberCount = %d, want 1", got.MemberCount)
	}
	if got.RepAccession != "A0A8X6L4D0" {
		t.Errorf("RepAccession = %q, want A0A8X6L4D0", got.RepAccession)
	}
}

func TestGetMembers(t *testing.T) {
	payload := wireMembersResponse{
		Results: []wireMember{
			{
				MemberID:    "A0A8X6L4D0_TRICU",
				MemberType:  "UniProtKB Unreviewed (TrEMBL)",
				OrgName:     "Trichechus manatus latirostris",
				TaxonID:     127582,
				ProtName:    "Tp53",
				Accessions:  []string{"A0A8X6L4D0"},
				UniRef50ID:  "UniRef50_A0A8X6L4D0",
				UniRef90ID:  "UniRef90_A0A8X6L4D0",
				UniRef100ID: "UniRef100_A0A8X6L4D0",
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/UniRef50_A0A8X6L4D0/members" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("format = %q, want json", r.URL.Query().Get("format"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	members, err := c.GetMembers(context.Background(), "UniRef50_A0A8X6L4D0", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("len(members) = %d, want 1", len(members))
	}
	got := members[0]
	if got.ID != "A0A8X6L4D0_TRICU" {
		t.Errorf("ID = %q, want A0A8X6L4D0_TRICU", got.ID)
	}
	if got.Type != "UniProtKB Unreviewed (TrEMBL)" {
		t.Errorf("Type = %q", got.Type)
	}
	if got.Accession != "A0A8X6L4D0" {
		t.Errorf("Accession = %q, want A0A8X6L4D0", got.Accession)
	}
	if got.UniRef50 != "UniRef50_A0A8X6L4D0" {
		t.Errorf("UniRef50 = %q", got.UniRef50)
	}
}
