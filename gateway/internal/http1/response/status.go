package response

import (
	"fmt"
	"strconv"

	"github.com/ammartahamohamedy/http-gateway/gateway/internal/http1/grammar"
)

// reasonPhrases maps every status code with a defined meaning in the IANA HTTP
// Status Code Registry to its registered reason phrase. It is the single record
// of known codes. 306 and 418 are reserved as unused and are absent. 104 is
// absent because its temporary registration expired on 2026-11-13. 510 is
// obsoleted but keeps its meaning (RFC 2774), so it stays.
var reasonPhrases = map[int]string{
	100: "Continue",
	101: "Switching Protocols",
	102: "Processing",
	103: "Early Hints",

	200: "OK",
	201: "Created",
	202: "Accepted",
	203: "Non-Authoritative Information",
	204: "No Content",
	205: "Reset Content",
	206: "Partial Content",
	207: "Multi-Status",
	208: "Already Reported",
	226: "IM Used",

	300: "Multiple Choices",
	301: "Moved Permanently",
	302: "Found",
	303: "See Other",
	304: "Not Modified",
	305: "Use Proxy",
	307: "Temporary Redirect",
	308: "Permanent Redirect",

	400: "Bad Request",
	401: "Unauthorized",
	402: "Payment Required",
	403: "Forbidden",
	404: "Not Found",
	405: "Method Not Allowed",
	406: "Not Acceptable",
	407: "Proxy Authentication Required",
	408: "Request Timeout",
	409: "Conflict",
	410: "Gone",
	411: "Length Required",
	412: "Precondition Failed",
	413: "Content Too Large",
	414: "URI Too Long",
	415: "Unsupported Media Type",
	416: "Range Not Satisfiable",
	417: "Expectation Failed",
	421: "Misdirected Request",
	422: "Unprocessable Content",
	423: "Locked",
	424: "Failed Dependency",
	425: "Too Early",
	426: "Upgrade Required",
	428: "Precondition Required",
	429: "Too Many Requests",
	431: "Request Header Fields Too Large",
	451: "Unavailable For Legal Reasons",

	500: "Internal Server Error",
	501: "Not Implemented",
	502: "Bad Gateway",
	503: "Service Unavailable",
	504: "Gateway Timeout",
	505: "HTTP Version Not Supported",
	506: "Variant Also Negotiates",
	507: "Insufficient Storage",
	508: "Loop Detected",
	510: "Not Extended",
	511: "Network Authentication Required",
}

// parseStatusCode parses a status-code: exactly three ASCII digits (RFC 9112,
// Section 4) within 100-599 (RFC 9110, Section 15). Codes absent from
// reasonPhrases are valid, since recipients treat an unrecognized code as x00
// of its class. Errors return zero.
func parseStatusCode(rawCode string) (int, error) {
	if len(rawCode) != 3 || !grammar.IsDigits(rawCode) {
		return 0, fmt.Errorf("http1: status code %q is not three digits", rawCode)
	}

	statusCode, err := strconv.Atoi(rawCode)
	if err != nil || statusCode < 100 || statusCode > 599 {
		return 0, fmt.Errorf("http1: status code %q is outside 100-599", rawCode)
	}

	return statusCode, nil
}
