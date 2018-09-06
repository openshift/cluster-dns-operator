package manifests

import (
	"bytes"
	"io"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type Config struct {
	DNSClusterIP     string `json:"dnsClusterIP"`
	DNSClusterDomain string `json:"dnsClusterDomain"`
}

func NewConfig(content io.Reader) (*Config, error) {
	c := Config{}

	err := yaml.NewYAMLOrJSONDecoder(content, 100).Decode(&c)
	if err != nil {
		return nil, err
	}

	res := &c
	res.applyDefaults()

	return res, nil
}

func NewDefaultConfig() *Config {
	c := &Config{}
	c.applyDefaults()
	return c
}

func NewConfigFromString(content string) (*Config, error) {
	if content == "" {
		return NewDefaultConfig(), nil
	}

	return NewConfig(bytes.NewBuffer([]byte(content)))
}

func (c *Config) applyDefaults() {
	// We do not want to duplicate default values already set in YAML files
}
