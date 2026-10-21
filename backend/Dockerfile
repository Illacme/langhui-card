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
# 产物放 /out/server（独立路径）：早期版本直接 -o /app，而最终阶段 WORKDIR /app 会先建出
# /app 目录，COPY 于是把二进制塞进目录变成 /app/app，ENTRYPOINT ["/app"] 启动即
# "exec: /app: is a directory"——该镜像在 Docker 里从未真正跑起来过。
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

FROM alpine:3.19
WORKDIR /app
COPY --from=build /out/server /app/server
# migrations 必须进运行镜像：MySQL 形态的表结构由 internal/migrate 按序应用
# （runtime.Caller 在容器里只返回编译期 /src 路径，不可用；migrate 走「可执行文件
# 同级的 migrations/」候选，即 /app/migrations）。data_data 卷只挂 /app/data，
# 不影响这里。
COPY --from=build /src/migrations /app/migrations
EXPOSE 8080
ENTRYPOINT ["/app/server"]
