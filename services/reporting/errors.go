package reporting

import "errors"

var (
	ErrReportNotFound  = errors.New("reporting: report definition not found for this vertical")
	ErrInvalidReportID = errors.New("reporting: invalid report ID")
)
