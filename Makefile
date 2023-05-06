CURRENT_DIR := $(shell pwd)
CONFIG_FILE := $(CURRENT_DIR)/etc/powerx-local.yaml

BUILD_DIR := $(CURRENT_DIR)/cmd/server/
TARGET_PATH:=$(CURRENT_DIR)/powerx

app-init: app-migrate app-seed app-run

app-migrate:
	$(GOPATH)/bin/powerxctl database migrate -f $(CONFIG_FILE)

app-seed:
	$(GOPATH)/bin/powerxctl database seed -f $(CONFIG_FILE)

app-run:
	go build -o $(TARGET_PATH) $(BUILD_DIR)
	$(TARGET_PATH) -f $(CONFIG_FILE)

build-goctl-powerx-apis:
	goctl api go -api ./api/powerx.api -dir .



# ------

IMAGE_NAME := powerx
IMAGE_TAG := latest

build-image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) CURRENT_DIR/deploy/docker

run-container:
	docker run -it $(IMAGE_NAME):$(IMAGE_TAG) /bin/bash
