package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}
func run(t *testing.T, r Runner, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := r.Run(context.Background(), args, strings.NewReader("stdin-data"), &out, &errOut)
	return code, out.String(), errOut.String()
}
func configFile(t *testing.T, host, auth string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "version: 1\nhosts:\n  " + host + ":\n    auth:\n" + auth
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestRequestCLI(t *testing.T) {
	isolate(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Test") != "yes" {
			t.Error("incorrect request")
		}
		var b bytes.Buffer
		b.ReadFrom(r.Body)
		w.Header().Set("X-Response", "yes")
		w.WriteHeader(418)
		fmt.Fprint(w, b.String())
	}))
	defer server.Close()
	code, out, errOut := run(t, Runner{}, server.URL, "-H", "X-Test: yes", "--data", "stdin-data", "--include")
	if code != 0 || errOut != "" || !strings.Contains(out, "418") || !strings.Contains(out, "X-Response: yes") || !strings.HasSuffix(out, "stdin-data") {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	target := filepath.Join(t.TempDir(), "response")
	code, out, errOut = run(t, Runner{}, "--header=X-Test: yes", "--data", "@-", "--output", target, server.URL)
	if code != 0 || out != "" || errOut != "" {
		t.Fatal(code, out, errOut)
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "stdin-data" {
		t.Fatal("incorrect output file", err)
	}
	body := filepath.Join(t.TempDir(), "body")
	if err := os.WriteFile(body, []byte("from-file"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = run(t, Runner{}, server.URL, "-H", "X-Test: yes", "-d", "@"+body)
	if code != 0 || out != "from-file" || errOut != "" {
		t.Fatal(code, out, errOut)
	}
}
func TestAuthenticationSelection(t *testing.T) {
	isolate(t)
	t.Setenv("TOKEN", "configured")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, r.Header.Get("Authorization")) }))
	defer server.Close()
	path := configFile(t, "127.0.0.1", "      type: bearer\n      token: {env: TOKEN}\n")
	r := Runner{Client: server.Client()}
	for _, tc := range []struct {
		name  string
		extra []string
		want  string
	}{
		{"configured", nil, "Bearer configured"},
		{"disabled", []string{"--no-auth"}, ""},
		{"override", []string{"-H", "Authorization: Bearer explicit"}, "Bearer explicit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--config", path, server.URL}, tc.extra...)
			code, out, errOut := run(t, r, args...)
			if code != 0 || out != tc.want || errOut != "" {
				t.Fatal(code, out, errOut)
			}
		})
	}
	// Explicit Authorization must not resolve a missing credential source.
	t.Setenv("TOKEN", "")
	code, out, errOut := run(t, r, "--config", path, "-H", "Authorization: explicit", server.URL)
	if code != 0 || out != "explicit" || errOut != "" {
		t.Fatal(code, out, errOut)
	}
	unmatched := configFile(t, "unmatched.example", "      type: bearer\n      token: {env: MISSING}\n")
	code, out, errOut = run(t, r, "--config", unmatched, server.URL)
	if code != 0 || out != "" || errOut != "" {
		t.Fatal("unmatched host got auth", code, out, errOut)
	}
}
func TestRedirectIsolation(t *testing.T) {
	isolate(t)
	t.Setenv("TOKEN", "credential")
	var destHits atomic.Int32
	dest := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destHits.Add(1) }))
	defer dest.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/done", 302)
		case "/cross":
			http.Redirect(w, r, dest.URL, 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 302)
		default:
			fmt.Fprint(w, r.Header.Get("Authorization"))
		}
	}))
	defer source.Close()
	path := configFile(t, "127.0.0.1", "      type: bearer\n      token: {env: TOKEN}\n")
	r := Runner{Client: source.Client()}
	code, out, errOut := run(t, r, "--config", path, "--location", source.URL+"/same")
	if code != 0 || out != "Bearer credential" || errOut != "" {
		t.Fatal(code, out, errOut)
	}
	code, out, errOut = run(t, r, "--config", path, source.URL+"/cross", "-i")
	if code != 0 || !strings.Contains(out, "302") || errOut != "" {
		t.Fatal("default followed redirect", code, out, errOut)
	}
	for _, endpoint := range []string{"/cross", "/loop"} {
		code, _, errOut = run(t, r, "--config", path, "--location", source.URL+endpoint)
		if code != 1 || errOut == "" || strings.Contains(errOut, "credential") {
			t.Fatal(code, errOut)
		}
	}
	if destHits.Load() != 0 {
		t.Fatal("credentials sent across origin")
	}
}
func TestInvalidArgumentsAndConfig(t *testing.T) {
	isolate(t)
	for _, args := range [][]string{
		{}, {"example.com"}, {"https://user:secret@example.com"}, {"https://example.com/#fragment"},
		{"--unknown"}, {"--timeout", "0", "https://example.com"}, {"-X"},
		{"-H", "Authorization: one", "-H", "authorization: two", "http://example.com"},
		{"-H", "X-Test: sensitive\r\nInjected: yes", "http://example.com"},
		{"--config", "/does/not/exist", "http://example.com"},
		{"http://example.com", "http://other.example"},
	} {
		code, out, errOut := run(t, Runner{}, args...)
		if code != 2 || out != "" || errOut == "" || strings.Contains(errOut, "sensitive") || strings.Contains(errOut, "secret") {
			t.Fatalf("args %v: %d %q %q", args, code, out, errOut)
		}
	}
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		code, out, errOut := run(t, Runner{}, args...)
		if code != 0 || out == "" || errOut != "" {
			t.Fatal(code, out, errOut)
		}
	}
}
func TestNoConfiguredCredentialsOverHTTP(t *testing.T) {
	isolate(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	path := configFile(t, "127.0.0.1", "      type: bearer\n      token: {env: MISSING}\n")
	code, _, errOut := run(t, Runner{}, "--config", path, server.URL)
	if code != 1 || !strings.Contains(errOut, "HTTPS") || hits.Load() != 0 {
		t.Fatal(code, errOut)
	}
}
func TestTimeout(t *testing.T) {
	isolate(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	code, _, errOut := run(t, Runner{}, "--timeout", "50ms", server.URL)
	if code != 1 || !strings.Contains(errOut, "timed out") {
		t.Fatal(code, errOut)
	}
}
func TestSameOrigin(t *testing.T) {
	// This is additionally covered over real HTTP redirects above; these cases
	// protect normalization and the scheme boundary without external DNS.
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"https://EXAMPLE.com/", "https://example.com:443/", true},
		{"https://example.com/", "http://example.com/", false},
		{"https://example.com/", "https://sub.example.com/", false},
	} {
		a, _ := http.NewRequest("GET", tc.a, nil)
		b, _ := http.NewRequest("GET", tc.b, nil)
		if sameOrigin(a.URL, b.URL) != tc.want {
			t.Fatal(tc)
		}
	}
}

func TestOAuthThroughCLIAndCacheClear(t *testing.T) {
	isolate(t)
	t.Setenv("CLIENT_SECRET", "private-client-secret")
	var tokenCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenCalls.Add(1)
			if r.Header.Get("Authorization") == "" {
				t.Error("missing client authentication")
			}
			fmt.Fprint(w, `{"access_token":"cached-token","token_type":"Bearer","expires_in":3600}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer cached-token" {
			t.Error("missing API token")
		}
		fmt.Fprint(w, "resource")
	}))
	defer server.Close()
	path := configFile(t, "127.0.0.1", "      type: oauth2-client-credentials\n      token_url: "+server.URL+"/token\n      client_id: app\n      client_secret: {env: CLIENT_SECRET}\n")
	runner := Runner{Client: server.Client()}
	for range 2 {
		code, out, errOut := run(t, runner, "--config", path, server.URL+"/resource")
		if code != 0 || out != "resource" || errOut != "" {
			t.Fatal(code, out, errOut)
		}
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("expected reuse, token calls %d", tokenCalls.Load())
	}
	code, out, errOut := run(t, runner, "cache", "clear")
	if code != 0 || out != "" || errOut != "" {
		t.Fatal(code, out, errOut)
	}
	code, _, errOut = run(t, runner, "--config", path, server.URL+"/resource")
	if code != 0 || errOut != "" || tokenCalls.Load() != 2 {
		t.Fatal("cache not cleared", code, errOut, tokenCalls.Load())
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("output unavailable") }
func TestOutputFailure(t *testing.T) {
	isolate(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "response") }))
	defer server.Close()
	var diagnostics bytes.Buffer
	code := (Runner{}).Run(context.Background(), []string{server.URL}, strings.NewReader(""), brokenWriter{}, &diagnostics)
	if code != 1 || diagnostics.Len() == 0 {
		t.Fatal("output failure ignored", code, diagnostics.String())
	}
}
