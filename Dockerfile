# Stage 1: Build Front-end
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable && corepack prepare pnpm@10.28.1 --activate
WORKDIR /app

# 复制依赖定义文件
COPY pnpm-lock.yaml pnpm-workspace.yaml package.json ./
COPY frontend/package.json ./frontend/

# 安装依赖
RUN pnpm install --frozen-lockfile

# 复制前端源码
COPY frontend/ ./frontend/
# 复制 GraphQL 定义（如果前端构建需要读取 schema）
COPY graph/ ./graph/

# 执行构建
RUN pnpm --filter image-funnel-frontend run build

# Stage 2: Build Back-end
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend-builder
WORKDIR /app

# 安装 git 以便下载依赖
RUN apk add --no-cache git

# 复制依赖定义文件
COPY go.mod go.sum ./
RUN go mod download

# 复制全量源码以确保生成代码和引用的内部包都存在
COPY . .

# 构建参数：由 Docker Buildx 自动传入
ARG TARGETOS
ARG TARGETARCH
# 构建参数：版本号，由 GitHub Action 传入
ARG VERSION=dev

# 构建二进制文件
# CGO_ENABLED=0 确保静态链接
ENV GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    CGO_ENABLED=0
RUN go test ./... && \
    go build \
    -ldflags "-X main.version=${VERSION} -s -w" \
    -o image-funnel ./cmd/server

# Stage 3: Final Runtime
FROM alpine:3.21
WORKDIR /app

# 安装运行时依赖
# imagemagick: 后端图像处理核心组件
# ca-certificates: HTTPS 支持
# tzdata: 时区支持
RUN apk add --no-cache \
    imagemagick \
    ca-certificates \
    tzdata

# 设置环境变量默认值
ENV IMAGE_FUNNEL_PORT=80 \
    IMAGE_FUNNEL_ROOT_DIR=/app/workspace

# 从之前的阶段复制构建产物
# 将前端静态文件放在二进制文件同级的 dist 目录下，符合 main.go 的生产环境查找逻辑
COPY --from=backend-builder /app/image-funnel /app/image-funnel
COPY --from=frontend-builder /app/frontend/dist /app/dist
COPY deployments/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# 创建持久化数据目录
RUN mkdir -p /app/workspace /app/data

# 暴露默认端口
EXPOSE 80

# 指定挂载点
VOLUME ["/app/workspace", "/app/data"]

# 启动程序
ENTRYPOINT ["/app/docker-entrypoint.sh"]
