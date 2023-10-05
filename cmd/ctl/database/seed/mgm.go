package seed

import (
	"PowerX/internal/model/crm/market"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateMGMRules(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&market.MGMRule{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init mgm rules  failed"))
	}

	data := DefaultMGMRules(db)
	if count == 0 {
		if err = db.Model(&market.MGMRule{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init mgm rules failed"))
		}
	}

	return err
}

func DefaultMGMRules(db *gorm.DB) []*market.MGMRule {

	ucDD := powerx.NewDataDictionaryUseCase(db)
	arrayRules := []*market.MGMRule{}
	item, _ := ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneDirectRecruitment)
	rule := &market.MGMRule{
		CommissionRate1: 0.05,
		CommissionRate2: 0,
		Scene:           int(item.Id),
		Description:     "描述：会员A成功招募了新会员B，新会员B在系统内进行了消费。\n\n分佣率：A获得B的总消费额的一定比例，例如5%。",
	}
	arrayRules = append(arrayRules, rule)

	item, _ = ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneIndirectRecruitment)
	rule = &market.MGMRule{
		CommissionRate1: 0.05,
		CommissionRate2: 0.3,
		Scene:           int(item.Id),
		Description:     "描述：会员A招募了新会员B，新会员B成功招募了C。C在系统内进行了消费。\n\n分佣率：A获得C的总消费额的一定比例，例如3%。",
	}
	arrayRules = append(arrayRules, rule)

	item, _ = ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneTeamPerformanceReward)
	rule = &market.MGMRule{
		CommissionRate1: 0.02,
		CommissionRate2: 0,
		Scene:           int(item.Id),
		Description:     "描述：会员A成功招募了多个会员，并带领团队一起推广，团队的总业绩达到一定水平。\n\n分佣率：A获得团队总业绩的一定比例，例如2%。",
	}
	arrayRules = append(arrayRules, rule)

	item, _ = ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneLevelUpgradeReward)
	rule = &market.MGMRule{
		CommissionRate1: 0.05,
		CommissionRate2: 0,
		Scene:           int(item.Id),
		Description:     "描述：会员A的团队中达到一定数量的下级会员，A升级成高级会员。\n\n分佣率：A获得自己及其下级会员的总业绩的一定比例，例如5%。",
	}
	arrayRules = append(arrayRules, rule)

	item, _ = ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneMonthlyRecruitmentCompetition)
	rule = &market.MGMRule{
		CommissionRate1: 0.05,
		CommissionRate2: 0,
		Scene:           int(item.Id),
		Description:     "描述：每个月内，会员A成功拉新的会员数量排名前三的获得额外奖励。\n\n分佣率：第一名获得总消费额的5%，第二名获得总消费额的3%，第三名获得总消费额的2%。\n\n",
	}
	arrayRules = append(arrayRules, rule)

	item, _ = ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneProductPromotionReward)
	rule = &market.MGMRule{
		CommissionRate1: 0.1,
		CommissionRate2: 0,
		Scene:           int(item.Id),
		Description:     "描述：会员A成功推广了某个特定产品，被推广的产品有额外奖励计划。\n\n分佣率：A获得特定产品销售额的一定比例，例如10%。",
	}
	arrayRules = append(arrayRules, rule)

	item, _ = ucDD.GetDataDictionaryItem(context.Background(), market.TypeMGMScene, market.MGMSceneVIPMemberReward)
	rule = &market.MGMRule{
		CommissionRate1: 0.15,
		CommissionRate2: 0,
		Scene:           int(item.Id),
		Description:     "描述：成功招募并维持了高额消费的VIP会员获得额外奖励。\n\n分佣率：VIP会员的消费额度的一定比例，例如15%。",
	}
	arrayRules = append(arrayRules, rule)

	return arrayRules

}
