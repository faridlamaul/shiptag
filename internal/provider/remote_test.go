package provider

import "testing"

func TestParseRemote(t *testing.T) {
	for _, tc := range []struct{ in, host, path string }{
		{"git@github.com:o/r.git", "github.com", "o/r"},
		{"ssh://git@gitlab.example.com:2222/g/s/p.git", "gitlab.example.com", "g/s/p"},
		{"https://user@bitbucket.org/ws/r.git", "bitbucket.org", "ws/r"},
		{"https://github.com/o/r", "github.com", "o/r"},
	} {
		h, p, err := ParseRemote(tc.in)
		if err != nil || h != tc.host || p != tc.path {
			t.Errorf("%s: got %q %q %v", tc.in, h, p, err)
		}
	}
	if _, _, err := ParseRemote(""); err == nil {
		t.Error("want error for empty")
	}
}
