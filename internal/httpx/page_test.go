package httpx_test

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

func TestParseSearch(t *testing.T) {
	cases := map[string]struct {
		q       string
		want    string // "" means no filter
		wantErr bool
	}{
		"absent":              {q: "", want: ""},
		"blank":               {q: "   ", want: ""},
		"trimmed":             {q: "  api ", want: "api"},
		"wildcards are plain": {q: `50%_off\`, want: `50\%\_off\\`},
		"too long":            {q: strings.Repeat("a", 101), wantErr: true},
		"limit itself":        {q: strings.Repeat("é", 100), want: strings.Repeat("é", 100)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?q="+url.QueryEscape(tc.q), nil)
			got, err := httpx.ParseSearch(r)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && got != nil || tc.want != "" && (got == nil || *got != tc.want) {
				t.Fatalf("got %v, want %q", got, tc.want)
			}
		})
	}
}
