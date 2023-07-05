CURRENT_DIR := $(shell pwd)
CONFIG_FILE := $(CURRENT_DIR)/etc/powerx-local.yaml

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

DEPLOY_POWERX_EXE_PATH:=$(CURRENT_DIR)/deploy/powerx
DEPLOY_POWERX_CTL_EXE_PATH:=$(CURRENT_DIR)/deploy/powerxctl

DEPLOY_POWERX_EXE_PATH_WINDOWS:=$(CURRENT_DIR)/deploy/powerx.exe
DEPLOY_POWERX_CTL_EXE_PATH_WINDOWS:=$(CURRENT_DIR)/deploy/powerxctl.exe


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
	cp $(POWERX_EXE_PATH_LINUX) $(DEPLOY_POWERX_EXE_PATH)
	cp $(POWERX_CTL_PATH_LINUX) $(DEPLOY_POWERX_CTL_EXE_PATH)

app-build-windows:
	CGO_ENABLED=0  GOOS=windows  GOARCH=amd64 go build -o $(POWERX_EXE_PATH_WINDOWS) $(BUILD_EXE_PATH)
	CGO_ENABLED=0  GOOS=windows  GOARCH=amd64 go build -o $(POWERX_CTL_PATH_WINDOWS) $(BUILD_CTL_PATH)
	cp $(POWERX_EXE_PATH_WINDOWS) $(DEPLOY_POWERX_EXE_PATH_WINDOWS)
	cp $(POWERX_CTL_PATH_WINDOWS) $(DEPLOY_POWERX_CTL_EXE_PATH_WINDOWS)

app-build-macos:
	CGO_ENABLED=0  GOOS=darwin  GOARCH=arm64 go build -o $(POWERX_EXE_PATH_MAC_OS) $(BUILD_EXE_PATH)
	CGO_ENABLED=0  GOOS=darwin  GOARCH=arm64 go build -o $(POWERX_CTL_PATH_MAC_OS) $(BUILD_CTL_PATH)
	cp $(POWERX_EXE_PATH_MAC_OS) $(DEPLOY_POWERX_EXE_PATH)
	cp $(POWERX_CTL_PATH_MAC_OS) $(DEPLOY_POWERX_CTL_EXE_PATH)

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




.PHONY: go
build: ## Compilation main.go to iss file
	#@go build -o app  main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app1 cmd/server/powerx.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ctl cmd/ctl/powerxctl.go
gen:
	goctl api go -api ./api/powerx.api -dir .
swag:
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkdepartment.json" -api api/admin/scrm/organization/weworkdepartment.api -dir swagger
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkemployee.json" -api api/admin/scrm/organization/weworkemployee.api -dir swagger

	goctl api plugin -plugin goctl-swagger="swagger -filename weworkgroup.json" -api api/admin/scrm/app/weworkgroup.api -dir swagger
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkappmessage.json" -api api/admin/scrm/app/weworkappmessage.api -dir swagger
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkapp.json" -api api/admin/scrm/app/weworkapp.api -dir swagger
	#wechat.bot
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkbot.json" -api api/admin/scrm/bot/weworkbot.api -dir swagger
	#wechat.customer
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkcustomer.json" -api api/admin/scrm/customer/weworkcustomer.api -dir swagger
	goctl api plugin -plugin goctl-swagger="swagger -filename weworkcustomergroup.json" -api api/admin/scrm/customer/weworkcustomergroup.api -dir swagger

	#goctl api plugin -plugin goctl-swagger="swagger -filename admin.json" -api api/admin.api -dir swagger



.PHONY: cp
pro:#
	scp -r app1 root@101.132.69.159:/www/wwwroot/scrm.site
	scp -r ctl root@101.132.69.159:/www/wwwroot/scrm.site
	# Aliyun23120
conf:
	scp -r etc root@101.132.69.159:/www/wwwroot/scrm.site

minio:
	nohup ./minio >> logs/minio.log &

ssh:
	ssh root@47.103.27.100
	#Aliyun23629

##################################服务商
#通讯录
#地址： https://work.weixin.qq.com/wework_admin/frame#/apps/contactsApi/showAlarm
#可信IP
#Secret: 	oZZX1rCLwDLReIXbVju8qZbHSosTWUU1PfWa6t_fXRM



##################################开发者
# 客户
# 地址： https://work.weixin.qq.com/wework_admin/frame#customer/analysis
# Secret: 2G59kbylNSbItSkhMTEkuZBd_Ai2raSAu4mSWf7CmHo

# 组织架构同步
# 地址： https://work.weixin.qq.com/wework_admin/frame#/dataMigration/index
# Secret: R9IhLYZ_03jstM3JnicLMHQDsiagLFX9DhIIVjOcwyU


# 配置回调授权码
#./tunnel config set auth xxxxxxxxxxxxxx
# 配置本地应用服务的端口
#./tunnel config set port 8090
# 配置本地应用服务的指令回调地址
#./tunnel config set commandCallback http://127.0.0.1:8881/webhook/wework/message
# 配置本地应用服务的数据回调地址
#./tunnel config set dataCallback http://127.0.0.1:8881/webhook/wework/callback

# http://127.0.0.1:8881/webhook/wework/message?echostr=dXcIqV7J5CBlqm4GqHsbyUCVGy83WbbayfiJBYo3SIQnifXXdKT2N6d6G3AuLKat5uqtNx2l6KAB8nWk7HnaBw%3D%3D&msg_signature=b456eb27cf5838974971e70225da33903acbea1c&nonce=1687440812&timestamp=1687680674
