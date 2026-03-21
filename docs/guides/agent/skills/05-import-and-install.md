# Skills 导入与多源安装指南（Admin）

本文对齐当前实现：支持 upload 导入，也支持通过 `install-tasks` 从多种 Git 源安装。

## 1. 两条链路

1. `upload` 导入（管理面板）
- 适合已有 skill bundle（`s3://...tgz`）的场景。
- 仍然需要 `bundle_uri/checksum` 等底层字段。

2. `install-tasks` 安装（推荐）
- 面向“从仓库安装”的场景。
- 用户只需要填写仓库与目录信息，不需要手填 `bundle_uri/checksum/signature`。

## 2. install-tasks：用户需要填写什么

必填：
- `path`：仓库内 skill 目录（例如 `ln-001-standards-researcher`）
- `repo` 或 `repo_url`：二选一

推荐填写：
- `provider`：`github | gitlab | gitee | bitbucket | generic_git`
  - 不填时会自动推断（`repo=owner/repo` 默认按 `github` 处理）

可选：
- `ref`：分支/标签，默认 `main`
- `source`：`third_party`（默认）或 `plugin`
- `skill_id`：不填则使用 `path` 的 basename
- `version`：默认 `1.0.0`
- `method`：`auto`（默认），可选 `git` / `download`
- `auto_import`：默认 `true`

说明：
- `repo` 形态：`owner/repo`（如 `openai/skills`）
- `repo_url` 形态：完整地址（如 `https://gitlab.com/group/repo.git`）

## 3. install-tasks 调用示例

### 3.1 GitHub（短格式 repo）

```bash
curl -sS -X POST "$HTTP_BASE/admin/skills/install-tasks" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "github",
    "repo": "openai/skills",
    "path": "skills/.curated/gh-address-comments",
    "ref": "main",
    "source": "third_party",
    "auto_import": true
  }'
```

`path` 必须是仓库内某个具体 Skill 目录，且该目录下有 `SKILL.md`。

### 3.2 GitLab/Gitee（完整 repo_url）

```bash
curl -sS -X POST "$HTTP_BASE/admin/skills/install-tasks" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "gitlab",
    "repo_url": "https://gitlab.com/example-org/skills-repo.git",
    "path": "skills/doc-researcher",
    "ref": "main",
    "auto_import": true
  }'
```

### 3.3 任意 Git 仓库（generic_git）

```bash
curl -sS -X POST "$HTTP_BASE/admin/skills/install-tasks" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "generic_git",
    "repo_url": "ssh://git@example.com/team/skills.git",
    "path": "skills/my-skill",
    "ref": "release/v1",
    "auto_import": true
  }'
```

## 4. 任务状态追踪

创建后返回 `202` + `task_id`，然后轮询：

```bash
curl -sS "$HTTP_BASE/admin/skills/install-tasks/<task_id>" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

状态：
- `pending`
- `running`
- `success`
- `failed`（查看 `error_summary/stderr_log`）

Web Admin 弹窗默认会通过 WebSocket 订阅实时状态（`_topic.system.notification`，`kind=skills.install.task`），优先实时更新；若超时未收到事件，再提示去列表查询。

列表查询支持按 `provider/repo/skill_id/status` 过滤：

```bash
curl -sS "$HTTP_BASE/admin/skills/install-tasks?provider=github&status=success&page=1&page_size=20" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

## 5. 成功判定

1. `task.status=success`
2. `auto_import=true` 时，Registry 中存在对应技能：

```bash
curl -sS "$HTTP_BASE/admin/skills?skill_id=skill.thirdparty.demo" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

## 6. Web Admin 弹窗使用

页面路径：`左侧菜单 -> 技能库（右上角“导入/安装 Skill”）`  
弹窗内有两种模式：
- `仓库安装（推荐）`：填写 `provider + repo/repo_url + path`，后台走 `install-tasks`。
- `Upload 导入`：用于已上传 bundle 的管理员流程。

## 7. 常见错误

- `repo or repo_url is required`
  - 需要至少提供一个仓库定位字段。

- `repo must be in owner/repo format`
  - `repo` 字段格式不合法，改用 `owner/repo`。

- `repo_url host does not match provider xxx`
  - `provider` 与 `repo_url` 域名不一致（例如 provider=github，但 URL 是 gitlab.com）。

- `download method is only supported by github installer`
  - 非 GitHub 场景请使用 `method=auto` 或 `git`。

- `SKILL.md not found in selected skill directory`
  - `path` 指向的目录不是 Skill 根目录，或目录结构不正确。
