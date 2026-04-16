#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

PID_FILE="$SCRIPT_DIR/.pinhaoclaw.pid"
LOG_FILE="/tmp/pinhaoclaw.log"
DEFAULT_PORT=9000

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; }

# ── 获取运行中的 PID ──
get_pid() {
    if [ -f "$PID_FILE" ]; then
        local pid
        pid=$(cat "$PID_FILE" 2>/dev/null)
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            echo "$pid"
            return 0
        fi
        # PID 文件过期，清理
        rm -f "$PID_FILE"
    fi
    # 回退：通过端口查找
    local port_pid
    port_pid=$(ss -tlnp 2>/dev/null | grep ":${DEFAULT_PORT}" | grep -oP 'pid=\K[0-9]+' | head -1)
    if [ -n "$port_pid" ]; then
        echo "$port_pid"
        return 0
    fi
    return 1
}

is_running() {
    get_pid >/dev/null 2>&1
}

# ── 检查依赖 ──
check_deps() {
    local missing=()
    command -v go >/dev/null 2>&1 || missing+=("go")
    command -v node >/dev/null 2>&1 || missing+=("node")
    command -v npm >/dev/null 2>&1 || missing+=("npm")

    if [ ${#missing[@]} -gt 0 ]; then
        err "缺少依赖: ${missing[*]}"
        err "请先安装 Go (1.22+) 和 Node.js (18+)"
        exit 1
    fi
}

# ── 构建前端 ──
build_frontend() {
    info "构建前端..."
    cd "$SCRIPT_DIR/pinhaoclaw-frontend"

    if [ ! -d "node_modules" ]; then
        info "安装前端依赖..."
        npm install --registry=https://registry.npmmirror.com
    fi

    npm run build:h5

    if [ ! -f "dist/build/h5/index.html" ]; then
        err "前端构建失败：未找到 dist/build/h5/index.html"
        exit 1
    fi
    ok "前端构建完成"
    cd "$SCRIPT_DIR"
}

# ── 编译后端 ──
build_backend() {
    info "编译后端..."
    cd "$SCRIPT_DIR"

    CGO_ENABLED=0 go build -o pinhaoclaw .

    if [ ! -f "pinhaoclaw" ]; then
        err "后端编译失败"
        exit 1
    fi
    ok "后端编译完成: $(du -h pinhaoclaw | cut -f1)"
}

# ── 准备 .env ──
prepare_env() {
    if [ ! -f ".env" ]; then
        if [ -f ".env.example" ]; then
            cp .env.example .env
            warn "已从 .env.example 创建 .env，请根据实际情况修改"
        else
            warn "未找到 .env 文件，将使用默认配置"
        fi
    fi
}

# ── 前台启动 ──
start_foreground() {
    local port="${1:-$DEFAULT_PORT}"
    info "启动 PinHaoClaw (端口: $port)..."
    info "按 Ctrl+C 停止"
    echo ""
    ./pinhaoclaw -p "$port"
}

# ── 后台启动 ──
start_daemon() {
    local port="${1:-$DEFAULT_PORT}"

    # 检查端口是否已被占用
    local existing_pid
    existing_pid=$(ss -tlnp 2>/dev/null | grep ":${port}" | grep -oP 'pid=\K[0-9]+' | head -1)
    if [ -n "$existing_pid" ]; then
        err "端口 $port 已被占用 (PID: $existing_pid)，请先 ./start.sh stop"
        exit 1
    fi

    if [ ! -f "pinhaoclaw" ]; then
        err "未找到 pinhaoclaw 二进制文件，请先运行 ./start.sh build"
        exit 1
    fi

    prepare_env
    info "启动 PinHaoClaw (端口: $port, 后台运行)..."
    nohup ./pinhaoclaw -p "$port" > "$LOG_FILE" 2>&1 &
    local pid=$!
    echo "$pid" > "$PID_FILE"
    sleep 1

    if kill -0 "$pid" 2>/dev/null; then
        ok "PinHaoClaw 已启动 (PID: $pid, 日志: $LOG_FILE)"
    else
        err "启动失败，请查看日志: $LOG_FILE"
        rm -f "$PID_FILE"
        exit 1
    fi
}

# ── 停止 ──
do_stop() {
    local pid
    pid=$(get_pid) || true

    if [ -z "$pid" ]; then
        # 再检查端口是否有进程
        local port_pid
        port_pid=$(ss -tlnp 2>/dev/null | grep ":${DEFAULT_PORT}" | grep -oP 'pid=\K[0-9]+' | head -1)
        if [ -n "$port_pid" ]; then
            pid="$port_pid"
        else
            warn "PinHaoClaw 未在运行"
            return 0
        fi
    fi

    info "停止 PinHaoClaw (PID: $pid)..."
    kill "$pid" 2>/dev/null

    # 等待进程退出，最多 5 秒
    local i=0
    while [ $i -lt 5 ] && kill -0 "$pid" 2>/dev/null; do
        sleep 1
        i=$((i + 1))
    done

    if kill -0 "$pid" 2>/dev/null; then
        warn "进程未响应 SIGTERM，发送 SIGKILL..."
        kill -9 "$pid" 2>/dev/null
    fi

    rm -f "$PID_FILE"
    ok "PinHaoClaw 已停止"
}

# ── 重启 ──
do_restart() {
    if is_running; then
        do_stop
    fi
    start_daemon "$1"
}

# ── 查看状态 ──
do_status() {
    if is_running; then
        local pid
        pid=$(get_pid)
        ok "PinHaoClaw 运行中 (PID: $pid)"
        # 尝试健康检查
        local port="${1:-$DEFAULT_PORT}"
        local health
        health=$(curl -s --connect-timeout 2 "http://localhost:$port/health" 2>/dev/null || true)
        if [ -n "$health" ]; then
            info "健康检查: $health"
        fi
    else
        warn "PinHaoClaw 未运行"
    fi
}

# ── 查看日志 ──
do_log() {
    if [ -f "$LOG_FILE" ]; then
        tail -n 50 "$LOG_FILE"
    else
        warn "日志文件不存在: $LOG_FILE"
    fi
}

# ── 主流程 ──
main() {
    local action="${1:-all}"
    local port="${2:-$DEFAULT_PORT}"

    echo -e "${CYAN}"
    echo "  PinHaoClaw"
    echo -e "${NC}"
    echo ""

    case "$action" in
        frontend|fe)
            check_deps
            build_frontend
            ;;
        backend|be)
            check_deps
            build_backend
            ;;
        build)
            check_deps
            build_frontend
            build_backend
            ok "构建完成！运行 ./start.sh start 启动服务"
            ;;
        start)
            start_daemon "$port"
            ;;
        stop)
            do_stop
            ;;
        restart)
            do_restart "$port"
            ;;
        status)
            do_status "$port"
            ;;
        log|logs)
            do_log
            ;;
        run)
            if [ ! -f "pinhaoclaw" ]; then
                err "未找到 pinhaoclaw 二进制文件，请先运行 ./start.sh build"
                exit 1
            fi
            prepare_env
            start_foreground "$port"
            ;;
        all)
            check_deps
            build_frontend
            build_backend
            start_daemon "$port"
            ;;
        clean)
            info "清理构建产物..."
            rm -f pinhaoclaw
            rm -rf pinhaoclaw-frontend/dist
            rm -f "$PID_FILE"
            ok "清理完成"
            ;;
        *)
            echo "用法: $0 {start|stop|restart|status|log|build|run|all|frontend|backend|clean} [port]"
            echo ""
            echo "  start     后台启动（需先 build）"
            echo "  stop      停止服务"
            echo "  restart   重启服务"
            echo "  status    查看运行状态"
            echo "  log       查看最近日志"
            echo "  build     构建前端 + 后端（不启动）"
            echo "  run       前台启动（需先 build，Ctrl+C 停止）"
            echo "  all       构建 + 后台启动（默认）"
            echo "  frontend  仅构建前端"
            echo "  backend   仅编译后端"
            echo "  clean     清理构建产物"
            echo ""
            echo "示例:"
            echo "  $0              # 构建 + 后台启动"
            echo "  $0 start        # 后台启动，默认端口 $DEFAULT_PORT"
            echo "  $0 start 8080   # 后台启动，端口 8080"
            echo "  $0 stop         # 停止"
            echo "  $0 restart      # 重启"
            echo "  $0 status       # 查看状态"
            echo "  $0 log          # 查看日志"
            exit 1
            ;;
    esac
}

main "$@"
