FROM golang:alpine as builder

ENV GOPROXY https://goproxy.cn,direct

COPY ./ /source/
WORKDIR /source/

RUN go build -o powerx main.go

FROM alpine
# China mirrors
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

RUN apk update --no-cache
RUN apk add --no-cache ca-certificates
RUN apk add --no-cache tzdata
ENV TZ Asia/Shanghai
COPY --from=builder /source/powerx /app/powerx

RUN chmod +x /app/powerx

WORKDIR /app
EXPOSE 80


ENTRYPOINT ["/app/powerx","serve"]
