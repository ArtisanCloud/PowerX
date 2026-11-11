package router

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminStaticReadinessRequiresActiveRoute(t *testing.T) {
	r := NewDynamicRouter("/_p", gin.New())
	id := "com.powerx.plugins.fixture"
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("fixture"), 0600))
	require.False(t, r.AdminStaticReady(id, dir), "磁盘上有备用文件，但尚未挂载")
	r.MountAdminStatic(id, dir)
	require.True(t, r.AdminStaticReady(id, dir))
	require.False(t, r.AdminStaticReady(id, t.TempDir()), "其他版本的挂载不能当作当前目录")
	upstream, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)
	r.MountAdminProxy(id, upstream)
	require.False(t, r.AdminStaticReady(id, dir), "代理优先，备用文件不代表已静态降级")
	r.MountAdminStatic(id, dir)
	require.True(t, r.AdminStaticReady(id, dir), "显式挂静态路由后才能报告静态就绪")
	require.NoError(t, os.Remove(filepath.Join(dir, "index.html")))
	require.False(t, r.AdminStaticReady(id, dir))
	r.Unmount(id)
	require.False(t, r.AdminStaticReady(id, dir))
}
