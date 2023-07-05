package test

import (
	"PowerX/internal/config"
	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/core/conf"
	"path/filepath"
	"testing"
)

var svcCtx *svc.ServiceContext

// maybe use flag
const configFile = "../etc/powerx_test.yaml"

// TestMain 会在启动test包内任何测试之前被调用, 以下的TestMain初始化了一个svcCtx, 用于带状态的uc测试, 可以使用单独一份test配置在测试库里跑带状态的测试
func TestMain(m *testing.M) {
	var c config.Config

	conf.MustLoad(configFile, &c)
	c.EtcDir = filepath.Dir(configFile)

	svcCtx = svc.NewServiceContext(c)
	m.Run()
}
