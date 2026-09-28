#!/bin/sh
SERVICE_NAME="lanfileTransfer"
BIN_PATH="/usr/local/bin/lanfileTransfer-c"
SAVE_DIR="/var/downloads"

# 检查是否为 root
if [ "$(id -u)" != "0" ]; then
    echo "请使用 sudo 或 root 运行"
    exit 1
fi

# 安装二进制文件
install -m 755 bin/lanfileTransfer-c "$BIN_PATH"

# 创建保存目录
mkdir -p "$SAVE_DIR"

# 检测 init 系统
if command -v systemctl >/dev/null 2>&1; then
    echo "检测到 systemd，安装 service..."
    
    cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=lanfileTransfer-c Service
After=network.target

[Service]
Type=simple
ExecStart=${BIN_PATH} -a -s ${SAVE_DIR}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}.service
    systemctl start ${SERVICE_NAME}.service
    echo "✓ 已安装并启动: systemctl status ${SERVICE_NAME}"

elif [ -f /etc/rc.local ]; then
    echo "检测到 rc.local，添加自启..."
    echo "${BIN_PATH} -a -s ${SAVE_DIR} &" >> /etc/rc.local
    echo "✓ 已添加到 /etc/rc.local"

elif command -v update-rc.d >/dev/null 2>&1; then
    echo "检测到 sysvinit，安装 init 脚本..."
    # 简单的 init.d 脚本
    cat > /etc/init.d/${SERVICE_NAME} << EOF
#!/bin/sh
### BEGIN INIT INFO
# Provides:          ${SERVICE_NAME}
# Required-Start:    \$network
# Required-Stop:     \$network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Description:       lanfileTransfer-c service
### END INIT INFO

BIN=${BIN_PATH}
PIDFILE=/var/run/${SERVICE_NAME}.pid

case "\$1" in
    start)
        echo "Starting ${SERVICE_NAME}..."
        start-stop-daemon --start --background --make-pidfile --pidfile \$PIDFILE --exec \$BIN -- -a -s ${SAVE_DIR}
        ;;
    stop)
        echo "Stopping ${SERVICE_NAME}..."
        start-stop-daemon --stop --pidfile \$PIDFILE
        rm -f \$PIDFILE
        ;;
    restart)
        \$0 stop; sleep 1; \$0 start
        ;;
    status)
        if [ -f \$PIDFILE ] && kill -0 \$(cat \$PIDFILE) 2>/dev/null; then
            echo "Running"
        else
            echo "Stopped"
        fi
        ;;
esac
EOF
    chmod +x /etc/init.d/${SERVICE_NAME}
    update-rc.d ${SERVICE_NAME} defaults
    /etc/init.d/${SERVICE_NAME} start
    echo "✓ 已安装并启动"
fi

echo "完成！"
