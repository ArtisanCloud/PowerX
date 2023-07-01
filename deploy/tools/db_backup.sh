#!/bin/bash

# 配置数据库连接信息
DB_HOST="localhost"
DB_PORT="5432"
DB_NAME="your_database_name"
DB_USER="your_username"
DB_PASSWORD="your_password"

# 配置备份文件保存路径
BACKUP_DIR="/path/to/backup/directory"

# 配置日期和时间格式
DATE=$(date +"%Y%m%d")
TIME=$(date +"%H%M%S")

# 构建备份文件名
BACKUP_FILE="${DB_NAME}_${DATE}_${TIME}.sql"

# 创建备份文件保存目录
mkdir -p "$BACKUP_DIR"

# 导出数据库备份
pg_dump -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$BACKUP_DIR/$BACKUP_FILE"

# 检查导出结果
if [ $? -eq 0 ]; then
    echo "数据库备份已成功导出到: $BACKUP_DIR/$BACKUP_FILE"
else
    echo "导出数据库备份时出现错误"
fi
