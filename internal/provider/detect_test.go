package provider

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDetect(t *testing.T) {
	ghEnv := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "o/r", "GITHUB_SHA": "abc",
		"GITHUB_SERVER_URL": "https://ghe.x", "GITHUB_API_URL": "https://ghe.x/api/v3", "GITHUB_TOKEN": "t"}
	glEnv := map[string]string{"GITLAB_CI": "true", "CI_PROJECT_PATH": "g/s/p", "CI_COMMIT_SHA": "abc",
		"CI_SERVER_URL": "https://gl.x", "CI_API_V4_URL": "https://gl.x/api/v4", "CI_JOB_TOKEN": "j"}
	glEnvPAT := map[string]string{}
	for k, v := range glEnv {
		glEnvPAT[k] = v
	}
	glEnvPAT["GITLAB_TOKEN"] = "p"

	tests := []struct {
		name string
		in   DetectInput
		want Info
	}{
		{"github ci", DetectInput{Getenv: env(ghEnv)},
			Info{Name: "github", Repo: "o/r", DefaultRef: "abc", WebURL: "https://ghe.x", APIURL: "https://ghe.x/api/v3", Token: "t", InCI: true}},
		{"github GH_TOKEN", DetectInput{Getenv: env(map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "o/r", "GH_TOKEN": "t2"})},
			Info{Name: "github", Repo: "o/r", DefaultRef: "HEAD", WebURL: "https://github.com", APIURL: "https://api.github.com", Token: "t2", InCI: true}},
		{"gitlab job token", DetectInput{Getenv: env(glEnv)},
			Info{Name: "gitlab", Repo: "g/s/p", DefaultRef: "abc", WebURL: "https://gl.x", APIURL: "https://gl.x/api/v4", Token: "j", TokenKind: "job", InCI: true}},
		{"gitlab pat wins", DetectInput{Getenv: env(glEnvPAT)},
			Info{Name: "gitlab", Repo: "g/s/p", DefaultRef: "abc", WebURL: "https://gl.x", APIURL: "https://gl.x/api/v4", Token: "p", InCI: true}},
		{"bitbucket ci", DetectInput{Getenv: env(map[string]string{"BITBUCKET_BUILD_NUMBER": "7", "BITBUCKET_REPO_FULL_NAME": "ws/r", "BITBUCKET_COMMIT": "abc", "BITBUCKET_TOKEN": "b"})},
			Info{Name: "bitbucket", Repo: "ws/r", DefaultRef: "abc", WebURL: "https://bitbucket.org", APIURL: "https://api.bitbucket.org/2.0", Token: "b", InCI: true}},
		{"gitlab remote", DetectInput{RemoteURL: "git@gitlab.example.com:g/p.git", Getenv: env(nil)},
			Info{Name: "gitlab", Repo: "g/p", DefaultRef: "HEAD", WebURL: "https://gitlab.example.com", APIURL: "https://gitlab.example.com/api/v4"}},
		{"github remote", DetectInput{RemoteURL: "git@github.com:o/r.git", Getenv: env(nil)},
			Info{Name: "github", Repo: "o/r", DefaultRef: "HEAD", WebURL: "https://github.com", APIURL: "https://api.github.com"}},
		{"flags override env", DetectInput{Provider: "gitlab", Repo: "x/y", BaseURL: "https://gl.y", Getenv: env(ghEnv)},
			Info{Name: "gitlab", Repo: "x/y", DefaultRef: "HEAD", WebURL: "https://gl.y", APIURL: "https://gl.y/api/v4"}},
		{"github base url flag", DetectInput{Provider: "github", BaseURL: "https://ghe.y", Getenv: env(ghEnv)},
			Info{Name: "github", Repo: "o/r", DefaultRef: "abc", WebURL: "https://ghe.y", APIURL: "https://ghe.y/api/v3", Token: "t", InCI: true}},
		{"api url flag wins", DetectInput{APIURL: "https://api.custom/v3", Getenv: env(ghEnv)},
			Info{Name: "github", Repo: "o/r", DefaultRef: "abc", WebURL: "https://ghe.x", APIURL: "https://api.custom/v3", Token: "t", InCI: true}},
		{"token flag wins", DetectInput{Token: "flag", Getenv: env(glEnv)},
			Info{Name: "gitlab", Repo: "g/s/p", DefaultRef: "abc", WebURL: "https://gl.x", APIURL: "https://gl.x/api/v4", Token: "flag", InCI: true}},
		{"missing token ok", DetectInput{RemoteURL: "https://github.com/o/r", Getenv: env(nil)},
			Info{Name: "github", Repo: "o/r", DefaultRef: "HEAD", WebURL: "https://github.com", APIURL: "https://api.github.com"}},
		{"bitbucket remote", DetectInput{RemoteURL: "https://user@bitbucket.org/ws/r.git", Getenv: env(nil)},
			Info{Name: "bitbucket", Repo: "ws/r", DefaultRef: "HEAD", WebURL: "https://bitbucket.org", APIURL: "https://api.bitbucket.org/2.0"}},
		{"untrusted remote host ignores env token", DetectInput{RemoteURL: "git@github.evil.example:o/r.git", Getenv: env(map[string]string{"GITHUB_TOKEN": "t"})},
			Info{Name: "github", Repo: "o/r", DefaultRef: "HEAD", WebURL: "https://github.evil.example", APIURL: "https://github.evil.example/api/v3"}},
		{"confirmed remote host uses env token", DetectInput{RemoteURL: "git@github.evil.example:o/r.git", BaseURL: "https://github.evil.example", Getenv: env(map[string]string{"GITHUB_TOKEN": "t"})},
			Info{Name: "github", Repo: "o/r", DefaultRef: "HEAD", WebURL: "https://github.evil.example", APIURL: "https://github.evil.example/api/v3", Token: "t"}},
		{"self-hosted gitlab remote ignores env token", DetectInput{RemoteURL: "git@gitlab.example.com:g/p.git", Getenv: env(map[string]string{"GITLAB_TOKEN": "p"})},
			Info{Name: "gitlab", Repo: "g/p", DefaultRef: "HEAD", WebURL: "https://gitlab.example.com", APIURL: "https://gitlab.example.com/api/v4"}},
		{"self-hosted gitlab remote token flag", DetectInput{RemoteURL: "git@gitlab.example.com:g/p.git", Token: "flag", Getenv: env(map[string]string{"GITLAB_TOKEN": "p"})},
			Info{Name: "gitlab", Repo: "g/p", DefaultRef: "HEAD", WebURL: "https://gitlab.example.com", APIURL: "https://gitlab.example.com/api/v4", Token: "flag"}},
		{"self-hosted gitlab api url flag uses env token", DetectInput{RemoteURL: "git@gitlab.example.com:g/p.git", APIURL: "https://gitlab.example.com/api/v4", Getenv: env(map[string]string{"GITLAB_TOKEN": "p"})},
			Info{Name: "gitlab", Repo: "g/p", DefaultRef: "HEAD", WebURL: "https://gitlab.example.com", APIURL: "https://gitlab.example.com/api/v4", Token: "p"}},
		{"public github remote uses env token", DetectInput{RemoteURL: "git@github.com:o/r.git", Getenv: env(map[string]string{"GITHUB_TOKEN": "t"})},
			Info{Name: "github", Repo: "o/r", DefaultRef: "HEAD", WebURL: "https://github.com", APIURL: "https://api.github.com", Token: "t"}},
		{"public gitlab remote uses env token", DetectInput{RemoteURL: "https://gitlab.com/g/p.git", Getenv: env(map[string]string{"GITLAB_TOKEN": "p"})},
			Info{Name: "gitlab", Repo: "g/p", DefaultRef: "HEAD", WebURL: "https://gitlab.com", APIURL: "https://gitlab.com/api/v4", Token: "p"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Detect(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				// tokens deliberately not printed
				got.Token, tc.want.Token = redact(got.Token), redact(tc.want.Token)
				t.Fatalf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func redact(s string) string {
	if s == "" {
		return ""
	}
	return "<set>"
}

func TestDetectErrors(t *testing.T) {
	_, err := Detect(DetectInput{RemoteURL: "git@example.org:a/b.git", Getenv: env(nil)})
	if err == nil || !strings.Contains(err.Error(), "--provider") {
		t.Fatalf("err=%v", err)
	}
	_, err = Detect(DetectInput{Provider: "github", Getenv: env(nil)})
	if err == nil || !strings.Contains(err.Error(), "--repo") {
		t.Fatalf("err=%v", err)
	}
	if _, err = Detect(DetectInput{Provider: "nope", Getenv: env(nil)}); err == nil {
		t.Fatal("want unknown provider error")
	}
}

func TestTokenHelp(t *testing.T) {
	for _, n := range []string{"github", "gitlab", "bitbucket"} {
		if h := TokenHelp(n); !strings.Contains(h, "--token") || strings.Contains(h, "\n") {
			t.Errorf("%s", n)
		}
	}
	// Self-hostable providers explain why a remote-derived host gets no env token.
	for _, n := range []string{"github", "gitlab"} {
		if !strings.Contains(TokenHelp(n), "--base-url") {
			t.Errorf("%s: no --base-url hint", n)
		}
	}
}
