#!/bin/sh
set -e

# 数据目录下的密钥文件路径
SECRET_FILE="/app/data/.secret_key"

# 如果没有设置环境变量且密钥文件不存在，则生成一个新的
if [ -z "$IMAGE_FUNNEL_SECRET_KEY" ]; then
    if [ -f "$SECRET_FILE" ]; then
        export IMAGE_FUNNEL_SECRET_KEY=$(cat "$SECRET_FILE")
    else
        # 生成 32 字节并进行 base64 编码
        NEW_KEY=$(head -c 32 /dev/urandom | base64)
        echo "$NEW_KEY" > "$SECRET_FILE"
        export IMAGE_FUNNEL_SECRET_KEY="$NEW_KEY"
        echo "Generated new secret key and saved to $SECRET_FILE"
    fi
fi

# 执行原始启动程序
exec /app/image-funnel "$@"
