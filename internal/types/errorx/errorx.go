package errorx

import (
	"fmt"
	"github.com/pkg/errors"
)

type Error struct {
	StatusCode int
	Reason     string
	Msg        string
}

type ResponseErr struct {
	Reason string `json:"reason"`
	Msg    string `json:"msg"`
}

func (e *Error) Error() string {
	return e.Reason
}

func (e *Error) WithCause(cause string) error {
	ne := NewError(e.StatusCode, e.Reason, fmt.Sprintf("%s: %s", e.Msg, cause))
	return ne
}

func WithCause(err error, cause string) error {
	switch e := err.(type) {
	case *Error:
		return e.WithCause(cause)
	default:
		ne := errors.Wrap(errors.New(cause), e.Error())
		return ne
	}
}

func (e *Error) Data() *ResponseErr {
	return &ResponseErr{
		Reason: e.Reason,
		Msg:    e.Msg,
	}
}

func NewError(statusCode int, reason string, msg string) error {
	return &Error{
		StatusCode: statusCode,
		Reason:     reason,
		Msg:        msg,
	}
}
