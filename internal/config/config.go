// Package config reads and validates knok's host authentication configuration.
package config

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const MaxSize = 1 << 20

var yamlLine = regexp.MustCompile(`(?:^|\n|yaml: )\s*line ([0-9]+)`)

func yamlError(message string, err error) error {
	if line := yamlLine.FindStringSubmatch(err.Error()); len(line) == 2 {
		return fmt.Errorf("%s at line %s", message, line[1])
	}
	return fmt.Errorf("%s", message)
}

type Secret struct {
	Literal string   `json:"literal,omitempty"`
	Env     string   `yaml:"env" json:"env,omitempty"`
	Command []string `yaml:"command" json:"command,omitempty"`
}
type Auth struct {
	Type            string   `yaml:"type" json:"type"`
	Token           Secret   `yaml:"token" json:"token,omitempty"`
	Username        string   `yaml:"username" json:"username,omitempty"`
	Password        Secret   `yaml:"password" json:"password,omitempty"`
	TokenURL        string   `yaml:"token_url" json:"token_url,omitempty"`
	ClientID        Secret   `yaml:"client_id" json:"client_id,omitempty"`
	ClientSecret    Secret   `yaml:"client_secret" json:"client_secret,omitempty"`
	Scopes          []string `yaml:"scopes" json:"scopes,omitempty"`
	TokenAuthMethod string   `yaml:"token_auth_method" json:"token_auth_method,omitempty"`
}
type Host struct {
	Auth Auth `yaml:"auth"`
}
type Config struct {
	Version int             `yaml:"version"`
	Hosts   map[string]Host `yaml:"hosts"`
}

func Hostname(s string) string { return strings.ToLower(strings.TrimSuffix(s, ".")) }

func DefaultPath() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		if !filepath.IsAbs(base) {
			return "", fmt.Errorf("XDG_CONFIG_HOME must be absolute")
		}
		return filepath.Join(base, "knok", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "knok", "config.yaml"), nil
}

func Load(path string, explicit bool) (Config, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) && !explicit {
		return Config{Version: 1}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("open configuration: %w", err)
	}
	defer f.Close()
	return Parse(f)
}

func Parse(r io.Reader) (Config, error) {
	var c Config
	b, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return c, fmt.Errorf("read configuration: %w", err)
	}
	if len(b) > MaxSize {
		return c, fmt.Errorf("configuration exceeds 1 MiB")
	}
	var node yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(b))
	// Do not forward parser errors: they can quote input containing credentials.
	if err := dec.Decode(&node); err != nil {
		return c, yamlError("invalid YAML configuration", err)
	}
	if err := checkNode(&node, 0); err != nil {
		return c, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("configuration must contain one YAML document")
	}
	strict := yaml.NewDecoder(bytes.NewReader(b))
	strict.KnownFields(true)
	if err := strict.Decode(&c); err != nil {
		return c, yamlError("configuration contains an unknown field, duplicate key, or invalid value", err)
	}
	if c.Version != 1 {
		return c, fmt.Errorf("configuration version must be 1")
	}
	hostLines := make(map[string]int)
	if len(node.Content) > 0 {
		root := node.Content[0]
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value == "hosts" {
				hosts := root.Content[i+1]
				for j := 0; j+1 < len(hosts.Content); j += 2 {
					hostLines[hosts.Content[j].Value] = hosts.Content[j].Line
				}
			}
		}
	}
	normalized := make(map[string]Host, len(c.Hosts))
	for name, h := range c.Hosts {
		key := Hostname(name)
		if !validHost(key) {
			return c, fmt.Errorf("invalid host entry at line %d", hostLines[name])
		}
		if _, ok := normalized[key]; ok {
			return c, fmt.Errorf("duplicate normalized hostname at line %d", hostLines[name])
		}
		if err := h.Auth.validate(); err != nil {
			return c, fmt.Errorf("host authentication at line %d: %w", hostLines[name], err)
		}
		normalized[key] = h
	}
	c.Hosts = normalized
	return c, nil
}

func checkNode(n *yaml.Node, depth int) error {
	if depth > 32 {
		return fmt.Errorf("configuration nesting exceeds 32 levels")
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return fmt.Errorf("YAML aliases and anchors are unsupported at line %d", n.Line)
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!int" && depth != 2 {
		return fmt.Errorf("expected a string at line %d; quote numeric identifiers", n.Line)
	}
	if n.Tag != "" && n.Tag != "!!map" && n.Tag != "!!seq" && n.Tag != "!!str" && n.Tag != "!!int" && n.Tag != "!!null" {
		return fmt.Errorf("unsupported YAML type at line %d", n.Line)
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" || seen[k.Value] {
				return fmt.Errorf("invalid or duplicate mapping key at line %d", k.Line)
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := checkNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func validHost(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func (s Secret) present() bool { return s.Literal != "" || s.Env != "" || s.Command != nil }
func (s Secret) valid() bool {
	sources := 0
	if s.Literal != "" {
		sources++
	}
	if s.Env != "" {
		sources++
	}
	if s.Command != nil {
		sources++
		if len(s.Command) == 0 || s.Command[0] == "" {
			return false
		}
	}
	return sources == 1
}
func (a *Auth) validate() error {
	switch a.Type {
	case "bearer":
		if !a.Token.valid() || a.Username != "" || a.Password.present() || a.hasOAuth() {
			return fmt.Errorf("bearer requires only a token")
		}
	case "basic":
		if a.Username == "" || strings.Contains(a.Username, ":") || !a.Password.valid() || a.Token.present() || a.hasOAuth() {
			return fmt.Errorf("basic requires a username and password")
		}
	case "oauth2-client-credentials":
		if !a.ClientID.valid() || !a.ClientSecret.valid() || a.Token.present() || a.Username != "" || a.Password.present() {
			return fmt.Errorf("OAuth requires client_id and client_secret")
		}
		u, err := url.Parse(a.TokenURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("token_url must be an absolute HTTPS URL without userinfo or fragment")
		}
		if a.TokenAuthMethod == "" {
			a.TokenAuthMethod = "client_secret_basic"
		}
		if a.TokenAuthMethod != "client_secret_basic" && a.TokenAuthMethod != "client_secret_post" {
			return fmt.Errorf("invalid token_auth_method")
		}
		for _, s := range a.Scopes {
			if s == "" {
				return fmt.Errorf("empty OAuth scope")
			}
			for _, c := range s {
				if c < 0x21 || c > 0x7e || c == '"' || c == '\\' {
					return fmt.Errorf("invalid OAuth scope")
				}
			}
		}
	default:
		return fmt.Errorf("unsupported authentication type")
	}
	return nil
}
func (a Auth) hasOAuth() bool {
	return a.TokenURL != "" || a.ClientID.present() || a.ClientSecret.present() || a.Scopes != nil || a.TokenAuthMethod != ""
}
