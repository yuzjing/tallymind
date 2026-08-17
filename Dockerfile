# ==========================================
# 第一阶段：构建阶段
# ==========================================
FROM golang:alpine AS builder

WORKDIR /build

# 1. 利用 Docker 缓存机制，优先下载依赖
COPY go.mod go.sum ./
# RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download

# 2. 复制源码并编译为无依赖静态二进制文件
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tallymind ./cmd/tallymind/main.go

# ==========================================
# 第二阶段：采用 Alpine 极简镜像
# ==========================================
FROM alpine:latest

# 安装 CA 证书 (确保访问 Gemini/企微 HTTPS 不报证书错) 并设置时区
RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从第一阶段仅复制编译好的静态二进制文件
COPY --from=builder /build/tallymind .

# 暴露端口
EXPOSE 8080

CMD ["./tallymind"]