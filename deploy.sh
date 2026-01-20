#!/bin/bash

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 配置
SSH_HOST="a1"
REMOTE_DIR="/opt/natsvr"
REMOTE_BIN="bin/natsvr-cloud"
REMOTE_CONFIG="/opt/natsvr/etc/cloud.yaml"
REMOTE_LOG="/opt/natsvr/data/cloud.log"

build_linux() {
    echo -e "${BLUE}构建 Linux 版本...${NC}"
    
    # 构建前端
    echo -e "${YELLOW}构建前端...${NC}"
    cd web && npm install && npm run build
    cd ..
    
    # 复制前端
    echo -e "${YELLOW}复制前端资源...${NC}"
    rm -rf cmd/cloud/dist
    cp -r web/dist cmd/cloud/dist
    
    # 构建 Linux amd64 后端
    echo -e "${YELLOW}构建 Linux amd64 后端...${NC}"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/cloud-linux-amd64 ./cmd/cloud
    
    echo -e "${GREEN}构建完成: bin/cloud-linux-amd64${NC}"
}

deploy() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  部署 Cloud 到 ${SSH_HOST}${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    # 构建
    build_linux

    # 创建远程目录
    echo -e "${BLUE}创建远程目录...${NC}"
    ssh "$SSH_HOST" "mkdir -p $REMOTE_DIR/bin $REMOTE_DIR/etc $REMOTE_DIR/data"

    # 停止远程服务
    echo -e "${YELLOW}停止远程服务...${NC}"
    ssh "$SSH_HOST" "pkill -f '$REMOTE_BIN' || true"
    sleep 1

    # 上传二进制文件
    echo -e "${BLUE}上传文件...${NC}"
    scp bin/cloud-linux-amd64 "$SSH_HOST:$REMOTE_DIR/$REMOTE_BIN"

    # 设置执行权限
    ssh "$SSH_HOST" "chmod +x $REMOTE_DIR/$REMOTE_BIN"

    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  部署完成!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "二进制文件位置: ${YELLOW}$SSH_HOST:$REMOTE_DIR/$REMOTE_BIN${NC}"
    echo -e "配置文件: ${YELLOW}$SSH_HOST:$REMOTE_CONFIG${NC}"
    echo ""
    echo -e "手动启动命令:"
    echo -e "  ${CYAN}ssh $SSH_HOST '$REMOTE_DIR/$REMOTE_BIN -config $REMOTE_CONFIG'${NC}"
    echo ""
}

deploy_and_start() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  部署并启动 Cloud 到 ${SSH_HOST}${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    # 构建
    build_linux

    # 创建远程目录
    echo -e "${BLUE}创建远程目录...${NC}"
    ssh "$SSH_HOST" "mkdir -p $REMOTE_DIR/bin $REMOTE_DIR/etc $REMOTE_DIR/data"

    # 停止远程服务
    echo -e "${YELLOW}停止远程服务...${NC}"
    ssh "$SSH_HOST" "pkill -f '$REMOTE_BIN' || true"
    sleep 1

    # 上传二进制文件
    echo -e "${BLUE}上传文件...${NC}"
    scp bin/cloud-linux-amd64 "$SSH_HOST:$REMOTE_DIR/$REMOTE_BIN"

    # 设置执行权限并启动
    echo -e "${BLUE}启动服务...${NC}"
    ssh "$SSH_HOST" "chmod +x $REMOTE_DIR/$REMOTE_BIN && nohup $REMOTE_DIR/$REMOTE_BIN -config $REMOTE_CONFIG > $REMOTE_LOG 2>&1 &"
    
    sleep 2

    # 检查是否启动成功
    if ssh "$SSH_HOST" "pgrep -f '$REMOTE_BIN'" > /dev/null; then
        echo -e "${GREEN}========================================${NC}"
        echo -e "${GREEN}  服务启动成功!${NC}"
        echo -e "${GREEN}========================================${NC}"
        echo ""
        echo -e "配置文件: ${YELLOW}$REMOTE_CONFIG${NC}"
        echo -e "日志文件: ${YELLOW}$REMOTE_LOG${NC}"
        echo ""
        echo -e "查看日志: ${CYAN}ssh $SSH_HOST 'tail -f $REMOTE_LOG'${NC}"
        echo -e "停止服务: ${CYAN}$0 stop${NC}"
    else
        echo -e "${RED}服务启动失败，请检查日志${NC}"
        ssh "$SSH_HOST" "cat $REMOTE_LOG" || true
        exit 1
    fi
}

stop_remote() {
    echo -e "${YELLOW}停止远程服务...${NC}"
    ssh "$SSH_HOST" "pkill -f '$REMOTE_BIN' || true"
    echo -e "${GREEN}服务已停止${NC}"
}

status() {
    echo -e "${BLUE}检查远程服务状态...${NC}"
    if ssh "$SSH_HOST" "pgrep -f '$REMOTE_BIN'" > /dev/null 2>&1; then
        echo -e "${GREEN}服务运行中${NC}"
        ssh "$SSH_HOST" "ps aux | grep '$REMOTE_BIN' | grep -v grep"
    else
        echo -e "${YELLOW}服务未运行${NC}"
    fi
}

logs() {
    echo -e "${BLUE}查看远程日志...${NC}"
    ssh "$SSH_HOST" "tail -f $REMOTE_LOG"
}

usage() {
    echo -e "${GREEN}NatSvr Cloud 部署工具${NC}"
    echo ""
    echo "用法: $0 <命令>"
    echo ""
    echo -e "${GREEN}命令:${NC}"
    echo "  deploy              构建并上传到远程服务器 (不启动)"
    echo "  start               构建、上传并启动服务 (使用现有配置)"
    echo "  stop                停止远程服务"
    echo "  status              查看远程服务状态"
    echo "  logs                查看远程日志"
    echo ""
    echo -e "${GREEN}配置:${NC}"
    echo "  远程主机: $SSH_HOST"
    echo "  安装目录: $REMOTE_DIR"
    echo "  配置文件: $REMOTE_CONFIG"
    echo ""
    echo -e "${GREEN}示例:${NC}"
    echo "  $0 deploy    # 仅部署 (不重启)"
    echo "  $0 start     # 部署并重启服务"
    echo "  $0 stop      # 停止服务"
    echo "  $0 status    # 查看状态"
    echo "  $0 logs      # 查看日志"
    echo ""
}

case "${1:-}" in
    deploy)
        deploy
        ;;
    start)
        deploy_and_start
        ;;
    stop)
        stop_remote
        ;;
    status)
        status
        ;;
    logs)
        logs
        ;;
    -h|--help|help)
        usage
        ;;
    *)
        usage
        exit 1
        ;;
esac
