# 第一阶段
FROM golang:1.20-alpine AS builder

# 配置go proxy为中国国内proxy
ENV GOPROXY=https://goproxy.cn,direct

# 拷贝当前目录到docker内
WORKDIR /app
COPY . .

# 编译安装 powerxctl
RUN cd /app/cmd/ctl && go install powerxctl.go

# 对cmd/server目录执行go build
RUN cd /app/cmd/server && go build -o powerx

# 第二阶段
FROM alpine:latest

# 安装编译工具和依赖项
RUN apt-get update && apt-get install -y build-essential

# 拷贝文件
COPY --from=builder /app/cmd/server/powerx /app/powerx
COPY --from=builder /app/etc /app/etc

WORKDIR /app

EXPOSE 8888

# 运行可执行文件
CMD ["make", "-f", "/app/Makefile","-C", "/app", "app-init"]