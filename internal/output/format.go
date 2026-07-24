package output

import "fmt"

// Format represents the output format for CLI commands.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
)

// ParseFormat validates and returns a Format from a string.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatText, FormatJSON, FormatYAML:
		return Format(s), nil
	default:
		return "", fmt.Errorf("invalid output format %q: must be one of text, json, yaml", s)
	}
}
