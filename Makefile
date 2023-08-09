CURRENT_DIR := $(shell pwd)
CONFIG_FILE := $(CURRENT_DIR)/etc/powerx.yaml

# 设定需要编译的go文件目录
BUILD_EXE_PATH := $(CURRENT_DIR)/cmd/server/powerx.go
BUILD_CTL_PATH := $(CURRENT_DIR)/cmd/ctl/powerxctl.go


PATH_BUILD := $(CURRENT_DIR)/.build
PATH_BUILD_LINUX := $(PATH_BUILD)/linux
PATH_BUILD_WINDOWS := $(PATH_BUILD)/windows
PATH_BUILD_MAC_OS := $(PATH_BUILD)/macos

# 将编译好的执行文件，放入根目录下
POWERX_EXE_PATH:=$(CURRENT_DIR)/powerx
POWERX_CTL_EXE_PATH:=$(CURRENT_DIR)/powerxctl

POWERX_EXE_PATH_LINUX := $(PATH_BUILD_LINUX)/powerx
POWERX_CTL_PATH_LINUX := $(PATH_BUILD_LINUX)/powerxctl

POWERX_EXE_PATH_WINDOWS := $(PATH_BUILD_WINDOWS)/powerx.exe
POWERX_CTL_PATH_WINDOWS := $(PATH_BUILD_WINDOWS)/powerxctl.exe

POWERX_EXE_PATH_MAC_OS := $(PATH_BUILD_MAC_OS)/powerx
POWERX_CTL_PATH_MAC_OS := $(PATH_BUILD_MAC_OS)/powerxctl

DEPLOY_POWERX_EXE_PATH:=$(CURRENT_DIR)/../PowerXDocker/powerx
DEPLOY_POWERX_CTL_EXE_PATH:=$(CURRENT_DIR)/../PowerXDocker/powerxctl

DEPLOY_POWERX_EXE_PATH_WINDOWS:=$(CURRENT_DIR)/../PowerXDocker/powerx.exe
DEPLOY_POWERX_CTL_EXE_PATH_WINDOWS:=$(CURRENT_DIR)/../PowerXDocker/powerxctl.exe


app-init: app-migrate app-seed app-run
app-init-db: app-migrate app-seed

app-migrate:
	go build -o $(POWERX_CTL_EXE_PATH) $(BUILD_CTL_PATH)
	$(POWERX_CTL_EXE_PATH) database migrate -f $(CONFIG_FILE)

app-seed:
	go build -o $(POWERX_CTL_EXE_PATH) $(BUILD_CTL_PATH)
	$(POWERX_CTL_EXE_PATH) database seed -f $(CONFIG_FILE)

app-run:
	go build -o $(POWERX_EXE_PATH) $(BUILD_EXE_PATH)
	$(POWERX_EXE_PATH) -f $(CONFIG_FILE)


# ------

app-build-linux:
	CGO_ENABLED=0  GOOS=linux  GOARCH=amd64 go build -o $(POWERX_EXE_PATH_LINUX) $(BUILD_EXE_PATH)
	CGO_ENABLED=0  GOOS=linux  GOARCH=amd64 go build -o $(POWERX_CTL_PATH_LINUX) $(BUILD_CTL_PATH)
#	cp $(POWERX_EXE_PATH_LINUX) $(DEPLOY_POWERX_EXE_PATH)
#	cp $(POWERX_CTL_PATH_LINUX) $(DEPLOY_POWERX_CTL_EXE_PATH)

app-build-windows:
	CGO_ENABLED=0  GOOS=windows  GOARCH=amd64 go build -o $(POWERX_EXE_PATH_WINDOWS) $(BUILD_EXE_PATH)
	CGO_ENABLED=0  GOOS=windows  GOARCH=amd64 go build -o $(POWERX_CTL_PATH_WINDOWS) $(BUILD_CTL_PATH)
#	cp $(POWERX_EXE_PATH_WINDOWS) $(DEPLOY_POWERX_EXE_PATH_WINDOWS)
#	cp $(POWERX_CTL_PATH_WINDOWS) $(DEPLOY_POWERX_CTL_EXE_PATH_WINDOWS)

app-build-macos:
	CGO_ENABLED=0  GOOS=darwin  GOARCH=arm64 go build -o $(POWERX_EXE_PATH_MAC_OS) $(BUILD_EXE_PATH)
	CGO_ENABLED=0  GOOS=darwin  GOARCH=arm64 go build -o $(POWERX_CTL_PATH_MAC_OS) $(BUILD_CTL_PATH)
#	cp $(POWERX_EXE_PATH_MAC_OS) $(DEPLOY_POWERX_EXE_PATH)
#	cp $(POWERX_CTL_PATH_MAC_OS) $(DEPLOY_POWERX_CTL_EXE_PATH)

# ------

build-goctl-powerx-apis:
	goctl api go -api ./api/powerx.api -dir .



# ------

IMAGE_NAME := powerx
IMAGE_TAG := latest

build-image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) CURRENT_DIR/deploy/docker

run-container:
	docker run -it $(IMAGE_NAME):$(IMAGE_TAG) /bin/bash




