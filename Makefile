# 根目录 Makefile

# 定义子 make 文件所在目录
MAKE_FILES_DIR := make_files

# 引入所有子 make 文件
include $(MAKE_FILES_DIR)/build.mk
include $(MAKE_FILES_DIR)/clean.mk
include $(MAKE_FILES_DIR)/config.mk
include $(MAKE_FILES_DIR)/dev.mk
include $(MAKE_FILES_DIR)/mcp.mk
include $(MAKE_FILES_DIR)/database.mk
include $(MAKE_FILES_DIR)/docker.mk
include $(MAKE_FILES_DIR)/dist.mk
include $(MAKE_FILES_DIR)/test.mk
include $(MAKE_FILES_DIR)/proto.mk
include $(MAKE_FILES_DIR)/audit_partition.mk
include $(MAKE_FILES_DIR)/perm.mk
include $(MAKE_FILES_DIR)/secret.mk
include $(MAKE_FILES_DIR)/capability.mk
