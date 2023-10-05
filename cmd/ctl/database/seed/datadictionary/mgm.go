package datadictionary

import (
	"PowerX/internal/model"
	"PowerX/internal/model/crm/market"
)

func defaultMGMDataDictionary() *model.DataDictionaryType {
	return &model.DataDictionaryType{
		Items: []*model.DataDictionaryItem{
			&model.DataDictionaryItem{
				Key:   market.MGMSceneDirectRecruitment,
				Type:  market.TypeMGMScene,
				Name:  "直接会员招募",
				Value: market.MGMSceneDirectRecruitment,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MGMSceneIndirectRecruitment,
				Type:  market.TypeMGMScene,
				Name:  "间接会员招募",
				Value: market.MGMSceneIndirectRecruitment,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MGMSceneTeamPerformanceReward,
				Type:  market.TypeMGMScene,
				Name:  "团队业绩奖励（仅限两层）",
				Value: market.MGMSceneTeamPerformanceReward,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MGMSceneLevelUpgradeReward,
				Type:  market.TypeMGMScene,
				Name:  "级别升级奖励",
				Value: market.MGMSceneLevelUpgradeReward,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MGMSceneMonthlyRecruitmentCompetition,
				Type:  market.TypeMGMScene,
				Name:  "月度拉新竞赛（仅限两层）",
				Value: market.MGMSceneMonthlyRecruitmentCompetition,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MGMSceneProductPromotionReward,
				Type:  market.TypeMGMScene,
				Name:  "推广特定产品奖励",
				Value: market.MGMSceneProductPromotionReward,
				Sort:  0,
			},
			&model.DataDictionaryItem{
				Key:   market.MGMSceneVIPMemberReward,
				Type:  market.TypeMGMScene,
				Name:  "VIP会员奖励",
				Value: market.MGMSceneVIPMemberReward,
				Sort:  0,
			},
		},
		Type:        market.TypeMGMScene,
		Name:        "MGM场景类型",
		Description: "各种MGM 客户转介绍的类型",
	}

}
