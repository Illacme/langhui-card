# 多阶段构建：编译期用 golang 镜像，运行期用极简 alpine
# 注意：引入 minio-go / aliyun-oss SDK 后，依赖要求 Go >= 1.26，故构建镜像用 1.26
FROM golang:1.26-alpine AS build
WORKDIR /src
# GOPROXY 兜底链：国内网络直连 proxy.golang.org 超时是常见死法（go mod download 卡住/失败），
# 故默认先走 goproxy.cn（全球可达的 CDN），失败再回退官方代理与 direct。
# 非大陆用户想强制官方源：docker build --build-arg GOPROXY=https://proxy.golang.org,direct .
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./cmd/server

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
