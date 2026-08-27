#!/bin/bash
# AIOps Platform 生产模式管理脚本
# 用法:
#   ./aiops-ctl.sh start    - 启动服务（生产模式，Go 托管 React）
#   ./aiops-ctl.sh stop     - 停止服务
#   ./aiops-ctl.sh restart  - 重启服务
#   ./aiops-ctl.sh status   - 查看状态
#   ./aiops-ctl.sh logs     - 查看日志
#   ./aiops-ctl.sh build    - 重新构建前端+后端

set -e

PROJECT_DIR="/Users/dbmac250804/Desktop/elk/aiops-platform"
BACKEND_BIN="/tmp/aiops-server"
LOG_FILE="/tmp/aiops-server.log"
PID_FILE="/tmp/aiops-server.pid"
PORT=8080

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

get_pid() {
    if [ -f "$PID_FILE" ]; then
        local pid=$(cat "$PID_FILE" 2>/dev/null)
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            echo "$pid"
            return 0
        fi
    fi
    # fallback: 从端口找
    lsof -tiTCP:$PORT -sTCP:LISTEN 2>/dev/null | head -1
}

is_running() {
    local pid=$(get_pid)
    [ -n "$pid" ] && return 0 || return 1
}

cmd_build() {
    echo -e "${YELLOW}构建前端...${NC}"
    cd "$PROJECT_DIR/frontend"
    npm run build 2>&1 | tail -3

    echo -e "${YELLOW}构建后端...${NC}"
    cd "$PROJECT_DIR/backend"
    go build -o "$BACKEND_BIN" ./cmd/server
    echo -e "${GREEN}构建完成${NC}"
}

cmd_start() {
    if is_running; then
        echo -e "${YELLOW}服务已在运行 (PID $(get_pid))${NC}"
        return 0
    fi

    # 确保二进制存在
    if [ ! -f "$BACKEND_BIN" ]; then
        echo -e "${YELLOW}二进制不存在，先构建...${NC}"
        cmd_build
    fi

    echo -e "${YELLOW}启动 AIOps Platform (生产模式)...${NC}"
    cd "$PROJECT_DIR/backend"
    nohup "$BACKEND_BIN" > "$LOG_FILE" 2>&1 &
    local pid=$!
    echo "$pid" > "$PID_FILE"

    # 等待启动
    sleep 4
    if kill -0 "$pid" 2>/dev/null; then
        echo -e "${GREEN}启动成功 (PID $pid)${NC}"
        echo "  地址: http://localhost:$PORT"
        echo "  日志: $LOG_FILE"
    else
        echo -e "${RED}启动失败，查看日志: $LOG_FILE${NC}"
        tail -20 "$LOG_FILE"
        return 1
    fi
}

cmd_stop() {
    local pid=$(get_pid)
    if [ -z "$pid" ]; then
        echo -e "${YELLOW}服务未运行${NC}"
        return 0
    fi
    echo -e "${YELLOW}停止服务 (PID $pid)...${NC}"
    kill "$pid" 2>/dev/null || true
    # 等待优雅关闭
    for i in 1 2 3 4 5; do
        if ! kill -0 "$pid" 2>/dev/null; then
            break
        fi
        sleep 1
    done
    # 强制 kill
    if kill -0 "$pid" 2>/dev/null; then
        echo -e "${YELLOW}强制终止...${NC}"
        kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$PID_FILE"
    echo -e "${GREEN}已停止${NC}"
}

cmd_restart() {
    cmd_stop
    sleep 1
    cmd_start
}

cmd_status() {
    if is_running; then
        local pid=$(get_pid)
        echo -e "${GREEN}● 运行中${NC} (PID $pid)"
        ps -o pid,rss,%cpu,%mem,etime,command -p "$pid" 2>/dev/null | tail -1
        echo ""
        echo -e "  地址: http://localhost:$PORT"
        # 健康检查
        local health=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/health" 2>/dev/null)
        echo -e "  健康: $([ "$health" = "200" ] && echo "${GREEN}OK${NC}" || echo "${RED}FAIL ($health)${NC}")"
    else
        echo -e "${RED}● 未运行${NC}"
    fi
}

cmd_logs() {
    if [ -f "$LOG_FILE" ]; then
        tail -50 "$LOG_FILE"
    else
        echo "无日志文件"
    fi
}

case "${1:-status}" in
    start)   cmd_start ;;
    stop)    cmd_stop ;;
    restart) cmd_restart ;;
    status)  cmd_status ;;
    logs)    cmd_logs ;;
    build)   cmd_build ;;
    *)
        echo "用法: $0 {start|stop|restart|status|logs|build}"
        exit 1
        ;;
esac
