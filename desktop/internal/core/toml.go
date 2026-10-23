package core

// TOML import support. github.com/pelletier/go-toml/v2 (same library the
// wisteria cores use) parses the imported client config.

import (
	"net/url"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

func tomlUnmarshal(data []byte, v any) error {
	return toml.Unmarshal(data, v)
}

func urlPathUnescape(s string) (string, error) {
	return url.PathUnescape(s)
}

// trimFloat renders a float without trailing ".0" noise.
func trimFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return strings.TrimSuffix(s, ".0")
}
