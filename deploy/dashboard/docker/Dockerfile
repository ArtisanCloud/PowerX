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
COPY --from=builder /app/nginx/nginx.conf /etc/nginx/nginx.conf



# RUN mkdir -p /var/log/nginx/server
# RUN touch /var/log/nginx/server/access.log
# RUN touch /var/log/nginx/server/error.log

# RUN mkdir -p /var/log/nginx/web
# RUN touch /var/log/nginx/web/access.log
# RUN touch /var/log/nginx/web/error.log

RUN mkdir -p /var/log/nginx/server
RUN mkdir -p /var/log/nginx/web
RUN chmod -R 777 /var/log/nginx

# 暴露80端口
EXPOSE 80

# 启动nginx
CMD ["nginx", "-g", "daemon off;"]
