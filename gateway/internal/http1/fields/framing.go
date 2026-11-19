package fields

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/body"
	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/header"
)

// framingFields lists the fields that describe how a message body is framed.
var framingFields = []string{"content-length", "transfer-encoding", "trailer"}

// WithFraming returns a copy of headers whose framing fields describe the body
// a writer sends: Content-Length for body.ContentLength, a negative length
// treated as zero; Transfer-Encoding: chunked for body.Chunked, with a Trailer
// field listing trailerNames when there are any; and none for other modes. The
// received Content-Length, Transfer-Encoding, and Trailer fields are dropped,
// since they described the body as it was received. The input is not changed.
func WithFraming(headers header.Section, mode body.Mode, length int64, trailerNames []string) header.Section {
	section := maps.Clone(headers)
	if section == nil {
		section = make(header.Section)
	}

	for name := range slices.Values(framingFields) {
		delete(section, name)
	}

	switch mode {
	case body.ContentLength:
		section["content-length"] = []string{strconv.FormatInt(max(length, 0), 10)}
	case body.Chunked:
		section["transfer-encoding"] = []string{"chunked"}
		if len(trailerNames) > 0 {
			section["trailer"] = []string{strings.Join(trailerNames, ", ")}
		}
	}

	return section
}
