# 多阶段构建：编译期用 golang 镜像，运行期用极简 alpine
# 注意：引入 minio-go / aliyun-oss SDK 后，依赖要求 Go >= 1.26，故构建镜像用 1.26
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app ./cmd/server

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
