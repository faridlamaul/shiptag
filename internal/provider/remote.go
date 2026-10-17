package provider

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ParseRemote extracts the host (without port or userinfo) and repository path
// (without leading slash or trailing .git) from a git remote URL.
func ParseRemote(raw string) (host, path string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("empty remote URL")
	}
	if !strings.Contains(raw, "://") {
		// scp-like: [user@]host:path
		hp, p, ok := strings.Cut(raw, ":")
		if !ok {
			return "", "", errors.New("unrecognized remote URL")
		}
		if i := strings.LastIndex(hp, "@"); i >= 0 {
			hp = hp[i+1:]
		}
		host, path = hp, p
	} else {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("unrecognized remote URL")
		}
		host, path = u.Hostname(), u.Path
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return "", "", errors.New("unrecognized remote URL")
	}
	return host, path, nil
}
