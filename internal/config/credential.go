package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// UnmarshalYAML accepts either a literal string or one credential source.
func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
	*s = Secret{}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		s.Literal = node.Value
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("credential must be a string or source reference")
	}
	if len(node.Content) != 2 {
		return fmt.Errorf("credential reference must contain exactly one source")
	}
	// KnownFields does not propagate through custom unmarshalling. Validate
	// the allowed source keys before decoding into a non-recursive helper.
	for i := 0; i < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "env", "command":
		default:
			return fmt.Errorf("unknown credential reference field at line %d", node.Content[i].Line)
		}
	}
	var source struct {
		Env     string   `yaml:"env"`
		Command []string `yaml:"command"`
	}
	if err := node.Decode(&source); err != nil {
		return err
	}
	s.Env = source.Env
	s.Command = source.Command
	return nil
}
