package reporting

import "errors"

var (
	ErrReportNotFound  = errors.New("reporting: report definition not found for this vertical")
	ErrInvalidReportID = errors.New("reporting: invalid report ID")
)

// Logger is the structured logging interface used by the reporting service.
// Matches the contract defined in pkg/registration to keep a consistent pattern.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}
