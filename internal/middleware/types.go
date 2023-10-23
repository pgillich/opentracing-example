package middleware

import "strings"

const (
	// MetrAttrErr is the metric attribute for error
	MetrAttrErr         = "error"
	MetrAttrMethod      = "method"
	MetrAttrUrl         = "url"
	MetrAttrStatus      = "status"
	MetrAttrPathPattern = "path_pattern"
	MetrAttrPath        = "path"
	MetrAttrHost        = "host"
)

// ErrFormatter is a func type to format metric error attribute
type ErrFormatter func(error) string

// NoErr always returns "". Can be used to skip any error stats in the metrics
func NoErr(error) string {
	return ""
}

// FullErr returns the full error text.
// Be careful about the cardinality, if the error text has dynamic part(s) (see: Prometheus label)
func FullErr(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

// FirstErr returns the first part of error text before ':'
func FirstErr(err error) string {
	if err == nil {
		return ""
	}

	return strings.SplitN(err.Error(), ":", 2)[0]
}
