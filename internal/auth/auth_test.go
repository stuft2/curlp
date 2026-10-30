package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stuft2/knok/internal/config"
)

func testCache(t *testing.T) Cache {
	t.Helper()
	return Cache{Dir: filepath.Join(t.TempDir(), "knok", "tokens")}
}
func TestOAuthAcquisitionAndCache(t *testing.T) {
	for _, method := range []string{"client_secret_basic", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			t.Setenv("CLIENT_SECRET", "sec:ret +/")
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" {
					t.Error("wrong method")
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("scope") != "read write" {
					t.Error("wrong token form")
				}
				if method == "client_secret_basic" {
					want := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape("app +:")+":"+url.QueryEscape("sec:ret +/")))
					if r.Header.Get("Authorization") != want || r.Form.Get("client_secret") != "" {
						t.Error("incorrect Basic authentication")
					}
				} else if r.Form.Get("client_id") != "app +:" || r.Form.Get("client_secret") != "sec:ret +/" || r.Header.Get("Authorization") != "" {
					t.Error("incorrect form authentication")
				}
				fmt.Fprint(w, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`)
			}))
			defer server.Close()
			cache := testCache(t)
			a := config.Auth{Type: "oauth2-client-credentials", TokenURL: server.URL, ClientID: config.Secret{Literal: "app +:"}, ClientSecret: config.Secret{Env: "CLIENT_SECRET"}, Scopes: []string{"read", "write"}, TokenAuthMethod: method}
			p := Provider{Client: server.Client(), Cache: cache}
			for range 2 {
				req, _ := http.NewRequest("GET", "https://api.example/users", nil)
				if err := p.Apply(context.Background(), req, a); err != nil {
					t.Fatal(err)
				}
				if req.Header.Get("Authorization") != "Bearer access-token" {
					t.Fatal("missing resource credential")
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("token calls = %d", calls.Load())
			}
			entries, err := os.ReadDir(cache.Dir)
			if err != nil || len(entries) != 1 {
				t.Fatal("cache entry missing", err)
			}
			b, err := os.ReadFile(filepath.Join(cache.Dir, entries[0].Name()))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "sec:ret") {
				t.Fatal("cache contains client secret")
			}
			if err := cache.Clear(); err != nil {
				t.Fatal(err)
			}
			if _, ok := cache.load(cacheKey("api.example", a), time.Now()); ok {
				t.Fatal("cache clear did not remove token")
			}
		})
	}
}
func TestOAuthFailureAndExpiryResponses(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		wantErr    bool
		cache      bool
	}{
		{"no expiry", `{"access_token":"abc","token_type":"Bearer"}`, 200, false, false},
		{"invalid expiry", `{"access_token":"abc","token_type":"Bearer","expires_in":"3600"}`, 200, false, false},
		{"short expiry", `{"access_token":"abc","token_type":"Bearer","expires_in":10}`, 200, false, false},
		{"long expiry", `{"access_token":"abc","token_type":"Bearer","expires_in":3600}`, 200, false, true},
		{"wrong type", `{"access_token":"sensitive","token_type":"MAC"}`, 200, true, false},
		{"empty token", `{"access_token":"","token_type":"Bearer"}`, 200, true, false},
		{"control token", `{"access_token":"sensitive\n","token_type":"Bearer"}`, 200, true, false},
		{"malformed", `sensitive`, 200, true, false},
		{"oversized", strings.Repeat("s", 1<<20+1), 200, true, false},
		{"error body", `{"error":"sensitive"}`, 401, true, false},
	}
	t.Setenv("SECRET", "sensitive")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			cache := testCache(t)
			a := config.Auth{Type: "oauth2-client-credentials", TokenURL: server.URL, ClientID: config.Secret{Literal: "app"}, ClientSecret: config.Secret{Env: "SECRET"}}
			_, err := (Provider{Client: server.Client(), Cache: cache}).token(context.Background(), "example.com", a)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "sensitive") {
				t.Fatal("secret exposed")
			}
			_, ok := cache.load(cacheKey("example.com", a), time.Now())
			if ok != tc.cache {
				t.Fatalf("cache hit = %v", ok)
			}
		})
	}
}
func TestTokenEndpointRedirectDoesNotForwardCredentials(t *testing.T) {
	var hits atomic.Int32
	dest := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer dest.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dest.URL, 302) }))
	defer source.Close()
	t.Setenv("SECRET", "secret")
	a := config.Auth{TokenURL: source.URL, ClientID: config.Secret{Literal: "app"}, ClientSecret: config.Secret{Env: "SECRET"}}
	_, err := (Provider{Client: source.Client(), Cache: testCache(t)}).token(context.Background(), "example.com", a)
	if err == nil || hits.Load() != 0 {
		t.Fatal("token endpoint redirect followed")
	}
}
func TestBasicBearerAndHTTPS(t *testing.T) {
	t.Setenv("TOKEN", "abc\n")
	t.Setenv("PASSWORD", "secret")
	for _, a := range []config.Auth{{Type: "bearer", Token: config.Secret{Env: "TOKEN"}}, {Type: "basic", Username: "user", Password: config.Secret{Env: "PASSWORD"}}} {
		p := Provider{}
		bad, _ := http.NewRequest("GET", "http://example.com", nil)
		if p.Apply(context.Background(), bad, a) == nil {
			t.Fatal("allowed HTTP credentials")
		}
		req, _ := http.NewRequest("GET", "https://example.com", nil)
		if err := p.Apply(context.Background(), req, a); err != nil {
			t.Fatal(err)
		}
		if a.Type == "bearer" && req.Header.Get("Authorization") != "Bearer abc" {
			t.Fatal("bad bearer")
		}
		if a.Type == "basic" {
			user, pw, ok := req.BasicAuth()
			if !ok || user != "user" || pw != "secret" {
				t.Fatal("bad basic auth")
			}
		}
	}
}
func TestCacheBoundariesAndConcurrency(t *testing.T) {
	c := testCache(t)
	key := strings.Repeat("a", 64)
	c.save(key, "old", time.Now().Add(-time.Hour))
	if _, ok := c.load(key, time.Now()); ok {
		t.Fatal("expired token accepted")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { c.save(key, "new", time.Now().Add(time.Hour)); c.load(key, time.Now()) })
	}
	wg.Wait()
	if token, ok := c.load(key, time.Now()); !ok || token != "new" {
		t.Fatal("concurrent cache corrupted")
	}
	target := filepath.Join(c.Dir, key+".json")
	b, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var e entry
	if json.Unmarshal(b, &e) != nil {
		t.Fatal("invalid JSON")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(target)
		if st.Mode().Perm() != 0600 {
			t.Fatal("incorrect file mode")
		}
		st, _ = os.Stat(c.Dir)
		if st.Mode().Perm() != 0700 {
			t.Fatal("incorrect directory mode")
		}
		if err := os.Chmod(target, 0644); err != nil {
			t.Fatal(err)
		}
		if _, ok := c.load(key, time.Now()); ok {
			t.Fatal("unsafe permissions accepted")
		}
		if err := os.Chmod(target, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(target, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.load(key, time.Now()); ok {
		t.Fatal("malformed cache accepted")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "untouched")
	if err := os.WriteFile(elsewhere, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, target); err == nil {
		c.save(key, "overwrite", time.Now().Add(time.Hour))
		if _, ok := c.load(key, time.Now()); ok {
			t.Fatal("symlink accepted")
		}
		b, _ := os.ReadFile(elsewhere)
		if string(b) != "sentinel" {
			t.Fatal("symlink target overwritten")
		}
	}
}
func TestSecretCommand(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KNOK_TEST_HELPER", "1")
	source := func(mode string) config.Secret {
		return config.Secret{Command: []string{exe, "-test.run=^TestCredentialHelper$", "--", mode}}
	}
	value, err := Resolve(context.Background(), source("ok"))
	if err != nil || value != "secret" {
		t.Fatal("command output", err)
	}
	for _, mode := range []string{"fail", "large"} {
		_, err := Resolve(context.Background(), source(mode))
		if err == nil || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("unsafe command error", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resolve(ctx, source("ok")); err == nil {
		t.Fatal("cancellation ignored")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Resolve(ctx, source("wait")); err == nil {
		t.Fatal("command timeout ignored")
	}
	t.Setenv("EMPTY", "")
	if _, err := Resolve(context.Background(), config.Secret{Env: "EMPTY"}); err == nil {
		t.Fatal("empty secret accepted")
	}
}
func TestCredentialHelper(t *testing.T) {
	if os.Getenv("KNOK_TEST_HELPER") != "1" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "ok":
		fmt.Print("secret\r\n")
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, "sensitive")
		os.Exit(1)
	case "env":
		fmt.Print(os.Getenv("RESOLVED_ID"))
		os.Exit(0)
	case "wait":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "large":
		fmt.Print(strings.Repeat("sensitive", 10000))
		os.Exit(0)
	}
}

func TestUnsafeCacheDirectoryAndWriteFailure(t *testing.T) {
	t.Setenv("SECRET", "private-secret")
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, `{"access_token":"usable-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	c := testCache(t)
	if err := os.MkdirAll(filepath.Dir(c.Dir), 0700); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, c.Dir); err != nil {
		t.Skip("symlinks unavailable")
	}
	warnings := 0
	c.Warn = func(string) { warnings++ }
	a := config.Auth{TokenURL: server.URL, ClientID: config.Secret{Literal: "app"}, ClientSecret: config.Secret{Env: "SECRET"}}
	token, err := (Provider{Client: server.Client(), Cache: c}).token(context.Background(), "example.com", a)
	if err != nil || token != "usable-token" || warnings == 0 || hits.Load() != 1 {
		t.Fatal("cache failure disrupted request", err, warnings)
	}
	entries, err := os.ReadDir(elsewhere)
	if err != nil || len(entries) != 0 {
		t.Fatal("symlink directory was used")
	}
	if err := c.Clear(); err == nil {
		t.Fatal("clear accepted unsafe directory")
	}
}

func TestOAuthClientIDReferences(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"client_secret_basic", "client_secret_post"} {
		for _, source := range []string{"env", "command"} {
			t.Run(method+"/"+source, func(t *testing.T) {
				t.Setenv("KNOK_TEST_HELPER", "1")
				t.Setenv("RESOLVED_ID", "source app+:")
				t.Setenv("CLIENT_SECRET", "client-secret")
				ref := config.Secret{Env: "RESOLVED_ID"}
				if source == "command" {
					ref = config.Secret{Command: []string{exe, "-test.run=^TestCredentialHelper$", "--", "env"}}
				}
				var calls atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if method == "client_secret_post" {
						if r.Form.Get("client_id") != "source app+:" || r.Form.Get("client_secret") != "client-secret" {
							t.Error("incorrect form credentials")
						}
					} else {
						id, secret, ok := r.BasicAuth()
						if !ok || id != url.QueryEscape("source app+:") || secret != url.QueryEscape("client-secret") {
							t.Error("incorrect Basic credentials")
						}
					}
					fmt.Fprint(w, `{"access_token":"cached-token","token_type":"Bearer","expires_in":3600}`)
				}))
				defer server.Close()
				cache := testCache(t)
				a := config.Auth{Type: "oauth2-client-credentials", TokenURL: server.URL, ClientID: ref, ClientSecret: config.Secret{Env: "CLIENT_SECRET"}, TokenAuthMethod: method}
				provider := Provider{Client: server.Client(), Cache: cache}
				for i := 0; i < 2; i++ {
					req, _ := http.NewRequest("GET", "https://api.example.com", nil)
					if err := provider.Apply(context.Background(), req, a); err != nil {
						t.Fatal(err)
					}
					if req.Header.Get("Authorization") != "Bearer cached-token" {
						t.Fatal("missing access token")
					}
					// Cache hits must not resolve either source again, even if unavailable.
					t.Setenv("RESOLVED_ID", "")
					t.Setenv("CLIENT_SECRET", "")
				}
				if calls.Load() != 1 {
					t.Fatal("token not reused")
				}
				for _, text := range []string{"source app+:", "client-secret"} {
					b, err := os.ReadFile(filepath.Join(cache.Dir, cacheKey("api.example.com", a)+".json"))
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(b), text) {
						t.Fatal("resolved credentials persisted")
					}
				}
			})
		}
	}
}

func TestOAuthLiteralClientSecret(t *testing.T) {
	for _, method := range []string{"client_secret_basic", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if method == "client_secret_post" {
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.Form.Get("client_id") != "literal-id" || r.Form.Get("client_secret") != "literal-secret" {
						t.Error("incorrect literal form credentials")
					}
				} else {
					id, secret, ok := r.BasicAuth()
					if !ok || id != "literal-id" || secret != "literal-secret" {
						t.Error("incorrect literal Basic credentials")
					}
				}
				fmt.Fprint(w, `{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`)
			}))
			defer server.Close()
			a := config.Auth{TokenURL: server.URL, ClientID: config.Secret{Literal: "literal-id"}, ClientSecret: config.Secret{Literal: "literal-secret"}, TokenAuthMethod: method}
			cache := testCache(t)
			if _, err := (Provider{Client: server.Client(), Cache: cache}).token(context.Background(), "api.example.com", a); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(cache.Dir, cacheKey("api.example.com", a)+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "literal-secret") || strings.Contains(string(b), "literal-id") {
				t.Fatal("literal credentials persisted in token cache")
			}
		})
	}
	for _, a := range []config.Auth{
		{Type: "bearer", Token: config.Secret{Literal: "literal-token"}},
		{Type: "basic", Username: "user", Password: config.Secret{Literal: "literal-password"}},
	} {
		req, _ := http.NewRequest("GET", "https://api.example.com", nil)
		if err := (Provider{}).Apply(context.Background(), req, a); err != nil {
			t.Fatal(err)
		}
		if a.Type == "bearer" && req.Header.Get("Authorization") != "Bearer literal-token" {
			t.Fatal("incorrect literal bearer")
		}
		if a.Type == "basic" {
			u, p, ok := req.BasicAuth()
			if !ok || u != "user" || p != "literal-password" {
				t.Fatal("incorrect literal basic")
			}
		}
	}
}
