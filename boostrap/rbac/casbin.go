package rbac

import (
	"github.com/ArtisanCloud/PowerX/boostrap/rbac/global"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
)
import gormAdapter "github.com/casbin/gorm-adapter/v3"

func InitCasbin(db *gorm.DB) (err error) {

	adapter, err := gormAdapter.NewAdapterByDB(db)
	if err != nil {
		return err
	}

	mdl, err := model.NewModelFromString(`
	[request_definition]
	r = sub, obj, act
	
	[policy_definition]
	p = sub, obj, act
	
	[role_definition]
	g = _, _
	
	[policy_effect]
	e = some(where (p.eft == allow))
	
	[matchers]
	m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
`)
	if err != nil {
		return err
	}

	global.G_Enforcer, err = casbin.NewEnforcer(mdl, adapter)
	if err != nil {
		return err
	}
	// 开启权限认证日志
	global.G_Enforcer.EnableLog(true)

	//err = global.G_Enforcer.LoadPolicy()
	//if err != nil {
	//	return err
	//}

	//allPolicies := global.G_Enforcer.GetPolicy()
	//fmt.Dump(allPolicies)

	// 加载角色分组规则
	//_, err = global.G_Enforcer.AddRoleForUser("123", "admin", "123", "321")
	//if err != nil {
	//	logger.Logger.Error("AddRoleForUser error:", zap.Any("err", err))
	//	return err
	//}

	//_, err = global.G_Enforcer.AddPolicy("matrix", "url/list", "write")
	//_, err = global.G_Enforcer.AddPolicy("matrix", "url/list", "read", "uuid", "sourceName")

	//filterd := global.G_Enforcer.GetFilteredPolicy(3, "uuid")
	//fmt.DD("filter;", filterd)

	return err
}
