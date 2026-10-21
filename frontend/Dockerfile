# 多阶段构建：node 编译前端，nginx 托管静态产物并反代后端 API
FROM node:20-alpine AS build
WORKDIR /app
# 必须带上 package-lock.json 并走 npm ci：
# 只 COPY package.json 时 npm install 会做 fresh 解析，npm 10.8.x 的 arborist 在这棵
# peer 依赖树上必崩——Cannot read properties of null (reading 'edgesOut')（已实测复现，
# 带 lock 的 npm install / npm ci 均正常）。npm ci 按 lock 精确安装，顺带可复现、更快。
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
