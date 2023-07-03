# 第一阶段
FROM golang:1.20-alpine AS builder

# 配置go proxy为中国国内proxy
ENV GOPROXY=https://goproxy.cn,direct

# 拷贝当前目录到docker内
WORKDIR /app
RUN ls -l 
COPY ./ /app/
COPY ./etc/ /app/etc/



# 第二阶段
FROM alpine:latest

# 安装编译工具和依赖项
RUN apk update && apk add build-base

# 拷贝文件
COPY --from=builder /app/powerxctl /app/powerxctl
COPY --from=builder /app/Makefile /app/Makefile
COPY --from=builder /app/powerx /app/powerx
COPY --from=builder /app/etc/ /app/etc/

RUN chmod +x /app/powerxctl
RUN chmod +x /app/Makefile
RUN chmod +x /app/powerx

WORKDIR /app

EXPOSE 8888

# 运行可执行文件
CMD ["make", "-f", "/app/Makefile","-C", "/app", "app-init"]