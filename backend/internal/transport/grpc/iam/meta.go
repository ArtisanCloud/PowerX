package iam

import (
    "context"
    "strings"
    "time"

    commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
)

// okMeta 统一成功响应元信息
func okMeta(_ context.Context, reqID string) *commonv1.ResponseMeta {
    return &commonv1.ResponseMeta{Code: 200, Message: "success", Timestamp: time.Now().Unix(), RequestId: reqID}
}

// badMeta 统一错误响应元信息
func badMeta(_ context.Context, code int32, msg, reqID string) *commonv1.ResponseMeta {
    return &commonv1.ResponseMeta{Code: code, Message: strings.TrimSpace(msg), Timestamp: time.Now().Unix(), RequestId: reqID}
}

