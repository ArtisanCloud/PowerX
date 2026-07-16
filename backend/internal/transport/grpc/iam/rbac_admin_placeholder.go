package iam

// IamRbacAdminService 占位说明（T008）
//
// 设计契约来源：
// - specs/026-iam/contracts/iam-rbac-admin.proto
//
// 预期语义：
// - GetMeContext：返回 root/admin/member 视图分流所需上下文字段
// - SwitchTenant：执行租户切换（root 可切非 membership 租户，但必须真实存在）
// - CheckPermission：统一权限检查返回 allowed/reason
//
// 当前状态：
// - HTTP 路由已实现对应语义（/admin/user/auth/me/* 与 /admin/iam/me/check）
// - gRPC 管理语义暂未接入正式服务注册；此文件用于明确契约对齐与后续实现入口。
