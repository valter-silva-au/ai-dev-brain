package controlplane

import (
	"net/url"
	"reflect"
	"testing"
)

func TestReadOnlyDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		path            string
		hasVolume       bool
		hasSidecars     bool
		wantPath        string
		wantEscapedPath string
		wantQuery       url.Values
	}{
		{
			name:            "POSIX",
			path:            "/var/db/state.sqlite",
			wantPath:        "/var/db/state.sqlite",
			wantEscapedPath: "/var/db/state.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "POSIX literal backslash without volume",
			path:            `/var/db/literal\name.sqlite`,
			wantPath:        `/var/db/literal\name.sqlite`,
			wantEscapedPath: "/var/db/literal%5Cname.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "drive-qualified backslash path",
			path:            `C:\workspace\.aidb\state.sqlite`,
			hasVolume:       true,
			wantPath:        "/C:/workspace/.aidb/state.sqlite",
			wantEscapedPath: "/C:/workspace/.aidb/state.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "UNC",
			path:            `\\server\share\state.sqlite`,
			hasVolume:       true,
			wantPath:        "//server/share/state.sqlite",
			wantEscapedPath: "//server/share/state.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "spaces",
			path:            "/var/db/work space/state.sqlite",
			wantPath:        "/var/db/work space/state.sqlite",
			wantEscapedPath: "/var/db/work%20space/state.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "percent",
			path:            "/var/db/100%/state.sqlite",
			wantPath:        "/var/db/100%/state.sqlite",
			wantEscapedPath: "/var/db/100%25/state.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "query marker",
			path:            "/var/db/state?.sqlite",
			wantPath:        "/var/db/state?.sqlite",
			wantEscapedPath: "/var/db/state%3F.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "fragment marker",
			path:            "/var/db/state#.sqlite",
			wantPath:        "/var/db/state#.sqlite",
			wantEscapedPath: "/var/db/state%23.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "Unicode",
			path:            "/var/db/naïve/状态.sqlite",
			wantPath:        "/var/db/naïve/状态.sqlite",
			wantEscapedPath: "/var/db/na%C3%AFve/%E7%8A%B6%E6%80%81.sqlite",
			wantQuery: url.Values{
				"immutable": {"1"},
				"mode":      {"ro"},
			},
		},
		{
			name:            "sidecars keep mutable read-only mode",
			path:            "/var/db/state.sqlite",
			hasSidecars:     true,
			wantPath:        "/var/db/state.sqlite",
			wantEscapedPath: "/var/db/state.sqlite",
			wantQuery: url.Values{
				"mode": {"ro"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := url.Parse(readOnlyDSN(
				test.path,
				test.hasVolume,
				test.hasSidecars,
			))
			if err != nil {
				t.Fatalf("parse read-only DSN: %v", err)
			}
			if parsed.Scheme != "file" {
				t.Errorf("scheme = %q, want file", parsed.Scheme)
			}
			if parsed.Host != "" {
				t.Errorf("host = %q, want empty", parsed.Host)
			}
			if parsed.Path != test.wantPath {
				t.Errorf("path = %q, want %q", parsed.Path, test.wantPath)
			}
			if got := parsed.EscapedPath(); got != test.wantEscapedPath {
				t.Errorf(
					"escaped path = %q, want %q",
					got,
					test.wantEscapedPath,
				)
			}
			if got := parsed.Query(); !reflect.DeepEqual(got, test.wantQuery) {
				t.Errorf("query = %#v, want %#v", got, test.wantQuery)
			}
		})
	}
}
