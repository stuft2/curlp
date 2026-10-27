package config

import (
	"strings"
	"testing"
)

func TestParseAndNormalize(t *testing.T) {
	c, err := Parse(strings.NewReader(`version: 1
hosts:
  API.Example.com.:
    auth:
      type: bearer
      token: {env: API_TOKEN}
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Hosts["api.example.com"].Auth.Token.Env != "API_TOKEN" {
		t.Fatal("host was not normalized")
	}
}
func TestRejectInvalidConfig(t *testing.T) {
	cases := map[string]string{
		"unknown":              "version: 1\nsecret: sensitive\n",
		"duplicate":            "version: 1\nversion: 1\n",
		"normalized duplicate": "version: 1\nhosts: {API.example: {auth: {type: bearer, token: {env: X}}}, api.example.: {auth: {type: bearer, token: {env: X}}}}",
		"wildcard":             "version: 1\nhosts: {'*.example.com': {auth: {type: bearer, token: {env: X}}}}",
		"port":                 "version: 1\nhosts: {'api.example:443': {auth: {type: bearer, token: {env: X}}}}",
		"alias":                "version: 1\nhosts: &x {}\nother: *x",
		"custom tag":           "version: 1\nhosts: !secret {}",
		"multi document":       "version: 1\nhosts: {}\n---\nversion: 1",
		"version":              "version: 2",
		"malformed":            "version: 1\nhosts: [sensitive",
		"numeric secret":       "version: 1\nhosts: {example.com: {auth: {type: bearer, token: 123}}}",
		"mixed source":         "version: 1\nhosts: {example.com: {auth: {type: bearer, token: {env: X, command: [echo, sensitive]}}}}",
		"mixed auth":           "version: 1\nhosts: {example.com: {auth: {type: bearer, token: {env: X}, username: sensitive}}}",
		"empty command":        "version: 1\nhosts: {example.com: {auth: {type: bearer, token: {command: []}}}}",
		"insecure oauth":       "version: 1\nhosts: {example.com: {auth: {type: oauth2-client-credentials, token_url: 'http://auth.example/token', client_id: x, client_secret: {env: X}}}}",
		"deep":                 "version: 1\nhosts: " + strings.Repeat("[", 40) + "sensitive" + strings.Repeat("]", 40),
		"large":                strings.Repeat(" ", MaxSize+1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(input))
			if err == nil {
				t.Fatal("accepted invalid config")
			}
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatal("error exposed contents")
			}
		})
	}
}
func TestOAuthDefaults(t *testing.T) {
	c, err := Parse(strings.NewReader(`version: 1
hosts:
  example.com:
    auth:
      type: oauth2-client-credentials
      token_url: https://auth.example/token
      client_id: app
      client_secret: {env: SECRET}
      scopes: [read, write]
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Hosts["example.com"].Auth.TokenAuthMethod != "client_secret_basic" {
		t.Fatal("missing default")
	}
}

func TestValidationErrorsIdentifyLocation(t *testing.T) {
	for _, text := range []string{
		"version: 1\nunknown: sensitive\n",
		"version: 1\nhosts:\n  api.example.com:\n    auth:\n      type: bearer\n",
	} {
		_, err := Parse(strings.NewReader(text))
		if err == nil || !strings.Contains(err.Error(), "line ") || strings.Contains(err.Error(), "sensitive") {
			t.Fatal("missing safe error location", err)
		}
	}
}

func TestClientIDSources(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"literal", `my-client`, true},
		{"environment", `{env: CLIENT_ID}`, true},
		{"command", `{command: [pass-cli, item, view, --field, API Key]}`, true},
		{"empty", `''`, false},
		{"empty reference", `{}`, false},
		{"empty command", `{command: []}`, false},
		{"mixed reference", `{env: CLIENT_ID, command: [echo, x]}`, false},
		{"unknown field", `{env: CLIENT_ID, unknown: sensitive}`, false},
		{"nested unknown field", `{command: {unknown: sensitive}}`, false},
		{"sequence", `[sensitive]`, false},
		{"numeric", `123`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "version: 1\nhosts:\n  api.example.com:\n    auth:\n      type: oauth2-client-credentials\n      token_url: https://api.example.com/token\n      client_id: " + tc.value + "\n      client_secret: {env: SECRET}\n"
			c, err := Parse(strings.NewReader(text))
			if (err == nil) != tc.valid {
				t.Fatalf("valid = %v, error = %v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "sensitive") {
				t.Fatal("error exposed configuration")
			}
			if tc.name == "literal" && c.Hosts["api.example.com"].Auth.ClientID.Literal != "my-client" {
				t.Fatal("literal ID compatibility lost")
			}
		})
	}
}

func TestLiteralSecrets(t *testing.T) {
	for _, settings := range []string{
		"type: bearer\n      token: literal-token",
		"type: basic\n      username: user\n      password: 'literal-password'",
		"type: oauth2-client-credentials\n      token_url: https://example.com/token\n      client_id: app\n      client_secret: 'literal-secret'",
	} {
		_, err := Parse(strings.NewReader("version: 1\nhosts:\n  example.com:\n    auth:\n      " + settings + "\n"))
		if err != nil {
			t.Fatal("literal secret rejected", err)
		}
	}
}
