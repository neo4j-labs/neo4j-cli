// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package urlcheck

import (
	"strings"
	"testing"
)

func TestValidateRemoteURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "https to public host accepted",
			input:   "https://api.neo4j.io",
			wantErr: false,
		},
		{
			name:    "https with path and query accepted",
			input:   "https://api.openai.com/v1/embeddings?x=1",
			wantErr: false,
		},
		{
			name:    "http to localhost accepted",
			input:   "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "http to 127.0.0.1 accepted",
			input:   "http://127.0.0.1:11434",
			wantErr: false,
		},
		{
			name:    "http to ipv6 loopback accepted",
			input:   "http://[::1]:8080",
			wantErr: false,
		},
		{
			name:    "http to localhost with trailing dot accepted",
			input:   "http://localhost.:8080",
			wantErr: false,
		},
		{
			name:      "http to public host rejected",
			input:     "http://api.openai.com",
			wantErr:   true,
			errSubstr: "non-loopback",
		},
		{
			name:      "http to private rfc1918 ip rejected",
			input:     "http://10.0.0.1",
			wantErr:   true,
			errSubstr: "private network",
		},
		{
			name:      "http to 192.168 rfc1918 rejected",
			input:     "http://192.168.1.1",
			wantErr:   true,
			errSubstr: "private network",
		},
		{
			name:      "http to 172.16 rfc1918 rejected",
			input:     "http://172.16.5.5",
			wantErr:   true,
			errSubstr: "private network",
		},
		{
			name:      "https to private rfc1918 ip rejected",
			input:     "https://10.0.0.1",
			wantErr:   true,
			errSubstr: "private network",
		},
		{
			name:      "metadata IP rejected over http",
			input:     "http://169.254.169.254",
			wantErr:   true,
			errSubstr: "metadata IP",
		},
		{
			name:      "metadata IP rejected over https",
			input:     "https://169.254.169.254/latest/meta-data/",
			wantErr:   true,
			errSubstr: "metadata IP",
		},
		{
			name:      "metadata IP with trailing dot rejected",
			input:     "https://169.254.169.254./",
			wantErr:   true,
			errSubstr: "metadata IP",
		},
		{
			name:      "ipv6 link-local rejected",
			input:     "http://[fe80::1]",
			wantErr:   true,
			errSubstr: "link-local",
		},
		{
			name:      "ipv4 link-local rejected (other than metadata)",
			input:     "http://169.254.0.1",
			wantErr:   true,
			errSubstr: "link-local",
		},
		{
			name:      "multicast rejected",
			input:     "http://224.0.0.1",
			wantErr:   true,
			errSubstr: "multicast",
		},
		{
			name:      "unspecified ipv4 rejected",
			input:     "http://0.0.0.0",
			wantErr:   true,
			errSubstr: "unspecified",
		},
		{
			name:      "empty string rejected",
			input:     "",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "whitespace-only rejected",
			input:     "   ",
			wantErr:   true,
			errSubstr: "empty",
		},
		{
			name:      "ftp scheme rejected",
			input:     "ftp://example.com",
			wantErr:   true,
			errSubstr: "scheme",
		},
		{
			name:      "file scheme rejected",
			input:     "file:///etc/passwd",
			wantErr:   true,
			errSubstr: "scheme",
		},
		{
			name:      "no scheme rejected",
			input:     "example.com",
			wantErr:   true,
			errSubstr: "scheme",
		},
		{
			name:      "malformed url rejected",
			input:     "http://%41:8080/",
			wantErr:   true,
			errSubstr: "malformed",
		},
		{
			name:      "no host rejected",
			input:     "https:///path",
			wantErr:   true,
			errSubstr: "no host",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRemoteURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error for %q, got %v", tc.input, err)
			}
		})
	}
}
