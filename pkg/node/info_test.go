package node

import (
	"testing"
)

func TestNormalizeNodeURI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "just IP",
			input: "127.0.0.1",
			want:  "http://127.0.0.1:9650",
		},
		{
			name:  "IP with port",
			input: "127.0.0.1:9650",
			want:  "http://127.0.0.1:9650",
		},
		{
			name:  "IP with custom port",
			input: "192.168.1.1:8080",
			want:  "https://192.168.1.1:8080",
		},
		{
			name:  "full HTTP URI",
			input: "http://127.0.0.1:9650",
			want:  "http://127.0.0.1:9650",
		},
		{
			name:  "full HTTPS URI",
			input: "https://api.avax.network",
			want:  "https://api.avax.network",
		},
		{
			name:  "hostname only",
			input: "mynode.example.com",
			want:  "https://mynode.example.com:9650",
		},
		{
			name:  "hostname with port",
			input: "mynode.example.com:9650",
			want:  "https://mynode.example.com:9650",
		},
		{
			name:  "full URI with ext info path",
			input: "http://127.0.0.1:9650/ext/info",
			want:  "http://127.0.0.1:9650",
		},
		{
			name:  "full URI with ext info trailing slash",
			input: "http://127.0.0.1:9650/ext/info/",
			want:  "http://127.0.0.1:9650",
		},
		{
			name:  "localhost",
			input: "localhost",
			want:  "http://localhost:9650",
		},
		{
			name:  "localhost with port",
			input: "localhost:9650",
			want:  "http://localhost:9650",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNodeURI(tt.input)
			if err != nil {
				t.Fatalf("NormalizeNodeURI(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeNodeURI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeNodeURI_IPv6(t *testing.T) {
	// IPv6 addresses with brackets should be handled correctly
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "IPv6 with brackets and port",
			input: "[::1]:9650",
			want:  "http://[::1]:9650",
		},
		{
			name:  "IPv6 full URI",
			input: "http://[::1]:9650",
			want:  "http://[::1]:9650",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNodeURI(tt.input)
			if err != nil {
				t.Fatalf("NormalizeNodeURI(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeNodeURI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeNodeURI_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty",
			input: "",
		},
		{
			name:  "unsupported scheme",
			input: "ftp://127.0.0.1:9650",
		},
		{
			name:  "custom path not allowed",
			input: "http://127.0.0.1:9650/custom/path",
		},
		{
			name:  "query not allowed",
			input: "http://127.0.0.1:9650?x=1",
		},
		{
			name:  "fragment not allowed",
			input: "http://127.0.0.1:9650#frag",
		},
		{
			name:  "host shorthand with path",
			input: "127.0.0.1:9650/ext/info",
		},
		{
			name:  "non-local http disallowed by default",
			input: "http://mynode.example.com:9650",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeNodeURI(tt.input)
			if err == nil {
				t.Fatalf("NormalizeNodeURI(%q) expected error", tt.input)
			}
		})
	}
}

func TestNormalizeNodeURI_AllowInsecureHTTP(t *testing.T) {
	got, err := NormalizeNodeURIWithInsecureHTTP("http://mynode.example.com:9650", true)
	if err != nil {
		t.Fatalf("NormalizeNodeURIWithInsecureHTTP() returned error: %v", err)
	}
	if got != "http://mynode.example.com:9650" {
		t.Fatalf("NormalizeNodeURIWithInsecureHTTP() = %q, want %q", got, "http://mynode.example.com:9650")
	}
}
