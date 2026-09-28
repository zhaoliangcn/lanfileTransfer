/*
 * lanfileTransfer-c - 命令行版本
 * 兼容 linfileTransfer-go 协议
 * 
 * 功能：
 * - 自动接收文件
 * - 在线状态同步
 * - 交互式命令行
 * - 后台守护进程模式
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <getopt.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <time.h>
#include <pthread.h>
#include <errno.h>
#include <fcntl.h>
#include <dirent.h>
#include <libgen.h>

#include "discovery.h"
#include "protocol.h"
#include "transfer.h"
#include "globals.h"

/* 版本信息 */
#define VERSION "1.0.0"
#define APP_NAME "lanfileTransfer-c"

/* 配置结构 */
typedef struct {
    char device_id[64];
    char device_name[128];
    int discovery_port;
    int transfer_port;
    char save_path[512];
    char log_file[512];
    int auto_receive;
    int daemon_mode;
    int verbose;
    int max_concurrent;
    int chunk_size;
    int discovery_interval;
    char auth_token[65];
} Config;

/* 传递给远程命令执行线程的参数 */
typedef struct {
    char command[32];
    char peer_name[128];
    char peer_id[64];
} SystemCommandArgs;

/* 全局变量 */
volatile int g_running = 1;
DiscoveryManager g_discovery;
TransferManager g_transfer;
static Config g_config;

/* 函数声明 */
static void print_usage(const char *prog);
static void print_version(void);
static int parse_args(int argc, char *argv[]);
static void signal_handler(int sig);
static void install_signal_handlers(void);
static void on_peer_found(Peer *peer);
static void on_peer_lost(const char *peer_id);
static void on_transfer_event(TransferTask *task, const char *event_type);
static void on_control_msg(const char *type, ControlMessage *msg);
static int create_directory(const char *path);
static int run_cli(void);
static int daemonize(void);
static void *run_system_command(void *arg);
static int load_or_create_auth_token(void);

/* 打印使用帮助 */
static void print_usage(const char *prog) {
    printf("Usage: %s [OPTIONS]\n", prog);
    printf("\n");
    printf("Options:\n");
    printf("  -n, --name NAME        设备名称 (默认: hostname)\n");
    printf("  -d, --device-id ID     设备ID (默认: 自动生成)\n");
    printf("  -p, --port PORT        传输端口 (默认: 9876)\n");
    printf("  -D, --discovery-port PORT  发现端口 (默认: 9876)\n");
    printf("  -s, --save-dir DIR     保存目录 (默认: ./downloads)\n");
    printf("  -l, --log-file FILE    日志文件路径 (默认: ./lanfileTransfer-c.log)\n");
    printf("  -a, --auto-receive     自动接收文件\n");
    printf("  -c, --concurrent NUM   最大并发传输数 (默认: 5)\n");
    printf("  -b, --chunk-size KB    分块大小KB (默认: 64)\n");
    printf("  -i, --interval SEC     发现间隔秒 (默认: 5)\n");
    printf("  -f, --foreground       前台模式 (默认: 后台)\n");
    printf("  -v, --verbose          详细输出\n");
    printf("  -h, --help             显示帮助\n");
    printf("  -V, --version          显示版本\n");
    printf("\n");
    printf("Examples:\n");
    printf("  %s -n \"MyPC\" -a -s ~/Downloads\n", prog);
    printf("  %s --auto-receive --verbose\n", prog);
}

/* 打印版本信息 */
static void print_version(void) {
    printf("%s version %s\n", APP_NAME, VERSION);
    printf("Compatible with linfileTransfer-go protocol\n");
}

/* 解析命令行参数 */
static int parse_args(int argc, char *argv[]) {
    static struct option long_options[] = {
        {"name",           required_argument, 0, 'n'},
        {"device-id",      required_argument, 0, 'd'},
        {"port",           required_argument, 0, 'p'},
        {"discovery-port", required_argument, 0, 'D'},
        {"save-dir",       required_argument, 0, 's'},
        {"log-file",       required_argument, 0, 'l'},
        {"auto-receive",   no_argument,       0, 'a'},
        {"concurrent",     required_argument, 0, 'c'},
        {"chunk-size",     required_argument, 0, 'b'},
        {"interval",       required_argument, 0, 'i'},
        {"foreground",     no_argument,       0, 'f'},
        {"verbose",        no_argument,       0, 'v'},
        {"help",           no_argument,       0, 'h'},
        {"version",        no_argument,       0, 'V'},
        {0, 0, 0, 0}
    };

    /* 默认配置 */
    memset(&g_config, 0, sizeof(g_config));
    gethostname(g_config.device_name, sizeof(g_config.device_name));
    g_config.discovery_port = DEFAULT_PORT;
    g_config.transfer_port = DEFAULT_PORT;
    strcpy(g_config.save_path, "./downloads");
    strcpy(g_config.log_file, "./lanfileTransfer-c.log");
    g_config.auto_receive = 0;
    g_config.daemon_mode = 1;
    g_config.verbose = 0;
    g_config.max_concurrent = 5;
    g_config.chunk_size = 64 * 1024;
    g_config.discovery_interval = 5;

    int opt;
    int option_index = 0;

    while ((opt = getopt_long(argc, argv, "n:d:p:D:s:l:ac:b:i:fvVh", long_options, &option_index)) != -1) {
        switch (opt) {
            case 'n':
                strncpy(g_config.device_name, optarg, sizeof(g_config.device_name) - 1);
                break;
            case 'd':
                strncpy(g_config.device_id, optarg, sizeof(g_config.device_id) - 1);
                break;
            case 'p':
                g_config.transfer_port = atoi(optarg);
                break;
            case 'D':
                g_config.discovery_port = atoi(optarg);
                break;
            case 's':
                strncpy(g_config.save_path, optarg, sizeof(g_config.save_path) - 1);
                break;
            case 'l':
                strncpy(g_config.log_file, optarg, sizeof(g_config.log_file) - 1);
                break;
            case 'a':
                g_config.auto_receive = 1;
                break;
            case 'c':
                g_config.max_concurrent = atoi(optarg);
                break;
            case 'b':
                g_config.chunk_size = atoi(optarg) * 1024;
                break;
            case 'i':
                g_config.discovery_interval = atoi(optarg);
                break;
            case 'f':
                g_config.daemon_mode = 0;
                break;
            case 'v':
                g_config.verbose = 1;
                break;
            case 'h':
                print_usage(argv[0]);
                exit(0);
            case 'V':
                print_version();
                exit(0);
            default:
                print_usage(argv[0]);
                exit(1);
        }
    }

    /* 生成设备ID如果未指定 */
    if (g_config.device_id[0] == '\0') {
        snprintf(g_config.device_id, sizeof(g_config.device_id),
                 "%s-%ld", g_config.device_name, time(NULL));
    }

    if (load_or_create_auth_token() != 0) {
        fprintf(stderr, "[SECURITY] 无法初始化认证令牌，远程关机/重启功能将被禁用\n");
    }

    return 0;
}

/* =========================================================================
 * 共享认证令牌
 *
 * 远程关机/重启是一台机器上破坏性最高的操作。device_id 是
 * "<hostname>-<unixtime>"，完全可以猜出来，所以仅凭 target_id 校验等于没有
 * 校验。这里维护一个本地持久化的随机令牌，两端配对后互相持有。
 * ========================================================================= */
static int load_or_create_auth_token(void) {
    const char *dir = getenv("HOME");
    if (!dir || !dir[0]) dir = "/tmp";

    char path[512];
    snprintf(path, sizeof(path), "%s/.lanfiletransfer.token", dir);

    FILE *f = fopen(path, "r");
    if (f) {
        char buf[128];
        if (fgets(buf, sizeof(buf), f)) {
            size_t len = strcspn(buf, "\r\n");
            buf[len] = '\0';
            fclose(f);
            if (len >= 16) {
                strncpy(g_config.auth_token, buf, sizeof(g_config.auth_token) - 1);
                g_config.auth_token[sizeof(g_config.auth_token) - 1] = '\0';
                return 0;
            }
        } else {
            fclose(f);
        }
    }

    /* 生成 32 字节随机令牌 */
    unsigned char raw[32];
    FILE *urandom = fopen("/dev/urandom", "rb");
    if (!urandom) {
        fprintf(stderr, "[SECURITY] 无法打开 /dev/urandom\n");
        return -1;
    }
    size_t got = fread(raw, 1, sizeof(raw), urandom);
    fclose(urandom);
    if (got != sizeof(raw)) {
        fprintf(stderr, "[SECURITY] 读取随机数失败\n");
        return -1;
    }

    static const char hex[] = "0123456789abcdef";
    for (size_t i = 0; i < sizeof(raw); i++) {
        g_config.auth_token[i * 2]     = hex[raw[i] >> 4];
        g_config.auth_token[i * 2 + 1] = hex[raw[i] & 0x0F];
    }
    g_config.auth_token[64] = '\0';

    f = fopen(path, "w");
    if (f) {
        fprintf(f, "%s\n", g_config.auth_token);
        fclose(f);
        chmod(path, 0600);
    }

    printf("[SECURITY] 已生成认证令牌: %s\n", path);
    return 0;
}

/* =========================================================================
 * 远程命令执行线程
 *
 * 单独起线程的原因：on_control_msg 运行在 UDP 接收线程上，原实现在这里
 * sleep(10)，会把唯一的心跳/发现线程堵住 10 秒，期间无法发现任何设备。
 *
 * 用 fork+execv 而不是 system()，避免经过 shell 解析。
 * ========================================================================= */
static void *run_system_command(void *arg) {
    SystemCommandArgs *args = (SystemCommandArgs *)arg;
    int is_restart = (strcmp(args->command, CONTROL_COMMAND_RESTART) == 0);

    printf("\n");
    printf("========================================\n");
    printf("!! 已认证的远程控制命令 !!\n");
    printf("   发送者: %s (%s)\n", args->peer_name, args->peer_id);
    printf("   命令:   %s\n", is_restart ? "重启" : "关机");
    printf("========================================\n");
    fflush(stdout);

    /* 倒计时，期间可按 Ctrl+C 取消 */
    for (int i = 10; i > 0 && g_running; i--) {
        printf("系统将在 %d 秒后%s... (Ctrl+C 取消)\n", i, is_restart ? "重启" : "关机");
        fflush(stdout);
        for (int s = 0; s < 10 && g_running; s++) {
            usleep(100000);
        }
    }

    if (!g_running) {
        printf("已取消\n");
        free(args);
        return NULL;
    }

    pid_t pid = fork();
    if (pid == 0) {
#ifdef __APPLE__
        if (is_restart) {
            execlp("osascript", "osascript", "-e",
                   "tell app \"System Events\" to restart", (char *)NULL);
        } else {
            execlp("osascript", "osascript", "-e",
                   "tell app \"System Events\" to shut down", (char *)NULL);
        }
#else
        if (is_restart) {
            execlp("shutdown", "shutdown", "-r", "+0", (char *)NULL);
        } else {
            execlp("shutdown", "shutdown", "-h", "+0", (char *)NULL);
        }
#endif
        _exit(127);
    } else if (pid < 0) {
        perror("[SECURITY] fork");
    } else {
        printf("已执行 %s (pid %d)\n", is_restart ? "重启" : "关机", (int)pid);
    }

    free(args);
    return NULL;
}

/* 信号处理 */
static void signal_handler(int sig) {
    (void)sig;
    g_running = 0;
}

/* 注册信号处理函数
 *
 * 关键点：sa_flags 不含 SA_RESTART。glibc 的 signal() 默认会带上 SA_RESTART，
 * 导致阻塞中的 fgets(stdin) 在收到 SIGTERM 后被内核自动重启，守护进程永远
 * 卡在等待输入上，systemd 只能等 TimeoutStopSec 超时后 SIGKILL。 */
static void install_signal_handlers(void) {
    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0; /* 明确不设 SA_RESTART，让阻塞读返回 EINTR */

    sigaction(SIGINT, &sa, NULL);
    sigaction(SIGTERM, &sa, NULL);

    /* 写入已关闭的管道时不要让进程直接终止，否则会留下半个已落盘的文件 */
    signal(SIGPIPE, SIG_IGN);
}

/* 创建目录 */
static int create_directory(const char *path) {
    char tmp[512];
    char *p = NULL;
    size_t len;

    snprintf(tmp, sizeof(tmp), "%s", path);
    len = strlen(tmp);
    if (tmp[len - 1] == '/')
        tmp[len - 1] = '\0';

    for (p = tmp + 1; *p; p++) {
        if (*p == '/') {
            *p = 0;
            mkdir(tmp, 0755);
            *p = '/';
        }
    }
    return mkdir(tmp, 0755);
}

/* 回调：发现新对等节点 */
static void on_peer_found(Peer *peer) {
    if (g_config.verbose) {
        printf("[DISCOVERY] 发现设备: %s (%s:%d)\n", 
               peer->name, peer->ip, peer->port);
    }
}

/* 回调：丢失对等节点 */
static void on_peer_lost(const char *peer_id) {
    if (g_config.verbose) {
        printf("[DISCOVERY] 设备离线: %s\n", peer_id);
    }
}

/* 回调：传输事件 */
static void on_transfer_event(TransferTask *task, const char *event_type) {
    if (strcmp(event_type, "start") == 0) {
        printf("[TRANSFER] 开始传输: %s (%s) -> %s\n",
               task->file_name, task->peer_name, task->status == TRANSFER_TRANSFERRING ? "transferring" : "unknown");
    } else if (strcmp(event_type, "progress") == 0) {
        if (g_config.verbose) {
            printf("[TRANSFER] 进度: %s %.1f%% (%ld/%ld bytes)\n",
                   task->file_name, task->progress, 
                   (long)task->bytes_transferred, (long)task->file_size);
        }
    } else if (strcmp(event_type, "complete") == 0) {
        printf("[TRANSFER] ✓ 传输完成: %s (%ld bytes)\n",
               task->file_name, (long)task->file_size);
    } else if (strcmp(event_type, "error") == 0) {
        printf("[TRANSFER] ✗ 传输失败: %s - %s\n",
               task->file_name, task->error);
    } else if (strcmp(event_type, "cancel") == 0) {
        printf("[TRANSFER] 传输取消: %s\n", task->file_name);
    }
}

/* 回调：控制消息 */
static void on_control_msg(const char *type, ControlMessage *msg) {
    if (g_config.verbose) {
        printf("[CONTROL] 收到消息类型: %s\n", type);
    }
    
    if (strcmp(type, CONTROL_TYPE_TRANSFER_REQUEST) == 0) {
        if (g_config.auto_receive) {
            printf("[AUTO-RECEIVE] 自动接收: %s (%ld bytes) 来自 %s\n",
                   msg->file_name, (long)msg->file_size, msg->device_id);
            
            /* 自动接受传输请求 */
            char task_id[64];
            Peer *peer = discovery_get_peer(&g_discovery, msg->device_id);
            if (peer) {
                transfer_accept_request(&g_transfer,
                                       msg->device_id,
                                       msg->device_name,
                                       peer->ip,
                                       msg->transfer_id,
                                       msg->file_name,
                                       msg->file_size,
                                       task_id);
            }
        }
    } else if (strcmp(type, CONTROL_TYPE_SYSTEM_COMMAND) == 0) {
        printf("[DEBUG] system_command: from='%s' command='%s' target_id='%s'\n",
               msg->device_id, msg->command, msg->target_id);

        /* The command must name this device explicitly. Accepting an empty
         * target_id made the message a broadcast that shut down every peer. */
        if (msg->target_id[0] == '\0') {
            printf("[SECURITY] 拒绝: 缺少 target_id\n");
            return;
        }
        if (strcmp(msg->target_id, g_config.device_id) != 0) {
            printf("[SECURITY] 拒绝: target_id 不匹配 (\"%s\" != \"%s\")\n",
                   msg->target_id, g_config.device_id);
            return;
        }

        /* Shared secret check. Without it any host that can reach this UDP port
         * could power the machine off. */
        if (g_config.auth_token[0] == '\0' || msg->auth_token[0] == '\0' ||
            strcmp(msg->auth_token, g_config.auth_token) != 0) {
            printf("[SECURITY] 拒绝: 认证令牌无效 (来自 %s)\n", msg->device_id);
            return;
        }

        if (strcmp(msg->command, CONTROL_COMMAND_SHUTDOWN) != 0 &&
            strcmp(msg->command, CONTROL_COMMAND_RESTART) != 0) {
            printf("[SECURITY] 拒绝: 未知命令 '%s'\n", msg->command);
            return;
        }

        /* Hand off to a detached thread: this callback runs on the UDP receive
         * thread, and the grace period used to block all peer discovery. */
        SystemCommandArgs *args = malloc(sizeof(SystemCommandArgs));
        if (!args) return;
        strncpy(args->command,
                (strcmp(msg->command, CONTROL_COMMAND_RESTART) == 0)
                    ? CONTROL_COMMAND_RESTART : CONTROL_COMMAND_SHUTDOWN,
                sizeof(args->command) - 1);
        args->command[sizeof(args->command) - 1] = '\0';
        strncpy(args->peer_name, msg->device_name, sizeof(args->peer_name) - 1);
        args->peer_name[sizeof(args->peer_name) - 1] = '\0';
        strncpy(args->peer_id, msg->device_id, sizeof(args->peer_id) - 1);
        args->peer_id[sizeof(args->peer_id) - 1] = '\0';

        pthread_t thread;
        if (pthread_create(&thread, NULL, run_system_command, args) == 0) {
            pthread_detach(thread);
        } else {
            fprintf(stderr, "[SECURITY] 无法创建命令线程\n");
            free(args);
        }
    }
}

/* 将相对路径转为绝对路径（在 chdir("/") 前调用） */
static void resolve_path(char *path, size_t path_size) {
    if (!path || !path[0] || path[0] == '/') return;
    char cwd[1024];
    char abs_path[1024];
    if (getcwd(cwd, sizeof(cwd))) {
        snprintf(abs_path, sizeof(abs_path), "%s/%s", cwd, path);
        /* strncpy does not terminate when the source fills the destination, which
         * would leave g_config.save_path / log_file unterminated for every later
         * use. snprintf always terminates. */
        snprintf(path, path_size, "%s", abs_path);
        /* 清理 ./ 冗余 */
        char *p;
        while ((p = strstr(path, "/./")) != NULL) {
            memmove(p, p + 2, strlen(p + 2) + 1);
        }
    }
}

/* 后台守护进程化 - 将 stdout/stderr 重定向到日志文件 */
static int daemonize(void) {
    pid_t pid, sid;

    /* 检查是否已经是守护进程 */
    if (getppid() == 1)
        return 0;

    /* 第一次fork */
    pid = fork();
    if (pid < 0)
        return -1;
    if (pid > 0)
        exit(0); /* 父进程退出 */

    /* 创建会话 */
    sid = setsid();
    if (sid < 0)
        return -1;

    /* 改变工作目录 */
    if (chdir("/") < 0)
        return -1;

    /* 关闭标准文件描述符 */
    close(0);

    /* 重定向标准输入到 /dev/null */
    int fd_null = open("/dev/null", O_RDWR);
    if (fd_null >= 0) {
        dup2(fd_null, 0);
        if (fd_null > 2)
            close(fd_null);
    }

    /* 重定向 stdout/stderr 到日志文件（使用 freopen 确保 stdio 状态正确） */
    if (g_config.log_file[0]) {
        if (freopen(g_config.log_file, "a", stdout) == NULL) {
            /* fallback: 写入 /dev/null */
            freopen("/dev/null", "a", stdout);
        }
        if (freopen(g_config.log_file, "a", stderr) == NULL) {
            freopen("/dev/null", "a", stderr);
        }
    } else {
        freopen("/dev/null", "a", stdout);
        freopen("/dev/null", "a", stderr);
    }

    /* 设置行缓冲，确保每行日志立即写入文件 */
    setvbuf(stdout, NULL, _IOLBF, 0);
    setvbuf(stderr, NULL, _IOLBF, 0);

    return 0;
}

/* 交互式命令行 */
static int run_cli(void) {
    char buffer[1024];
    char *cmd;
    char *args;
    int running = 1;

    printf("\n");
    printf("========================================\n");
    printf("  lanfileTransfer-c 命令行界面 v%s\n", VERSION);
    printf("========================================\n");
    printf("输入 'help' 显示帮助命令\n");
    printf("输入 'quit' 退出程序\n");
    printf("========================================\n");
    printf("\n");

    while (running && g_running) {
        printf("lanfileTransfer> ");
        fflush(stdout);

        if (fgets(buffer, sizeof(buffer), stdin) == NULL) {
            break;
        }

        /* 去除换行符 */
        buffer[strcspn(buffer, "\n")] = 0;

        /* 跳过空行 */
        if (strlen(buffer) == 0)
            continue;

        cmd = strtok(buffer, " ");
        if (cmd == NULL)
            continue;

        args = strtok(NULL, "");

        if (strcmp(cmd, "quit") == 0 || strcmp(cmd, "exit") == 0) {
            running = 0;
        } else if (strcmp(cmd, "help") == 0 || strcmp(cmd, "?") == 0) {
            printf("\n可用命令:\n");
            printf("  help              显示此帮助信息\n");
            printf("  quit/exit         退出程序\n");
            printf("  peers             显示在线设备列表\n");
            printf("  status            显示当前传输状态\n");
            printf("  tasks             显示所有传输任务\n");
            printf("  send <file> [peer] 发送文件到指定设备\n");
            printf("  msg <peer_id> <text> 发送聊天消息\n");
            printf("  shutdown <peer_id> 发送远程关机命令\n");
            printf("  restart <peer_id>  发送远程重启命令\n");
            printf("  cancel <task_id>  取消传输任务\n");
            printf("  pause <task_id>   暂停传输任务\n");
            printf("  resume <task_id>  恢复暂停的任务\n");
            printf("  refresh           刷新设备列表\n");
            printf("  config            显示当前配置\n");
            printf("  verbose           切换详细模式\n");
            printf("\n");
        } else if (strcmp(cmd, "peers") == 0) {
            Peer *peers;
            int count;
            if (discovery_get_peers(&g_discovery, &peers, &count) == 0) {
                printf("\n在线设备 (%d):\n", count);
                for (int i = 0; i < count; i++) {
                    printf("  [%d] %s - %s (%s:%d) %s\n",
                           i + 1,
                           peers[i].id,
                           peers[i].name,
                           peers[i].ip,
                           peers[i].port,
                           peers[i].online ? "[在线]" : "[离线]");
                }
                printf("\n");
            }
        } else if (strcmp(cmd, "status") == 0) {
            printf("\n系统状态:\n");
            printf("  设备名称: %s\n", g_config.device_name);
            printf("  设备ID: %s\n", g_config.device_id);
            printf("  监听端口: %d\n", g_config.transfer_port);
            printf("  自动接收: %s\n", g_config.auto_receive ? "启用" : "禁用");
            printf("  详细模式: %s\n", g_config.verbose ? "启用" : "禁用");
            printf("  运行状态: %s\n", g_running ? "运行中" : "已停止");
            printf("\n");
        } else if (strcmp(cmd, "tasks") == 0) {
            TransferTask *tasks;
            int count;
            if (transfer_get_all_tasks(&g_transfer, &tasks, &count) == 0) {
                printf("\n传输任务 (%d):\n", count);
                for (int i = 0; i < count; i++) {
                    printf("  [%d] %s\n", i + 1, tasks[i].file_name);
                    printf("       状态: %s | 进度: %.1f%% | 速度: %.1f KB/s\n",
                           transfer_status_str(tasks[i].status),
                           tasks[i].progress,
                           tasks[i].speed);
                    printf("       对等节点: %s\n", tasks[i].peer_name);
                    if (tasks[i].error[0]) {
                        printf("       错误: %s\n", tasks[i].error);
                    }
                    printf("\n");
                }
            }
        } else if (strcmp(cmd, "send") == 0) {
            if (args == NULL) {
                printf("用法: send <file_path> [peer_id]\n");
            } else {
                char *file_path = strtok(args, " ");
                char *peer_id = strtok(NULL, " ");
                
                if (file_path == NULL) {
                    printf("错误: 请指定文件路径\n");
                } else {
                    printf("发送文件: %s\n", file_path);
                    if (peer_id) {
                        printf("目标设备: %s\n", peer_id);
                        Peer *peer = discovery_get_peer(&g_discovery, peer_id);
                        if (peer) {
                            char task_id[64];
                            int ret = transfer_send_file(&g_transfer,
                                                        peer_id,
                                                        peer->name,
                                                        peer->ip,
                                                        file_path,
                                                        task_id);
                            if (ret == 0) {
                                printf("✓ 传输任务已创建: %s\n", task_id);
                            } else {
                                printf("✗ 创建传输任务失败\n");
                            }
                        } else {
                            printf("✗ 未找到设备: %s\n", peer_id);
                        }
                    } else {
                        /* 广播发送 */
                        printf("广播发送文件到所有在线设备...\n");
                        Peer *peers;
                        int count;
                        if (discovery_get_peers(&g_discovery, &peers, &count) == 0) {
                            for (int i = 0; i < count; i++) {
                                if (peers[i].online) {
                                    char task_id[64];
                                    int ret = transfer_send_file(&g_transfer,
                                                                peers[i].id,
                                                                peers[i].name,
                                                                peers[i].ip,
                                                                file_path,
                                                                task_id);
                                    if (ret == 0) {
                                        printf("✓ 已发送到 %s: %s\n", peers[i].name, task_id);
                                    }
                                }
                            }
                        }
                    }
                }
            }
        } else if (strcmp(cmd, "cancel") == 0) {
            if (args == NULL) {
                printf("用法: cancel <task_id>\n");
            } else {
                int ret = transfer_cancel(&g_transfer, args);
                if (ret == 0) {
                    printf("✓ 已取消任务: %s\n", args);
                } else {
                    printf("✗ 取消任务失败\n");
                }
            }
        } else if (strcmp(cmd, "pause") == 0) {
            if (args == NULL) {
                printf("用法: pause <task_id>\n");
            } else {
                int ret = transfer_pause(&g_transfer, args);
                if (ret == 0) {
                    printf("✓ 已暂停任务: %s\n", args);
                } else {
                    printf("✗ 暂停任务失败\n");
                }
            }
        } else if (strcmp(cmd, "resume") == 0) {
            if (args == NULL) {
                printf("用法: resume <task_id>\n");
            } else {
                int ret = transfer_resume(&g_transfer, args);
                if (ret == 0) {
                    printf("✓ 已恢复任务: %s\n", args);
                } else {
                    printf("✗ 恢复任务失败\n");
                }
            }
        } else if (strcmp(cmd, "refresh") == 0) {
            printf("刷新设备列表...\n");
            discovery_refresh_peers(&g_discovery);
        } else if (strcmp(cmd, "config") == 0) {
            printf("\n当前配置:\n");
            printf("  设备名称: %s\n", g_config.device_name);
            printf("  设备ID: %s\n", g_config.device_id);
            printf("  发现端口: %d\n", g_config.discovery_port);
            printf("  传输端口: %d\n", g_config.transfer_port);
            printf("  保存目录: %s\n", g_config.save_path);
            printf("  自动接收: %s\n", g_config.auto_receive ? "启用" : "禁用");
            printf("  最大并发: %d\n", g_config.max_concurrent);
            printf("  分块大小: %d KB\n", g_config.chunk_size / 1024);
            printf("  发现间隔: %d 秒\n", g_config.discovery_interval);
            printf("  详细模式: %s\n", g_config.verbose ? "启用" : "禁用");
            printf("\n");
        } else if (strcmp(cmd, "verbose") == 0) {
            g_config.verbose = !g_config.verbose;
            printf("详细模式: %s\n", g_config.verbose ? "启用" : "禁用");
        } else if (strcmp(cmd, "shutdown") == 0) {
            if (args == NULL || !args[0]) {
                printf("用法: shutdown <peer_id>\n");
                printf("示例: shutdown %s\n", g_config.device_id);
            } else {
                Peer *peer = discovery_get_peer(&g_discovery, args);
                if (!peer) {
                    printf("✗ 未找到设备: %s\n", args);
                } else if (!peer->online) {
                    printf("✗ 设备不在线: %s\n", args);
                } else {
                    printf("发送远程关机命令到 %s (%s)...\n", peer->name, peer->ip);
                    int ret = discovery_send_system_command(&g_discovery, args, CONTROL_COMMAND_SHUTDOWN);
                    printf("%s 远程关机命令已发送\n", ret == 0 ? "✓" : "✗");
                }
            }
        } else if (strcmp(cmd, "restart") == 0) {
            if (args == NULL || !args[0]) {
                printf("用法: restart <peer_id>\n");
                printf("示例: restart %s\n", g_config.device_id);
            } else {
                Peer *peer = discovery_get_peer(&g_discovery, args);
                if (!peer) {
                    printf("✗ 未找到设备: %s\n", args);
                } else if (!peer->online) {
                    printf("✗ 设备不在线: %s\n", args);
                } else {
                    printf("发送远程重启命令到 %s (%s)...\n", peer->name, peer->ip);
                    int ret = discovery_send_system_command(&g_discovery, args, CONTROL_COMMAND_RESTART);
                    printf("%s 远程重启命令已发送\n", ret == 0 ? "✓" : "✗");
                }
            }
        } else if (strcmp(cmd, "msg") == 0) {
            if (args == NULL || !args[0]) {
                printf("用法: msg <peer_id> <message text>\n");
                printf("示例: msg %s 你好\n", g_config.device_id);
            } else {
                char *peer_id = strtok(args, " ");
                char *message = strtok(NULL, "");
                if (!peer_id || !message) {
                    printf("用法: msg <peer_id> <message text>\n");
                } else {
                    printf("发送消息到 %s: %s\n", peer_id, message);
                    int ret = discovery_send_message(&g_discovery, peer_id, message);
                    printf("%s 消息已发送\n", ret == 0 ? "✓" : "✗");
                }
            }
        } else {
            printf("未知命令: %s\n", cmd);
            printf("输入 'help' 显示帮助\n");
        }
    }

    return 0;
}

/* 主函数 */
int main(int argc, char *argv[]) {
    int ret = 0;

    /* 解析命令行参数 */
    if (parse_args(argc, argv) != 0) {
        return 1;
    }

    /* 设置信号处理
     *
     * 必须用 sigaction 且 sa_flags 不含 SA_RESTART：signal() 在 glibc 下默认
     * 带 SA_RESTART，SIGTERM 到达时阻塞中的 fgets(stdin) 会被内核自动重启，
     * g_running 虽然已置 0 但循环永远等不到输入返回，进程就此挂死。
     * systemd stop 只能等满 TimeoutStopSec 再 SIGKILL。
     *
     * 去掉 SA_RESTART 后，fgets 以 EINTR 返回 NULL，run_cli 的循环随即 break，
     * 进程正常退出。 */
    install_signal_handlers();

    /* 守护进程模式 - 先将相对路径转为绝对路径，再后台化 */
    if (g_config.daemon_mode) {
        /* 保存目录转绝对路径 */
        resolve_path(g_config.save_path, sizeof(g_config.save_path));
        /* 日志文件转绝对路径 */
        resolve_path(g_config.log_file, sizeof(g_config.log_file));
        printf("[INIT] 日志文件: %s\n", g_config.log_file);
        printf("[INIT] 保存目录: %s\n", g_config.save_path);
        printf("[INIT] 正在后台运行...\n");
        fflush(stdout);
        if (daemonize() != 0) {
            fprintf(stderr, "Failed to daemonize\n");
            return 1;
        }
    }

    /* 创建保存目录 */
    if (create_directory(g_config.save_path) != 0 && errno != EEXIST) {
        fprintf(stderr, "Failed to create save directory: %s\n", g_config.save_path);
        return 1;
    }

    if (g_config.verbose) {
        printf("[INIT] 设备名称: %s\n", g_config.device_name);
        printf("[INIT] 设备ID: %s\n", g_config.device_id);
        printf("[INIT] 监听端口: %d\n", g_config.transfer_port);
        printf("[INIT] 自动接收: %s\n", g_config.auto_receive ? "启用" : "禁用");
    }

    /* 初始化发现管理器 */
    if (discovery_init(&g_discovery,
                      g_config.device_id,
                      g_config.device_name,
                      g_config.discovery_port,
                      g_config.discovery_interval) != 0) {
        fprintf(stderr, "Failed to initialize discovery\n");
        return 1;
    }

    /* 设置发现回调 */
    discovery_set_on_peer_found(&g_discovery, on_peer_found);
    discovery_set_on_peer_lost(&g_discovery, on_peer_lost);
    discovery_set_on_control_msg(&g_discovery, on_control_msg);

    /* 初始化传输管理器 */
    if (transfer_init(&g_transfer,
                     g_config.transfer_port,
                     g_config.chunk_size,
                     g_config.max_concurrent,
                     g_config.save_path,
                     g_config.auto_receive) != 0) {
        fprintf(stderr, "Failed to initialize transfer\n");
        return 1;
    }

    /* 设置传输回调 */
    transfer_set_on_event(&g_transfer, on_transfer_event);

    /* 启动发现服务 */
    if (discovery_start(&g_discovery) != 0) {
        fprintf(stderr, "Failed to start discovery\n");
        return 1;
    }

    /* 启动传输服务器 */
    if (transfer_start(&g_transfer) != 0) {
        fprintf(stderr, "Failed to start transfer server\n");
        return 1;
    }

    printf("\n");
    printf("========================================\n");
    printf("  lanfileTransfer-c 已启动\n");
    printf("========================================\n");
    printf("设备: %s\n", g_config.device_name);
    printf("ID: %s\n", g_config.device_id);
    printf("端口: %d\n", g_config.transfer_port);
    printf("自动接收: %s\n", g_config.auto_receive ? "启用" : "禁用");
    printf("保存目录: %s\n", g_config.save_path);
    printf("\n");

    /* 运行交互式命令行 */
    if (!g_config.daemon_mode) {
        run_cli();
    } else {
        /* 守护进程模式：等待信号 */
        while (g_running) {
            sleep(1);
        }
    }

    /* 清理资源 */
    printf("\n[SHUTDOWN] 正在停止服务...\n");
    transfer_stop(&g_transfer);
    discovery_stop(&g_discovery);

    printf("[SHUTDOWN] 已停止\n");

    return ret;
}