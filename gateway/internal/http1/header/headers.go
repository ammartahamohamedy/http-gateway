// Package header reads and parses an HTTP/1.1 field section. It checks generic
// field syntax and caller-supplied limits, not field-specific semantics.
package header

import "strings"

// Headers maps lowercase field names to separate field-line values in received
// order for each name. Keys must be lowercase; order across names and original
// name casing are not retained.
type Headers map[string][]string

// Values returns matching field-line values in order, comparing names without
// regard to case. It does not combine or split values. It returns nil if no
// fields match, including for nil or empty Headers. The returned slice is
// independent of h.
func (h Headers) Values(name string) []string {
	return append([]string(nil), h[strings.ToLower(name)]...)
}
