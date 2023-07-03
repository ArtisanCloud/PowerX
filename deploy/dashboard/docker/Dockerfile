# 第一阶段
FROM node:19 AS builder


# 拷贝项目文件到容器中
WORKDIR /app
# COPY ./dashboard/dist /app/dist
COPY ./dashboard /app/

# 安装依赖
# RUN #yarn install

# 执行yarn build
# RUN yarn build:notsc

# ------------------------------------------------

# 第二阶段
FROM nginx:latest

# 拷贝构建产物到nginx容器中
COPY --from=builder /app/dist /usr/share/nginx/html
COPY --from=builder /app/nginx.conf /etc/nginx/conf.d/default.conf

RUN chmod -R 777 /var/log/

# 暴露80端口
EXPOSE 80

# 启动nginx
CMD ["nginx", "-g", "daemon off;"]
