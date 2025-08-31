package iam

import "gorm.io/datatypes"

// 创建：只要求 name、key；其余可选
type CreateDepartmentReq struct {
	Name           string         `json:"name" binding:"required"`
	Key            *string        `json:"key,omitempty"`
	ParentID       *uint64        `json:"parent_id,omitempty"`
	Sort           *int           `json:"sort,omitempty"`
	LeaderMemberID *uint64        `json:"leader_member_id,omitempty"`
	Status         *int16         `json:"status,omitempty"` // 1=active
	Meta           datatypes.JSON `json:"meta,omitempty"`
}

// 更新：用指针区分“未传”和“置空/修改”
type UpdateDepartmentReq struct {
	Name           *string         `json:"name,omitempty"`
	Key            *string         `json:"key,omitempty"`
	NewParentID    *uint64         `json:"new_parent_id,omitempty"` // 为空不移动
	Sort           *int            `json:"sort,omitempty"`
	LeaderMemberID *uint64         `json:"leader_member_id,omitempty"`
	Status         *int16          `json:"status,omitempty"`
	Meta           *datatypes.JSON `json:"meta,omitempty"`
}

type DepartmentDTO struct {
	ID             uint64         `json:"id"`
	TenantID       uint64         `json:"tenant_id"`
	Name           string         `json:"name"`
	Key            string         `json:"key"`
	ParentID       *uint64        `json:"parent_id,omitempty"`
	Path           string         `json:"path"`
	Depth          int            `json:"depth"`
	Sort           int            `json:"sort"`
	LeaderMemberID *uint64        `json:"leader_member_id,omitempty"`
	Status         int16          `json:"status"`
	Meta           datatypes.JSON `json:"meta,omitempty"`
}
