package main

import "gopkg.in/yaml.v3"

// yamlUnmarshal wraps yaml.Unmarshal for use in the CLI.
func yamlUnmarshal(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}
