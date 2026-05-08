// pkg/cmd/database/seed/dept_sme.go
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	tenantRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

// 中小企业常见部门树（按 sort 排序）
// - 可根据你的业务随时增删
type deptNode struct {
	Key      string
	Name     string
	Sort     int
	Status   int16 // 1=active
	Meta     map[string]any
	Children []deptNode
}

// 你也可以改成从配置文件读取
// 中小企业部门树（分中心 + 子部门 + 组），按 sort 排序
func defaultSMETree() []deptNode {
	return []deptNode{
		{Key: "exec", Name: "总经办", Sort: 10, Status: 1},
		{Key: "hr", Name: "人力资源部", Sort: 20, Status: 1},
		{Key: "finance", Name: "财务部", Sort: 30, Status: 1},

		// === 市场中心（将 设计、销售 归口到这里）===
		{
			Key: "marketing-center", Name: "市场中心", Sort: 40, Status: 1, Meta: map[string]any{"type": "center"},
			Children: []deptNode{
				{
					Key: "marketing", Name: "市场部", Sort: 10, Status: 1,
					Children: []deptNode{
						{Key: "brand", Name: "品牌", Sort: 10, Status: 1},
						{Key: "growth", Name: "增长", Sort: 20, Status: 1},
						// 需要可再加：{Key:"pr", Name:"公关", Sort:30, Status:1},
						// 或 {Key:"perf", Name:"效果投放", Sort:40, Status:1},
					},
				},
				{
					Key: "design", Name: "设计部", Sort: 20, Status: 1,
					Children: []deptNode{
						{Key: "design-uiux", Name: "UI/UX", Sort: 10, Status: 1},
						{Key: "design-visual", Name: "视觉设计", Sort: 20, Status: 1},
					},
				},
				{
					Key: "sales", Name: "销售部", Sort: 30, Status: 1,
					Children: []deptNode{
						{Key: "sales-domestic", Name: "国内销售", Sort: 10, Status: 1},
						{Key: "sales-channel", Name: "渠道销售", Sort: 20, Status: 1},
						// 需要可再加：{Key:"sales-keyaccount", Name:"大客户", Sort:30, Status:1},
					},
				},
			},
		},

		// === 运营中心（可选：把客服/数据/商运放这里）===
		{
			Key: "ops", Name: "运营中心", Sort: 50, Status: 1, Meta: map[string]any{"type": "center"},
			Children: []deptNode{
				{Key: "biz-ops", Name: "业务运营", Sort: 10, Status: 1},
				{Key: "data", Name: "数据", Sort: 20, Status: 1},
				// 如需把客服也归口到运营中心：把下面 support 移到这里
				// {Key:"support", Name:"客服部", Sort:30, Status:1},
			},
		},

		// === 信息中心（把 研发 归口到这里）===
		{
			Key: "it", Name: "信息中心/IT", Sort: 60, Status: 1, Meta: map[string]any{"type": "center"},
			Children: []deptNode{
				{
					Key: "rnd", Name: "研发部", Sort: 10, Status: 1,
					Children: []deptNode{
						{Key: "backend", Name: "后端组", Sort: 10, Status: 1},
						{Key: "frontend", Name: "前端组", Sort: 20, Status: 1},
						{Key: "qa", Name: "测试QA", Sort: 30, Status: 1},
						{Key: "devops", Name: "运维/DevOps", Sort: 40, Status: 1},
					},
				},
				{Key: "it-infra", Name: "信息化/基础设施", Sort: 20, Status: 1},
				// 还可加：{Key:"sec", Name:"安全", Sort:30, Status:1},
			},
		},

		// 其余独立条线（如不想独立，也可以搬家到上面各中心）
		{Key: "support", Name: "客服部", Sort: 70, Status: 1},
		{Key: "legal", Name: "法务部", Sort: 80, Status: 1},
		{Key: "procurement", Name: "采购部", Sort: 90, Status: 1},
	}
}

// SeedSMEDepartments 为指定租户初始化“中小企业部门树”。
// - tenantKey 例： "system"（也可传你的业务租户 Key）
// - 幂等：多次执行不会重复插入
func SeedSMEDepartments(db *gorm.DB, tenantKey string) error {
	tenantRepo := tenantRepo.NewTenantRepository(db)
	deptRepo := infraiam.NewDepartmentRepository(db)

	// 1) 拿到租户（不存在可选择 Ensure/Find，这里沿用 EnsureByKey）
	ten, err := tenantRepo.EnsureByKey(seedCtx(), tenantKey, "SME Org", tenant.TenantPlanBasic, tenant.TenantTypeEnterprise)
	if err != nil {
		return fmt.Errorf("ensure tenant(%s): %w", tenantKey, err)
	}

	// 2) 构造部门树并逐层 ensure（父先于子）
	tree := defaultSMETree()

	// 用事务保证一致性（含 Move）
	tenantUUID := ten.UUID.String()

	return db.WithContext(seedCtx()).Transaction(func(tx *gorm.DB) error {
		// 递归创建/更新
		for _, root := range tree {
			if _, err := ensureDeptNode(tx, deptRepo, tenantUUID, nil, root); err != nil {
				return err
			}
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] departments ready for tenant=%s (uuid=%s)", tenantKey, tenantUUID)
		return nil
	})
}

// ensureDeptNode：确保一个节点存在且在正确父级下；必要时移动并更新属性。
// parentID == nil 表示根部门
func ensureDeptNode(tx *gorm.DB, repo *infraiam.DepartmentRepository, tenantUUID string, parentID *uint64, node deptNode) (*dbm.Department, error) {
	ctx := context.WithValue(seedCtx(), struct{}{}, nil)

	// 1) 先查是否已存在（tenant + key 唯一）
	exist, err := repo.FindByKey(ctx, tenantUUID, node.Key)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("find dept key=%s: %w", node.Key, err)
	}

	// 构造 meta JSON（可选）
	var meta datatypes.JSON
	if node.Meta != nil {
		if b, e := json.Marshal(node.Meta); e == nil {
			meta = datatypes.JSON(b)
		}
	}

	if exist == nil {
		// 2) 不存在：创建（使用 CreateWithPath 以生成 path/depth）
		newDept := &dbm.Department{
			TenantUUID: tenantUUID,
			Key:        node.Key,
			Name:       node.Name,
			ParentID:   parentID,
			Sort:       node.Sort,
			Status:     node.Status,
			Meta:       meta,
		}
		if err := repo.CreateWithPath(ctx, newDept); err != nil {
			return nil, fmt.Errorf("create dept key=%s: %w", node.Key, err)
		}
		exist = newDept
	} else {
		// 3) 已存在：必要时移动到新父级（批量修正 path/depth）
		needMove := false
		switch {
		case parentID == nil && exist.ParentID != nil:
			needMove = true
		case parentID != nil:
			if exist.ParentID == nil || *exist.ParentID != *parentID {
				needMove = true
			}
		}
		if needMove {
			if err := repo.Move(ctx, tenantUUID, exist.ID, parentID); err != nil {
				return nil, fmt.Errorf("move dept key=%s: %w", node.Key, err)
			}
		}
		// 4) 同步名称/排序/状态/Meta 等
		updates := map[string]any{
			"name":   node.Name,
			"sort":   node.Sort,
			"status": node.Status,
		}
		// 只有当你想更新 meta 时再带上（避免把 NULL/零值写坏）
		if len(meta) > 0 {
			updates["meta"] = meta
		}
		if err := tx.Model(&dbm.Department{}).
			Where("tenant_uuid = ? AND id = ?", tenantUUID, exist.ID).
			Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("update dept key=%s: %w", node.Key, err)
		}
	}

	// 5) 递归处理子节点
	for _, ch := range node.Children {
		if _, err := ensureDeptNode(tx, repo, tenantUUID, &exist.ID, ch); err != nil {
			return nil, err
		}
	}

	return exist, nil
}
