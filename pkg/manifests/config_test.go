package manifests

import (
	"os"
	"testing"
)

func TestConfigParsing(t *testing.T) {
	f, err := os.Open("../../examples/config/config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewConfig(f)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmptyConfigIsValid(t *testing.T) {
	_, err := NewConfigFromString("")
	if err != nil {
		t.Fatal(err)
	}
}
