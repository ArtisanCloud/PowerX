FROM golang:alpine as builder

ENV GOPROXY https://goproxy.cn,direct

COPY ./ /source/
WORKDIR /source/

RUN go build -o powerX main.go
RUN go build -o powerX-migrate database/migrations/main.go

FROM alpine
# China mirrors
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

RUN apk update --no-cache
RUN apk add --no-cache ca-certificates
RUN apk add --no-cache tzdata
ENV TZ Asia/Shanghai
COPY --from=builder /source/powerX /app/powerX
COPY --from=builder /source/powerX-migrate /app/powerX-migrate

RUN chmod +x /app/powerX
RUN chmod +x /app/powerX-migrate

WORKDIR /app
EXPOSE 80


ENTRYPOINT ["/app/powerX"]
