package skills

import (
	"context"
	"fmt"

	agentauthz "github.com/ArtisanCloud/PowerX/internal/service/agent_authz"
	evidencesvc "github.com/ArtisanCloud/PowerX/internal/service/agent_evidence"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pluginmanager "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gorm.io/gorm"
)

// NewRuntimeDependencyService 是当前 Core 的统一生产装配。没有 Workflow/脚本端口，
// 内置 typed 计算/V5 需要受控存储及显式新 Revision；外部工具仅开放已安装且固定原生实现的 HTTP/admin_user 计算合同。
func NewRuntimeDependencyService(db *gorm.DB) *DependencyService {
	checks, _ := NewRuntimeSkillTools(db, pluginmanager.CurrentPluginManager)
	return checks
}

// NewRuntimeSkillTools 是预检和实际调用的同一装配；测试可注入实际隔离 Manager。
func NewRuntimeSkillTools(db *gorm.DB, plugins PluginRuntimeProvider) (*DependencyService, *evidence.ToolService) {
	tools := evidence.NewToolService(evidence.ToolServiceOptions{})
	var pins *pluginToolRuntime
	options := DependencyServiceOptions{
		Executors:         map[string]bool{"llm_prompt": true, "capability": true},
		ResponseContracts: map[string]bool{evidence.ReportSchema: true, evidence.ReportSchemaV5: db != nil},
	}
	if db != nil {
		pins = newPluginToolRuntime(db, plugins)
		options.EvidenceReady = evidencesvc.NewService(db).Ready
		options.Catalog = pins.catalog
		options.Authorize = runtimeToolAuthorizer(agentauthz.NewService(db), iamsvc.NewRBACService(db))
		tools = evidence.NewToolService(evidence.ToolServiceOptions{Capability: pins.prepare})
	}
	options.PinnedTools = func(ctx context.Context, in DependencyCheckInput, tool ToolDependencyV2, snapshot *skillrepo.ToolCapabilitySnapshot) error {
		if tool.Binding.Kind != "builtin" {
			if pins == nil {
				return dependencyError(in, "skill.tool_protocol_unavailable", tool.DependencyKey, nil)
			}
			return pins.check(ctx, in, tool, snapshot)
		}
		if err := tools.CheckCalculator(tool); err != nil {
			return dependencyError(in, "skill.tool_dependency_version_or_contract_mismatch", tool.DependencyKey, err)
		}
		return nil
	}
	checks := NewDependencyService(options)
	if pins != nil {
		pins.checks = checks
	}
	return checks, tools
}

func NewRuntimeToolService() *evidence.ToolService {
	return evidence.NewToolService(evidence.ToolServiceOptions{})
}

func runtimeToolAuthorizer(agents *agentauthz.Service, rbac *iamsvc.RBACService) ToolAuthorizer {
	return func(ctx context.Context, in DependencyCheckInput, tool ToolDependencyV2, snapshot *skillrepo.ToolCapabilitySnapshot) error {
		// 不从 Skill payload/context map 读取 root、用户或 member；缺失受信任身份即拒绝。
		if reqctx.GetTenantUUID(ctx) != in.TenantUUID || reqctx.GetUserUUID(ctx) == "" || (reqctx.GetMemberID(ctx) == 0 && !reqctx.IsRoot(ctx)) || tool.Permission.Actor != "admin_user" {
			return fmt.Errorf("skill.tool_dependency_denied")
		}
		if in.Stage != DependencyPublish {
			if in.AgentID == 0 {
				return fmt.Errorf("skill.tool_dependency_denied")
			}
			result, err := agents.AuthorizeCapability(ctx, agentauthz.AuthorizeInput{
				Env: in.Env, TenantUUID: in.TenantUUID, UserUUID: reqctx.GetUserUUID(ctx),
				MemberID: reqctx.GetMemberID(ctx), IsRoot: reqctx.IsRoot(ctx), AgentID: in.AgentID,
				CapabilityID: tool.ToolKey, PermissionCode: tool.Permission.Code,
			})
			if err != nil {
				return err
			}
			if !result.Allowed {
				return fmt.Errorf("skill.tool_dependency_denied")
			}
			return nil
		}
		module, resource, action, ok := agentauthz.ParsePermissionCodeForPlugin(tool.Permission.Code, snapshot.Record.PluginID)
		if !ok {
			return fmt.Errorf("skill.tool_dependency_denied")
		}
		allowed, err := rbac.Enforce(ctx, iamsvc.ActorContext{IsRoot: reqctx.IsRoot(ctx), TenantUUID: in.TenantUUID}, in.TenantUUID, reqctx.GetMemberID(ctx), module, resource, action)
		if err != nil {
			return err
		}
		if !allowed {
			return fmt.Errorf("skill.tool_dependency_denied")
		}
		return nil
	}
}
