package http1

// Stage names a step of reading a message.
type Stage string

// Stages of reading a message, in order. request.Read starts at
// StageRequestLine and response.Read at StageStatusLine.
const (
	StageRequestLine Stage = "request-line"
	StageStatusLine  Stage = "status-line"
	StageHeaders     Stage = "headers"
	StageFields      Stage = "fields"
	StageBody        Stage = "body"
)

// StageError reports the stage at which reading a message failed, so callers
// can choose a response for errors without a sentinel. Error returns the
// underlying message unchanged; Unwrap keeps errors.Is and errors.As working
// for sentinels and *FieldError.
type StageError struct {
	Stage Stage
	Err   error
}

func (stageErr *StageError) Error() string { return stageErr.Err.Error() }

func (stageErr *StageError) Unwrap() error { return stageErr.Err }
