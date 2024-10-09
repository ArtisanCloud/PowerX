package market

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type MGMRule struct {
	powermodel.PowerModel

	Name            string  `gorm:"comment:规则名字" json:"name"`
	CommissionRate1 float32 `gorm:"type:decimal(10,2); comment:分佣率1" json:"commissionRate1"`
	CommissionRate2 float32 `gorm:"type:decimal(10,2); comment:分佣率2" json:"commissionRate2"`
	Scene           int     `gorm:"comment:场景码" json:"scene"`
	Description     string  `gorm:"comment:场景描述" json:"description"`
}

const MGMRuleUniqueId = powermodel.UniqueId

func (mdl *MGMRule) TableName() string {
	return model.PowerXSchema + "." + model.TableNameMGMRule
}

func (mdl *MGMRule) GetTableName(needFull bool) string {
	tableName := model.TableNameMGMRule
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

const (
	TypeMGMScene = "_mgm_scene"

	// 直接会员招募
	MGMSceneDirectRecruitment = "_direct_recruitment" // "直接会员招募",
	// 间接会员招募
	MGMSceneIndirectRecruitment = "_indirect_recruitment" // "间接会员招募",
	// 团队业绩奖励（仅限两层）
	MGMSceneTeamPerformanceReward = "_team_performance_reward" // "团队业绩奖励（仅限两层）",
	// 级别升级奖励
	MGMSceneLevelUpgradeReward = "_level_upgrade_reward" // "级别升级奖励",
	// 月度拉新竞赛（仅限两层）
	MGMSceneMonthlyRecruitmentCompetition = "_monthly_recruitment_competition" // "月度拉新竞赛（仅限两层）",
	// 推广特定产品奖励
	MGMSceneProductPromotionReward = "_product_promotion_reward" // "推广特定产品奖励",
	// VIP会员奖励
	MGMSceneVIPMemberReward = "_vip_member_reward" // "VIP会员奖励",
)

type CustomerChannel struct {
	powermodel.PowerModel

	CustomerID int64  `gorm:"comment:客户ID" json:"customerId"`
	ChannelID  int    `gorm:"comment:渠道ID" json:"channelId"`
	Code       string `gorm:"comment:渠道码" json:"code"`
}

func (mdl *CustomerChannel) TableName() string {
	return model.PowerXSchema + "." + model.TableNameCustomerChannel
}

func (mdl *CustomerChannel) GetTableName(needFull bool) string {
	tableName := model.TableNameCustomerChannel
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// InviteRecord 表示会员邀请记录
type InviteRecord struct {
	powermodel.PowerModel

	InviterID      int64  `gorm:"comment:邀请人ID" json:"inviterId"`
	InviteeID      int64  `gorm:"comment:被邀请人ID" json:"inviteeId"`
	InvitationCode string `gorm:"comment:邀请码" json:"invitationCode"`
	MgmSceneId     int    `gorm:"comment:MGM场景ID" json:"mgmSceneId"`
}

func (mdl *InviteRecord) TableName() string {
	return model.PowerXSchema + "." + model.TableNameInviteRecord
}

func (mdl *InviteRecord) GetTableName(needFull bool) string {
	tableName := model.TableNameInviteRecord
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// CommissionRecord 表示分佣记录
type CommissionRecord struct {
	powermodel.PowerModel

	InviterID     int64   `gorm:"comment:邀请人ID" json:"inviterId"`
	InviteeID     int64   `gorm:"comment:被邀请人ID" json:"inviteeId"`
	Amount        float64 `gorm:"comment:分佣金额" json:"amount"`
	OperationType string  `gorm:"comment:操作对象类型" json:"operationType"`
	OperationId   int64   `gorm:"comment:操作对象ID" json:"operationId"`
}

func (mdl *CommissionRecord) TableName() string {
	return model.PowerXSchema + "." + model.TableNameCommissionRecord
}

func (mdl *CommissionRecord) GetTableName(needFull bool) string {
	tableName := model.TableNameCommissionRecord
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// RewardRecord 表示奖励记录
type RewardRecord struct {
	powermodel.PowerModel

	CustomerID    int64   `gorm:"comment:会员ID" json:"customerId"`
	Amount        float64 `gorm:"comment:奖励金额" json:"amount"`
	OperationType string  `gorm:"comment:操作对象类型" json:"operationType"`
	OperationId   int64   `gorm:"comment:操作对象ID" json:"operationId"`
}

func (mdl *RewardRecord) TableName() string {
	return model.PowerXSchema + "." + model.TableNameRewardRecord
}

func (mdl *RewardRecord) GetTableName(needFull bool) string {
	tableName := model.TableNameRewardRecord
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
