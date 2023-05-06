CURRENT_DIR := /app
CONFIG_FILE := $(CURRENT_DIR)/etc/powerx.yaml

# BUILD_DIR := $(CURRENT_DIR)/cmd/server/

# 两个编译后的可执行文件，会被放在容器中的根目录下
POWERX_EXE_PATH:=$(CURRENT_DIR)/powerx
POWERX_CTL_EXE_PATH:=$(CURRENT_DIR)/powerxctl

app-init: app-migrate app-seed app-run

app-migrate:
	$(POWERX_CTL_EXE_PATH) database migrate -f $(CONFIG_FILE)

app-seed:
	$(POWERX_CTL_EXE_PATH) database seed -f $(CONFIG_FILE)

app-run:
	$(POWERX_EXE_PATH) -f $(CONFIG_FILE)
