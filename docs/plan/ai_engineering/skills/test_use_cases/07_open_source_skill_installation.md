# L7 - 开源 Skill 包安装到 PowerX

## 目标

验证开源 skill 包的“拉取 -> 校验 -> 注册 -> 发布 -> 调用”全流程。

## 前置条件

1. 已选定一个开源 skill（仓库地址、tag/commit）
2. PowerX 托管存储可用（S3/MinIO/本地）
3. 管理员 Token 可用

## 操作步骤

### 步骤 1：拉取并镜像到托管存储

保留 `source_url` 与 `source_commit_or_tag`。

### 步骤 2：生成 checksum（可选 signature）

记录到导入元数据。

### 步骤 3：导入并注册为 draft

调用 `admin/skills/import` 或等效流程。

### 步骤 4：发布并绑定 capability

发布后绑定 `com.powerx.skill.xxx.invoke`。

### 步骤 5：执行一次 tenant 调用

走 `tenant/invocations` 或 `tenant/skills/invoke`。

## 预期效果

1. 来源信息可追溯。  
2. 校验失败时禁止发布。  
3. 发布后可正常调用。

## 通过标准

1. 从导入到调用每一步都有审计记录。  
2. 对同一来源可重复导入并保证版本可控。

