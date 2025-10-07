package contracts_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NOTE: 占位测试，待实现 Handler 与路由后更新。
func TestAdminMediaAssetsContracts(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/api/v1/admin/media/assets", nil)
	if err != nil {
		t.Fatalf("unexpected error constructing request: %v", err)
	}

	rr := httptest.NewRecorder()

	// 尚未注册路由，调用应失败。占位断言保持测试失败状态，驱动实现阶段补齐。
	t.Fatalf("contract test placeholder — implement router to serve %s (req=%v, recorder=%v)", req.URL.Path, req, rr)
}
