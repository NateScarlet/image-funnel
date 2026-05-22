# 阶段 1：构建前端
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

# 配置 npm/pnpm 和 Corepack 的镜像源参数以支持国内网络环境，默认使用官方源避免空值引起 URL 解析错误
ARG npm_config_registry=https://registry.npmjs.org
ARG pnpm_config_registry=${npm_config_registry}
ARG COREPACK_NPM_REGISTRY=${npm_config_registry}

WORKDIR /app

# 复制依赖定义文件
COPY pnpm-lock.yaml pnpm-workspace.yaml package.json ./
COPY frontend/package.json ./frontend/

# 拉取依赖并缓存到 pnpm 存储库（仅当依赖定义文件变动时才会使缓存失效）
RUN --mount=type=cache,target=/root/.pnpm-store \
    --mount=type=cache,target=/root/.cache/node/corepack \
    corepack enable && \
    pnpm fetch

# 复制前端源码和 GraphQL 架构定义
COPY frontend/ ./frontend/
COPY graph/ ./graph/


# 在同一层内进行离线依赖链接、构建及 node_modules 清理，避免垃圾文件残留到中间层中
RUN --mount=type=cache,target=/root/.pnpm-store \
    --mount=type=cache,target=/root/.cache/node/corepack \
    corepack enable && \
    pnpm install --offline --frozen-lockfile && \
    pnpm --filter image-funnel-frontend run build && \
    rm -rf node_modules/ frontend/node_modules/

# 阶段 2：构建后端
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend-builder

# 支持替换 Alpine 的源地址以加速 apk 包安装
ARG ALPINE_MIRROR_URL
RUN if [ -n "${ALPINE_MIRROR_URL}" ]; then \
    sed -i "s@https\?://dl-cdn.alpinelinux.org/alpine@${ALPINE_MIRROR_URL}@g" /etc/apk/repositories; \
    fi

# 配置 Go 模块代理
ARG GOPROXY
ENV GOPROXY=${GOPROXY}

WORKDIR /app

# 安装 git 并缓存 apk 索引与包文件
RUN --mount=type=cache,target=/var/cache/apk \
    apk update && apk add --no-cache git

# 复制依赖定义文件并利用缓存拉取 go 模块
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 复制后端和共享逻辑的全部源码
COPY . .

# 自动由 Docker Buildx 传入的目标操作系统和架构
ARG TARGETOS
ARG TARGETARCH
# 版本号参数，默认值为 dev
ARG VERSION=dev

ENV GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    CGO_ENABLED=0

# 挂载编译器缓存和模块依赖缓存，执行单元测试并编译二进制文件
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go test ./... && \
    go build \
    -ldflags "-X main.version=${VERSION} -s -w" \
    -o image-funnel ./cmd/server

# 阶段 3：最终运行环境
FROM alpine:3.21

# 支持运行时替换 Alpine 的源地址
ARG ALPINE_MIRROR_URL
RUN if [ -n "${ALPINE_MIRROR_URL}" ]; then \
    sed -i "s@https\?://dl-cdn.alpinelinux.org/alpine@${ALPINE_MIRROR_URL}@g" /etc/apk/repositories; \
    fi

WORKDIR /app

# 安装必要的系统运行时依赖
RUN apk add --no-cache \
    imagemagick \
    ca-certificates \
    tzdata

# 设置应用默认环境变量
ENV IMAGE_FUNNEL_PORT=80 \
    IMAGE_FUNNEL_ROOT_DIR=/app/workspace \
    IMAGE_FUNNEL_ENABLE_DIRECTORY_STATS_CACHE=false

# 从前序构建阶段拷贝产物和执行脚本
COPY --from=backend-builder /app/image-funnel /app/image-funnel
COPY --from=frontend-builder /app/frontend/dist /app/dist
COPY deployments/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# 预先创建持久化所需的目录
RUN mkdir -p /app/workspace /app/data

# 声明暴露的端口和挂载数据卷
EXPOSE 80
VOLUME ["/app/workspace", "/app/data"]

# 设定入口执行脚本
ENTRYPOINT ["/app/docker-entrypoint.sh"]
