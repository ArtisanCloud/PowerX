package rbac

import "github.com/gin-gonic/gin"

// Checker 接口留给将来接入真正的权限系统
type Checker interface {
	Check(c *gin.Context, resource, action string) bool
}

var global Checker = allowAll{}

// SetGlobal 可在真实 RBAC 准备好时替换实现
func SetGlobal(c Checker) { global = c }

// Check 供业务侧直接调用
func Check(c *gin.Context, resource, action string) bool {
	if global == nil {
		return true
	}
	return global.Check(c, resource, action)
}

// 先用放行实现，确保联调不被权限卡住
type allowAll struct{}

func (allowAll) Check(_ *gin.Context, _, _ string) bool { return true }
