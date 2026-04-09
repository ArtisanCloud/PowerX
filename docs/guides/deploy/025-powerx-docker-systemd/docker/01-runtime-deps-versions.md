# 00. Docker 运行时依赖与版本（Ubuntu）

## 1. 目标
在 Ubuntu 上安装并固化 Docker Engine + Docker Compose 插件，满足 PowerX Docker 部署基线。

## 2. 版本基线
- Docker Engine：`24.x+`（建议）
- Docker Compose 插件：`v2.20+`
- 统一矩阵：`../01-runtime-version-matrix.md`

## 3. Ubuntu 安装（官方仓库）

```bash
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg

sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo \"$VERSION_CODENAME\") stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
```

## 4. 启动与开机自启

```bash
sudo systemctl enable --now docker
sudo systemctl status docker --no-pager
```

可选：将当前用户加入 `docker` 组（避免每次 `sudo`）：

```bash
sudo usermod -aG docker $USER
newgrp docker
```

## 5. 自检命令

```bash
docker --version
docker compose version
docker info
```

预期：
- `docker compose version` 为 `v2.20+`
- `docker info` 无连接 daemon 报错

## 6. 仓库登录（GHCR）

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
```

## 7. 常见问题

1. `permission denied while trying to connect to the Docker daemon socket`  
执行 `sudo usermod -aG docker $USER` 后重新登录会话。

2. `docker compose: command not found`  
未安装 `docker-compose-plugin`，请回到第 3 步补装。

3. 公司网络访问 Docker Hub/GHCR 慢  
先确认代理已生效，再执行 `docker pull`/`docker compose pull`。
