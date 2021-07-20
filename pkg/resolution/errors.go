package resolution

import (
	"errors"
)

// Error embeds both a short machine-readable string reason for resolution
// alongside the original error generated during the resolution flow.
type Error struct {
	Reason   string
	Original error
}

var _ error = &Error{}

// Error returns the original error's message. This is intended to meet the error.Error interface.
func (e *Error) Error() string {
	return e.Original.Error()
}

// Unwrap returns the original error without the Reason annotation. This is
// intended to support usage of errors.Is and errors.As with resolution.Errors.
func (e *Error) Unwrap() error {
	return e.Original
}

// NewError returns a resolution.Error with the given reason and underlying
// original error.
func NewError(reason string, err error) *Error {
	return &Error{
		Reason:   reason,
		Original: err,
	}
}

var (
	// ErrorTaskRunAlreadyResolved is a sentinel value that consumers of the resolution package can use to determine if a taskrun
	// was already resolved and, if so, customize their fallback behaviour.
	ErrorTaskRunAlreadyResolved = NewError("TaskRunAlreadyResolved", errors.New("TaskRun is already resolved"))

	// ErrorPipelineRunAlreadyResolved is a sentinel value that consumers of the resolution package can use to determine if a pipelinerun
	// was already resolved and, if so, customize their fallback behaviour.
	ErrorPipelineRunAlreadyResolved = NewError("PipelineRunAlreadyResolved", errors.New("PipelineRun is already resolved"))
)
