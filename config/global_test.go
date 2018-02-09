package config

import (
	"testing"
)

func TestNew(t *testing.T) {
	var compare = &Configuration{}
	var path = "/path/to/a/file"
	result := Load(path)
	// we just ensure that we have the same type in the result as what we expect
	if compare == result {
	}
}
