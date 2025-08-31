package rbac

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/gin-gonic/gin"
)

type Subject struct{ TenantID, UserID string }
type Result struct {
	Allow  bool
	Fields []string // 允许读/写的字段白名单
	Cond   string   // 可附加 domain 条件，供查询合并
}

type Checker interface {
	Check(ctx context.Context, sub Subject, resource, action string, attrs map[string]any) (Result, error)
}

type noopChecker struct{}

func NewChecker() Checker { return &noopChecker{} }
func (noopChecker) Check(context.Context, Subject, string, string, map[string]any) (Result, error) {
	return Result{Allow: true, Fields: nil, Cond: ""}, nil
}

func SubjectFromContext(c *gin.Context) Subject {
	var sub Subject

	sub.TenantID = fmt.Sprintf("%d", auth.GetTenantID(c.Request.Context()))
	sub.UserID = fmt.Sprintf("%d", auth.GetUserID(c.Request.Context()))

	return sub
}
