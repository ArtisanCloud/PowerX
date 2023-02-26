package handler

import (
	"PowerX/internal/types/errorx"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
)

func ErrorHandle(err error) (int, interface{}) {
	switch e := err.(type) {
	case *errorx.Error:
		return e.StatusCode, e.Data()
	default:
		logx.Error(err)
		invalid := errorx.WithCause(errorx.ErrBadRequest, err.Error()).(*errorx.Error)
		return invalid.StatusCode, invalid.Data()
	}
}

func ErrorHandleCtx(ctx context.Context, err error) (int, interface{}) {
	switch e := err.(type) {
	case *errorx.Error:
		return e.StatusCode, e.Data()
	default:
		logx.WithContext(ctx).Debug(err)
		invalid := errorx.WithCause(errorx.ErrBadRequest, err.Error()).(*errorx.Error)
		return invalid.StatusCode, invalid.Data()
	}
}
