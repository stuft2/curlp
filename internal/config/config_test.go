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
		"inline secret":        "version: 1\nhosts: {example.com: {auth: {type: bearer, token: sensitive}}}",
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
