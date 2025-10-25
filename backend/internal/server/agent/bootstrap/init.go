package bootstrap

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract/embed"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/factory"
	intent2 "github.com/ArtisanCloud/PowerX/internal/server/agent/factory/intent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/intent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"gorm.io/gorm"
	"log"
)

func InitAgentTools(ctx context.Context, cfg *config.AgentConfig, db *gorm.DB) error {
	// 新建Agent Manager
	gAgentManager := agent.GetAgentManager()

	// 新建一个Agent实例
	a, err := factory.NewAgentClient(ctx, cfg)
	if err != nil {
		return err
	}
	// AgentMeta信息
	m := &schemas.AgentMeta{}

	// 注册System Agent
	err = gAgentManager.Register(config.AgentSysKey, a, m)
	if err != nil {
		return err
	}

	// 注册的CRM Agent，兜底走 base_flow（你已有）
	if err = gAgentManager.SetDefaultAgent(config.AgentSysKey, config.BaseFlowKey); err != nil {
		return fmt.Errorf("SetDefaultAgent failed: %w", err)
	}

	// 2) ✨ 先注册内置 handlers（core.debug.* / core.response.format）
	if err := RegisterBuiltinHandlers(); err != nil {
		return fmt.Errorf("RegisterBuiltinHandlers failed: %w", err)
	}

	// 注册一个意图识别的Agent，加载blueprint的意图识别配置
	// * 注意，一定要先注册agent，然后在注册agent的意图识别
	err = RegisterIntentsForAgent(config.AgentSysKey, cfg.FlowSpec.BusinessDir)
	if err != nil {
		return err
	}

	// 打印路由信息
	//specs := gAgentManager.ListFlowRoutesByAgent(config.AgentCRMKey)
	//fmt.Printf("routes specs = %d\n", len(specs))
	//for _, sp := range specs {
	//	fmt.Printf("flow=%s pos=%d neg=%d\n", sp.FlowID, len(sp.Examples.Positive), len(sp.Examples.Negative))
	//}

	// 向量器（OpenAI/Ollama 任选其一）
	vec, err := intent2.NewVectorizerFromConfig(cfg.IntentRecognizer.Embedding)
	if err != nil {
		return fmt.Errorf("init vectorizer: %w", err)
	}
	//diagEmbedding(ctx, vec)

	// LLM 分类器
	//cls, err := intent2.NewClassifierFromConfig(cfg.IntentRecognizer.Classifier)
	//if err != nil {
	//	return err
	//}
	//fmt2.Dump(cls)

	gAgentManager.SetIntentStrategies([]contract.IntentStrategy{
		&intent.RuleStrategy{M: gAgentManager},
		&intent.EmbeddingStrategy{
			M:         gAgentManager,
			Vec:       vec,
			AgentID:   config.AgentSysKey,
			Threshold: 0.90, // ✨
			Alpha:     0.6,  // ✨ 负例惩罚
			Margin:    0.06, // ✨ 边际
		},
		//&intent.LLMStrategy{
		//	M:         gAgentManager,
		//	AgentID:   config.AgentSysKey,
		//	LLM:       cls,
		//	Threshold: 0.85,
		//	//Threshold: 0.6,
		//},
	}, 0.6, 0.95)

	// 接线 DB RunLogger → Manager
	WireAgentRunLogger(db)

	return err
}

func diagEmbedding(ctx context.Context, vec embed.Vectorizer) {
	q := "创建线索"
	qs, err := vec.Embed(ctx, []string{q})
	if err != nil || len(qs) == 0 {
		log.Printf("[EMB] query embed failed: %v", err)
		return
	}
	log.Printf("[EMB] query dim=%d first3=%v", len(qs[0]), qs[0][:min(3, len(qs[0]))])
}
