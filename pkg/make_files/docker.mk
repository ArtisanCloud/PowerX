# Docker 相关任务
# 用于容器化部署和管理

# Docker 镜像配置
DOCKER_IMAGE := $(PROJECT_NAME):$(VERSION)
DOCKER_REGISTRY := registry.example.com
DOCKER_TAG := latest

# Docker 构建
docker-build:
	@echo "$(CYAN)构建 Docker 镜像...$(NC)"
	@docker build -t $(DOCKER_IMAGE) .
	@docker tag $(DOCKER_IMAGE) $(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "$(GREEN)✅ Docker 镜像构建完成: $(DOCKER_IMAGE)$(NC)"

# 运行容器
docker-run:
	@echo "$(CYAN)启动 Docker 容器...$(NC)"
	@docker run -d \
		--name $(PROJECT_NAME) \
		-p 8077:8077 \
		-e ENV=production \
		$(DOCKER_IMAGE)
	@echo "$(GREEN)✅ 容器启动完成: $(PROJECT_NAME)$(NC)"

# 推送镜像
docker-push:
	@echo "$(CYAN)推送 Docker 镜像...$(NC)"
	@docker tag $(DOCKER_IMAGE) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE)
	@echo "$(GREEN)✅ 镜像推送完成$(NC)"

# 停止容器
docker-stop:
	@echo "$(CYAN)停止 Docker 容器...$(NC)"
	@docker stop $(PROJECT_NAME) || true
	@echo "$(GREEN)✅ 容器已停止$(NC)"

# 查看日志
docker-logs:
	@echo "$(CYAN)查看容器日志...$(NC)"
	@docker logs -f $(PROJECT_NAME)

# 进入容器
docker-exec:
	@echo "$(CYAN)进入容器...$(NC)"
	@docker exec -it $(PROJECT_NAME) /bin/sh

# 清理 Docker 资源
docker-clean:
	@echo "$(CYAN)清理 Docker 资源...$(NC)"
	@docker stop $(PROJECT_NAME) 2>/dev/null || true
	@docker rm $(PROJECT_NAME) 2>/dev/null || true
	@docker rmi $(DOCKER_IMAGE) 2>/dev/null || true
	@echo "$(GREEN)✅ Docker 资源清理完成$(NC)"

# 清理系统
docker-prune:
	@echo "$(CYAN)清理未使用的 Docker 资源...$(NC)"
	@docker system prune -f
	@echo "$(GREEN)✅ Docker 系统清理完成$(NC)"

.PHONY: docker-build docker-run docker-push docker-stop docker-logs docker-exec docker-clean docker-prune