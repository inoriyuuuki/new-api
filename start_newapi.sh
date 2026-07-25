#!/bin/bash
# New API - CentOS/OpenCloudOS 快速启动脚本（无 root 版本）
# 用法: bash start_newapi.sh
# 部署到用户目录，监听端口 9300

set -eo pipefail

# ========== 配置 ==========
APP_NAME="new-api"
PORT="9300"
INSTALL_DIR="${HOME}/${APP_NAME}"
DATA_DIR="${HOME}/${APP_NAME}_data"
GIT_REPO="https://github.com/inoriyuuuki/new-api.git"
BRANCH="main"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ========== Step 1: 检测系统依赖 ==========
log_info "Step 1/7: 检测系统依赖..."
for cmd in git wget curl tar make gcc; do
    if ! command -v $cmd &>/dev/null; then
        log_error "缺少 $cmd，请先安装: sudo yum install -y $cmd"
        exit 1
    fi
done
log_info "所有基础依赖已就绪"

# ========== Step 2: 安装 Node.js（Rspack 需要 Node >= 20.19） ==========
log_info "Step 2/7: 安装 Node.js..."
# 检测 Node.js 版本
NODE_OK=0
if command -v node &>/dev/null; then
    NODE_MAJOR=$(node --version | sed 's/v//' | cut -d. -f1)
    NODE_MINOR=$(node --version | sed 's/v//' | cut -d. -f2)
    if [ "$NODE_MAJOR" -gt 20 ] || { [ "$NODE_MAJOR" -eq 20 ] && [ "$NODE_MINOR" -ge 19 ]; }; then
        log_info "Node.js $(node --version) 满足要求"
        NODE_OK=1
    elif [ "$NODE_MAJOR" -ge 22 ]; then
        log_info "Node.js $(node --version) 满足要求"
        NODE_OK=1
    fi
fi

if [ "$NODE_OK" -eq 0 ]; then
    log_info "安装 Node.js 20 LTS（预编译二进制）..."
    NODE_VERSION="20.19.0"
    NODE_TAR="node-v${NODE_VERSION}-linux-x64.tar.xz"
    wget -q "https://nodejs.org/dist/v${NODE_VERSION}/${NODE_TAR}" -O "/tmp/${NODE_TAR}"
    tar -xf "/tmp/${NODE_TAR}" -C "${HOME}"
    export PATH="${HOME}/node-v${NODE_VERSION}-linux-x64/bin:${PATH}"
    if ! grep -q "node-v${NODE_VERSION}" "${HOME}/.bashrc" 2>/dev/null; then
        echo "export PATH=\$HOME/node-v${NODE_VERSION}-linux-x64/bin:\$PATH" >> "${HOME}/.bashrc"
    fi
    log_info "Node.js $(node --version) 安装完成"
fi

# ========== Step 3: 安装 Go（用户目录） ==========
log_info "Step 3/7: 安装 Go..."
export PATH="${HOME}/go/bin:/usr/local/go/bin:${PATH}"
if command -v go &>/dev/null; then
    GO_VER=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+')
    MAJOR=$(echo "$GO_VER" | cut -d. -f1)
    MINOR=$(echo "$GO_VER" | cut -d. -f2)
    if [ "$MAJOR" -ge 1 ] && [ "$MINOR" -ge 25 ]; then
        log_info "Go $GO_VER 满足要求，跳过安装"
    else
        log_warn "Go 版本过旧 ($GO_VER)，安装最新版..."
        GO_VERSION="1.26.1"
        GO_TAR="go${GO_VERSION}.linux-amd64.tar.gz"
        wget -q "https://go.dev/dl/${GO_TAR}" -O "/tmp/${GO_TAR}"
        rm -rf "${HOME}/go"
        tar -C "${HOME}" -xzf "/tmp/${GO_TAR}"
        export PATH="${HOME}/go/bin:${PATH}"
        if ! grep -q "go/bin" "${HOME}/.bashrc" 2>/dev/null; then
            echo 'export PATH=$HOME/go/bin:$PATH' >> "${HOME}/.bashrc"
        fi
        log_info "Go ${GO_VERSION} 安装完成"
    fi
else
    GO_VERSION="1.26.1"
    GO_TAR="go${GO_VERSION}.linux-amd64.tar.gz"
    log_info "正在下载 Go ${GO_VERSION}..."
    wget -q "https://go.dev/dl/${GO_TAR}" -O "/tmp/${GO_TAR}"
    rm -rf "${HOME}/go"
    tar -C "${HOME}" -xzf "/tmp/${GO_TAR}"
    export PATH="${HOME}/go/bin:${PATH}"
    if ! grep -q "go/bin" "${HOME}/.bashrc" 2>/dev/null; then
        echo 'export PATH=$HOME/go/bin:$PATH' >> "${HOME}/.bashrc"
    fi
    log_info "Go ${GO_VERSION} 安装完成"
fi

go version || { log_error "Go 未正确安装"; exit 1; }

# ========== Step 4: 安装 Bun（用户目录） ==========
log_info "Step 4/7: 安装 Bun..."
export BUN_INSTALL="${HOME}/.bun"
export PATH="${BUN_INSTALL}/bin:${PATH}"
if command -v bun &>/dev/null; then
    log_info "检测到 Bun $(bun --version)，跳过安装"
else
    curl -fsSL https://bun.sh/install | bash
    export PATH="${BUN_INSTALL}/bin:${PATH}"
    if ! grep -q ".bun/bin" "${HOME}/.bashrc" 2>/dev/null; then
        echo 'export PATH=$HOME/.bun/bin:$PATH' >> "${HOME}/.bashrc"
    fi
    log_info "Bun $(bun --version) 安装完成"
fi

# ========== Step 5: 克隆/更新代码 ==========
log_info "Step 5/7: 获取项目代码..."
if [ -d "${INSTALL_DIR}/.git" ]; then
    log_info "目录已存在，执行 git pull 更新..."
    cd "${INSTALL_DIR}"
    git stash 2>/dev/null || true
    git pull origin "${BRANCH}"
else
    log_info "克隆项目到 ${INSTALL_DIR}..."
    git clone -b "${BRANCH}" "${GIT_REPO}" "${INSTALL_DIR}"
    cd "${INSTALL_DIR}"
fi

# ========== Step 6: 构建项目 ==========
log_info "Step 6/7: 构建项目..."

# VERSION 文件
if [ ! -f VERSION ]; then
    echo "$(date +%Y%m%d).1" > VERSION
fi

# 构建前端
log_info "  构建前端（使用 Bun + Node.js $(node --version)）..."
cd "${INSTALL_DIR}/web"
bun install --frozen-lockfile 2>&1 | tail -3
DISABLE_ESLINT_PLUGIN='true' bun run build
log_info "  前端构建完成 ✓"

# 构建后端
log_info "  构建后端..."
cd "${INSTALL_DIR}"
CGO_ENABLED=0 go build \
    -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" \
    -o "${APP_NAME}" \
    .
log_info "  后端构建完成 ✓ (${INSTALL_DIR}/${APP_NAME})"

# ========== Step 7: 启动服务 ==========
log_info "Step 7/7: 启动服务..."

mkdir -p "${DATA_DIR}"
cd "${DATA_DIR}"

# 创建 .env 配置
if [ ! -f .env ]; then
    UUID=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "$(date +%s)-$$")
    cat > .env <<- EOF
PORT=${PORT}
SESSION_SECRET=${UUID}
EOF
    log_info "已生成默认 .env 配置文件"
fi

# 停止旧实例
PID_FILE="${DATA_DIR}/new-api.pid"
if [ -f "${PID_FILE}" ]; then
    OLD_PID=$(cat "${PID_FILE}")
    if kill -0 "${OLD_PID}" 2>/dev/null; then
        log_info "停止旧实例 (PID: ${OLD_PID})..."
        kill "${OLD_PID}" 2>/dev/null || true
        sleep 2
    fi
fi

# 启动服务
cd "${DATA_DIR}"
nohup "${INSTALL_DIR}/${APP_NAME}" --port "${PORT}" > "${DATA_DIR}/app.log" 2>&1 &
NEW_PID=$!
echo "${NEW_PID}" > "${PID_FILE}"

sleep 3

if kill -0 "${NEW_PID}" 2>/dev/null; then
    log_info "=========================================="
    log_info "  New API 启动成功!"
    log_info "  监听端口: ${PORT}"
    log_info "  PID: ${NEW_PID}"
    IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")
    log_info "  管理地址: http://${IP}:${PORT}"
    log_info "  数据目录: ${DATA_DIR}"
    log_info "  查看日志: tail -f ${DATA_DIR}/app.log"
    log_info "  停止服务: kill \$(cat ${PID_FILE})"
    log_info "=========================================="
    tail -5 "${DATA_DIR}/app.log"
else
    log_error "服务启动失败，查看日志:"
    tail -20 "${DATA_DIR}/app.log"
    exit 1
fi
