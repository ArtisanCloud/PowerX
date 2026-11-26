# 从 develop 发布到已安装的 systemd 实例

dev 和 prod 都从 `origin/develop` 构建。需要发布的改动先通过 PR 合入 `develop`。每次选择一个目标环境，记录 Git 提交和发布版本号。服务器上的 systemd 从该环境的 `backend` 和 `web-admin` 软链接启动程序，单独重启只会重新加载当前链接指向的版本。

| 环境 | 运行目录 | 配置文件 | 后端 / 管理端 HTTP 端口 | 发布切换脚本 |
| --- | --- | --- | --- | --- |
| dev | `/opt/powerx-dev` | `/etc/powerx-dev/config.yaml` | 8081 / 3001 | `switch-develop-systemd.sh` |
| prod | `/opt/powerx` | `/etc/powerx/config.yaml` | 8080 / 3000 | `switch-release-systemd.sh` |

### 选择环境并拉取代码

在服务器的同一个 Bash 会话按顺序执行以下步骤。示例选择 dev，发布 prod 时将第一行改为 `deploy_env=prod`。如果两套环境共用源码目录，等当前构建结束再在该目录开始下一次发布。

```bash
deploy_env=dev
set -euo pipefail
case "$deploy_env" in
  dev)  root=/opt/powerx-dev; unit=powerx-dev; api_port=8081; web_port=3001; switcher=switch-develop-systemd.sh ;;
  prod) root=/opt/powerx;     unit=powerx;     api_port=8080; web_port=3000; switcher=switch-release-systemd.sh ;;
  *) echo "请选择 dev 或 prod" >&2; exit 1 ;;
esac
cd ~/workspace/PowerX
```

接下来拉取源码，检查发布目录和回滚版本。

```bash
test "$(git branch --show-current)" = develop
git status --short
git diff --quiet && git diff --cached --quiet || { echo "请先核对部署目录中的本地改动" >&2; exit 1; }
git pull --ff-only origin develop
commit=$(git rev-parse HEAD)
test "$commit" = "$(git rev-parse FETCH_HEAD)"
version="$deploy_env-$(date +%Y%m%d-%H%M%S)-$(git rev-parse --short HEAD)"
previous=$(basename "$(dirname "$(readlink -e "$root/backend")")")
test "$(readlink -e "$root/web-admin")" = "$root/releases/$previous/web-admin"
printf '环境=%s  上一版=%s  新版=%s  提交=%s\n' "$deploy_env" "$previous" "$version" "$commit"
```

核对拉到的提交包含本次改动，并把上一版的版本号记入发布记录。如果本次要把 dev 验收过的版本发到 prod，应比对两次记录的提交 SHA；两次拉取之间可能又有 PR 合入。保留对应发布目录供回滚。工作区有额外源码、分支不符或无法快进时，先确认原因。

### 构建并切换

`NPM_INSTALL=0` 复用已安装且与锁文件一致的前端依赖。首次构建、缺少依赖或锁文件已变更时，用 `NPM_INSTALL=1` 让构建执行 `npm ci`。

```bash
test ! -e "dist/systemd/$version"
test ! -e "$root/releases/$version"
make dist DIST_VERSION="$version" NPM_INSTALL=0
artifact="dist/systemd/$version"
grep -Fx "commit=$commit" "$artifact/manifest.txt"
grep -Fx "version=$version" "$artifact/manifest.txt"
test -x "$artifact/backend/powerx"
test -s "$artifact/web-admin/.output/server/index.mjs"
sudo mv -T -- "$artifact" "$root/releases/$version"
sudo bash "backend/scripts/ops/$switcher" "$version" --without-setup-trace
```

切换脚本更新所选环境的软链接，调用 systemd 重载并重启后端和管理端，然后检查后端健康。dev 脚本会选用 `powerx-dev-*` 服务。启用了独立 Runner 的实例，按原部署方式在切换和回滚时都加上 `--with-runner`。如果版本包含数据库迁移，按该版本的迁移说明执行并单独记录结果。

### 验收和回滚

核对两项服务的运行目录、发布清单中的提交、后端健康响应和管理端 HTTP。以下函数也用于回滚后的验收。最后通过目标环境的实际域名打开本次改动涉及的页面或接口。

```bash
verify_release() (
  set -eu -o pipefail
  expected_version=$1
  expected_commit=$2
  release="$root/releases/$expected_version"
  grep -Fx "commit=$expected_commit" "$release/manifest.txt"
  for component in backend web-admin; do
    service="$unit-$component.service"
    systemctl is-active --quiet "$service"
    pid=$(systemctl show "$service" -p MainPID --value)
    test "$(readlink -e "/proc/$pid/cwd")" = "$release/$component"
    test "$(readlink -e "$root/$component")" = "$release/$component"
  done
  curl --noproxy '*' -fsS --max-time 10 "http://127.0.0.1:$api_port/api/v1/health" |
    python3 -c 'import json, sys
d = json.load(sys.stdin)["data"]
actual = (d.get("status"), d.get("deployment_env"), d.get("version"))
print("health", *actual)
sys.exit(0 if actual == ("ok", sys.argv[1], sys.argv[2]) else 1)' "$deploy_env" "$expected_version"
  curl --noproxy '*' -fsSL --max-time 15 -o /dev/null -w '管理端 HTTP %{http_code}\n' "http://127.0.0.1:$web_port/"
)
```

本次发布执行以下验收。

```bash
verify_release "$version" "$commit"
```

验收失败时，先查看所选服务的日志及当前软链接，确认切换脚本是否已恢复上一版。需要回滚程序时，从发布记录取出上一版，核对其目录和提交，使用**同一环境**的切换脚本。如果失败后已退出 Bash，重新执行最上方的环境选择代码块，按发布记录设置 `previous`，并重新定义 `verify_release` 函数。涉及数据库变更时，先核对上一版与当前数据的兼容性。

```bash
journalctl -u "$unit-backend.service" -u "$unit-web-admin.service" -n 60 --no-pager
readlink -e "$root/backend" "$root/web-admin"
previous_commit=$(sed -n 's/^commit=//p' "$root/releases/$previous/manifest.txt")
test -n "$previous_commit"
sudo bash "backend/scripts/ops/$switcher" "$previous" --without-setup-trace
verify_release "$previous" "$previous_commit"
```
