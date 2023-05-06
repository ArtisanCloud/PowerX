CURRENT_DIR := $(shell pwd)
CONFIG_FILE := $(CURRENT_DIR)/etc/powerx-local.yaml

# 设定需要编译的go文件目录
BUILD_EXE_DIR := $(CURRENT_DIR)/cmd/server/powerx.go
BUILD_CTL_DIR := $(CURRENT_DIR)/cmd/ctl/powerxctl.go

# 将编译好的执行文件，放入根目录下
POWERX_EXE_PATH:=$(CURRENT_DIR)/powerx
POWERX_CTL_EXE_PATH:=$(CURRENT_DIR)/powerxctl

app-init: app-migrate app-seed app-run

app-migrate:
	go build -o $(POWERX_CTL_EXE_PATH) $(BUILD_CTL_DIR)
	$(POWERX_CTL_EXE_PATH) database migrate -f $(CONFIG_FILE)

app-seed:
	go build -o $(POWERX_CTL_EXE_PATH) $(BUILD_CTL_DIR)
	$(POWERX_CTL_EXE_PATH) database seed -f $(CONFIG_FILE)

app-run:
	go build -o $(POWERX_EXE_PATH) $(BUILD_EXE_DIR)
	$(POWERX_EXE_PATH) -f $(CONFIG_FILE)

build-goctl-powerx-apis:
	goctl api go -api ./api/powerx.api -dir .



# ------

IMAGE_NAME := powerx
IMAGE_TAG := latest

build-image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) CURRENT_DIR/deploy/docker

run-container:
	docker run -it $(IMAGE_NAME):$(IMAGE_TAG) /bin/bash
