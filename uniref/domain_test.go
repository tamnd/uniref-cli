package uniref

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions,
// which need no network. The client's HTTP behaviour is covered in uniref_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "uniref" {
		t.Errorf("Scheme = %q, want uniref", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "uniref" {
		t.Errorf("Identity.Binary = %q, want uniref", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in      string
		typ     string
		id      string
		wantErr bool
	}{
		{"UniRef50_P04637", "cluster", "UniRef50_P04637", false},
		{"UniRef90_P04637", "cluster", "UniRef90_P04637", false},
		{"UniRef100_P04637", "cluster", "UniRef100_P04637", false},
		{"UniRef50_A0A8X6L4D0", "cluster", "UniRef50_A0A8X6L4D0", false},
		// invalid inputs
		{"P04637", "", "", true},
		{"TP53", "", "", true},
		{"uniref50_P04637", "", "", true}, // wrong case
		{"", "", "", true},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Classify(%q) = (%q, %q, nil), want error", tc.in, typ, id)
			}
			continue
		}
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType string
		id      string
		want    string
		wantErr bool
	}{
		{"cluster", "UniRef50_P04637", "https://www.uniprot.org/uniref/UniRef50_P04637", false},
		{"cluster", "UniRef90_P04637", "https://www.uniprot.org/uniref/UniRef90_P04637", false},
		{"cluster", "UniRef100_P04637", "https://www.uniprot.org/uniref/UniRef100_P04637", false},
		{"member", "P53_HUMAN", "", true},
		{"page", "foo", "", true},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Locate(%q, %q) = (%q, nil), want error", tc.uriType, tc.id, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}
