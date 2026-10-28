package http1

// HeaderField stores one header field's name and value without validation or
// normalization. Headers slices can retain repeated fields as separate entries
// in their original order; this type does not combine values.
type HeaderField struct {
	Name  string
	Value string
}
