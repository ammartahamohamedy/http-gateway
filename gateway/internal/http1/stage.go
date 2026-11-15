package http1

// Stage names a step of reading a message.
type Stage string

// Stages of request.Read, in order.
const (
	StageRequestLine Stage = "request-line"
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
