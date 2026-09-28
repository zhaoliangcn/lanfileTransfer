/*
 * transfer.c - TCP 文件传输实现
 *
 * 功能：
 * - TCP 服务器监听传入连接
 * - 分块发送文件
 * - 接收文件并写入磁盘
 * - 传输控制（暂停/恢复/取消）
 * - 断点续传支持
 */

#include "transfer.h"
#include "protocol.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <errno.h>
#include <fcntl.h>
#include <dirent.h>
#include <libgen.h>
#include <sys/time.h>
#include <poll.h>
#include <inttypes.h>

/* accept() 轮询间隔：决定 transfer_stop() 的关停延迟上限 */
#define ACCEPT_POLL_MS 500

/* TCP 服务器线程 */
static void* tcp_server_loop(void *arg);

/* 处理传入连接 */
static void handle_incoming_connection(TransferManager *mgr, int client_socket, struct sockaddr_in *client_addr);

/* 参数结构：传给接收线程 */
typedef struct {
    TransferManager *mgr;
    int client_socket;
    struct sockaddr_in client_addr;
} ReceiveArg;

/* 接收文件线程 */
static void* receive_file_thread(void *arg);

/* 发送文件线程 */
static void* send_file_thread(void *arg);

/* 发送/接收所有数据 */
static int send_all(int socket, const unsigned char *data, size_t size);
static int recv_all(int socket, unsigned char *data, size_t size);

/* ====================================================================== */
/* 初始化传输管理器 */
/* ====================================================================== */
int transfer_init(TransferManager *mgr,
                 int listen_port,
                 int chunk_size,
                 int max_concurrent,
                 const char *save_path,
                 bool auto_receive) {
    memset(mgr, 0, sizeof(TransferManager));

    /* 抑制 SIGPIPE，防止 broken pipe 时进程被杀死 */
    signal(SIGPIPE, SIG_IGN);

    mgr->listen_port = listen_port > 0 ? listen_port : DEFAULT_PORT;
    mgr->chunk_size = chunk_size > 0 ? chunk_size : DEFAULT_CHUNK_SIZE;
    mgr->max_concurrent = max_concurrent > 0 ? max_concurrent : 5;
    mgr->auto_receive = auto_receive;

    if (save_path) {
        strncpy(mgr->default_save_path, save_path, sizeof(mgr->default_save_path) - 1);
    } else {
        strcpy(mgr->default_save_path, "./downloads");
    }

    mgr->task_count = 0;
    mgr->tcp_server_socket = -1;
    mgr->running = false;
    mgr->started = false;

    pthread_mutex_init(&mgr->mutex, NULL);

    return 0;
}

/* A slot can be reused once its task reached a terminal state. Without this the
 * array only ever grew, so a long-lived daemon refused every transfer past
 * MAX_TASKS. */
static bool task_is_terminal(const TransferTask *task) {
    return task->status == TRANSFER_COMPLETED ||
           task->status == TRANSFER_FAILED ||
           task->status == TRANSFER_CANCELLED;
}

/* Pick a slot for a transfer. Must be called with mgr->mutex held.
 *
 * Priority:
 *   1. a live task with the same id (resumed / retried transfer)
 *   2. a finished task whose slot can be recycled
 *   3. a fresh slot appended to the array
 *
 * Returns -1 when no slot is available. */
static int acquire_task_slot(TransferManager *mgr, const char *transfer_id) {
    if (transfer_id && transfer_id[0]) {
        for (int i = 0; i < mgr->task_count; i++) {
            if (task_is_terminal(&mgr->tasks[i])) {
                continue;
            }
            if (strcmp(mgr->tasks[i].id, transfer_id) == 0) {
                return i;
            }
        }
    }

    /* Recycle the oldest finished slot. */
    for (int i = 0; i < mgr->task_count; i++) {
        if (task_is_terminal(&mgr->tasks[i])) {
            memset(&mgr->tasks[i], 0, sizeof(TransferTask));
            mgr->tasks[i].tcp_socket = -1;
            return i;
        }
    }

    if (mgr->task_count < MAX_TASKS) {
        return mgr->task_count++;
    }

    return -1;
}

/* Trim finished tasks from the tail so the reported count stays meaningful. */
static void compact_finished_tail(TransferManager *mgr) {
    while (mgr->task_count > 0 && task_is_terminal(&mgr->tasks[mgr->task_count - 1])) {
        mgr->task_count--;
    }
}

/* ====================================================================== */
/* 启动 TCP 服务器 */
/* ====================================================================== */
int transfer_start(TransferManager *mgr) {
    if (mgr->started) {
    return 0;
}

    /* 创建 TCP socket */
    mgr->tcp_server_socket = socket(AF_INET, SOCK_STREAM, 0);
    if (mgr->tcp_server_socket < 0) {
        perror("[TRANSFER] socket");
        return -1;
    }

    /* 允许地址重用 */
    int reuse = 1;
    if (setsockopt(mgr->tcp_server_socket, SOL_SOCKET, SO_REUSEADDR, &reuse, sizeof(reuse)) < 0) {
        perror("[TRANSFER] setsockopt SO_REUSEADDR");
        close(mgr->tcp_server_socket);
        return -1;
    }

    /* 绑定地址 */
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    addr.sin_port = htons(mgr->listen_port);

    if (bind(mgr->tcp_server_socket, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        perror("[TRANSFER] bind");
        close(mgr->tcp_server_socket);
        return -1;
    }

    /* 监听 */
    if (listen(mgr->tcp_server_socket, 10) < 0) {
        perror("[TRANSFER] listen");
        close(mgr->tcp_server_socket);
        return -1;
    }

    mgr->running = true;
    mgr->started = true;

    /* 启动 TCP 服务器线程 */
    if (pthread_create(&mgr->tcp_server_thread, NULL, tcp_server_loop, mgr) != 0) {
        perror("[TRANSFER] pthread_create tcp_server");
        mgr->running = false;
        mgr->started = false;
        close(mgr->tcp_server_socket);
        return -1;
    }

    printf("[TRANSFER] TCP 服务器已启动，端口: %d\n", mgr->listen_port);
    return 0;
}

/* ====================================================================== */
/* 停止传输管理器 */
/* ====================================================================== */
void transfer_stop(TransferManager *mgr) {
    if (!mgr->started) {
        return;
    }

    mgr->running = false;

    /* 关闭所有活动任务 */
    pthread_mutex_lock(&mgr->mutex);
    for (int i = 0; i < mgr->task_count; i++) {
        mgr->tasks[i].running = false;
        if (mgr->tasks[i].tcp_socket >= 0) {
            close(mgr->tasks[i].tcp_socket);
            mgr->tasks[i].tcp_socket = -1;
        }
    }
    pthread_mutex_unlock(&mgr->mutex);

    /* 关闭服务器 socket 以唤醒 accept。
       仅 close() 不足以唤醒阻塞在 accept() 的线程，所以先 shutdown()；
       同时 tcp_server_loop 用 poll() 轮询 running，双保险。 */
    if (mgr->tcp_server_socket >= 0) {
        shutdown(mgr->tcp_server_socket, SHUT_RDWR);
        close(mgr->tcp_server_socket);
        mgr->tcp_server_socket = -1;
    }

    /* 等待服务器线程 */
    pthread_join(mgr->tcp_server_thread, NULL);

    mgr->started = false;
    pthread_mutex_destroy(&mgr->mutex);
}

/* ====================================================================== */
/* TCP 服务器线程 */
/* ====================================================================== */
static void* tcp_server_loop(void *arg) {
    TransferManager *mgr = (TransferManager *)arg;

    while (mgr->running) {
        struct sockaddr_in client_addr;
        socklen_t addr_len = sizeof(client_addr);

        /* poll() 而非裸 accept()：close() 不会唤醒另一个线程阻塞在 accept() 中
         * （Linux 行为），所以 transfer_stop() 里的 pthread_join 会永久阻塞，
         * systemctl stop 只能等满 TimeoutStopSec 再 SIGKILL。
         * 轮询让 running 的变化最多延迟 ACCEPT_POLL_MS 就能生效。 */
        struct pollfd pfd;
        pfd.fd = mgr->tcp_server_socket;
        pfd.events = POLLIN;
        pfd.revents = 0;

        int ready = poll(&pfd, 1, ACCEPT_POLL_MS);
        if (ready < 0) {
            if (errno == EINTR) continue;
            if (!mgr->running) break;
            continue;
        }
        if (ready == 0) continue;      /* 超时，回到循环顶部检查 running */
        if (!(pfd.revents & POLLIN)) continue;

        int client_socket = accept(mgr->tcp_server_socket,
                                   (struct sockaddr *)&client_addr, &addr_len);
        if (client_socket < 0) {
            if (!mgr->running) break;
            if (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK) continue;
            if (errno == EBADF || errno == EINVAL) break;  /* 已被关闭 */
            continue;
        }

        handle_incoming_connection(mgr, client_socket, &client_addr);
    }

    return NULL;
}

/* ====================================================================== */
/* 处理传入连接 - Go 兼容版：不进行自定义握手，直接接收传输包 */
/* ====================================================================== */
static void handle_incoming_connection(TransferManager *mgr,
                                       int client_socket,
                                       struct sockaddr_in *client_addr) {
    /* 创建接收参数 */
    ReceiveArg *arg = (ReceiveArg *)malloc(sizeof(ReceiveArg));
    if (!arg) {
        close(client_socket);
        return;
    }
    arg->mgr = mgr;
    arg->client_socket = client_socket;
    arg->client_addr = *client_addr;

    pthread_t thread;
    pthread_create(&thread, NULL, receive_file_thread, arg);
    pthread_detach(thread);
}

/* ====================================================================== */
/* 发送文件 */
/* ====================================================================== */
int transfer_send_file(TransferManager *mgr,
                      const char *peer_id,
                      const char *peer_name,
                      const char *peer_addr,
                      const char *file_path,
                      char *task_id) {
    if (!file_path || !peer_addr) {
        return -1;
    }

    /* 检查文件是否存在 */
    struct stat st;
    if (stat(file_path, &st) != 0) {
        fprintf(stderr, "[TRANSFER] 文件不存在: %s\n", file_path);
        return -1;
    }

    if (!S_ISREG(st.st_mode)) {
        fprintf(stderr, "[TRANSFER] 不是普通文件: %s\n", file_path);
        return -1;
    }

    pthread_mutex_lock(&mgr->mutex);

    int slot = acquire_task_slot(mgr, NULL);
    if (slot < 0) {
        pthread_mutex_unlock(&mgr->mutex);
        fprintf(stderr, "[TRANSFER] 任务队列已满\n");
        return -1;
    }

    TransferTask *task = &mgr->tasks[slot];
    memset(task, 0, sizeof(TransferTask));
    task->tcp_socket = -1;

    /* 生成传输 ID */
    snprintf(task->id, sizeof(task->id), "send-%s-%ld",
             basename((char *)file_path), time(NULL));

    if (task_id) {
        strncpy(task_id, task->id, MAX_TRANSFER_ID - 1);
    }

    strncpy(task->peer_id, peer_id ? peer_id : "", MAX_PEER_ID - 1);
    strncpy(task->peer_name, peer_name ? peer_name : "", MAX_PEER_NAME - 1);
    strncpy(task->peer_addr, peer_addr, MAX_PEER_ADDR - 1);
    strncpy(task->file_path, file_path, MAX_FILE_PATH - 1);
    strncpy(task->file_name, basename((char *)file_path), MAX_FILE_NAME - 1);

    task->file_size = st.st_size;
    task->bytes_transferred = 0;
    task->type = TRANSFER_TYPE_FILE;
    task->status = TRANSFER_PENDING;
    task->is_sender = true;
    task->start_time = time(NULL);
    task->end_time = 0;
    task->speed = 0.0;
    task->progress = 0.0;
    /* Must be true: the send loop is guarded by `while (task->running && ...)`,
     * so leaving it false skipped the entire loop body. */
    task->running = true;
    task->checkpoint = 0;

    pthread_mutex_unlock(&mgr->mutex);

    /* 创建发送线程 */
    pthread_create(&task->thread, NULL, send_file_thread, task);

    /* 触发事件回调 */
    if (mgr->on_transfer_event) {
        mgr->on_transfer_event(task, "start");
    }

    return 0;
}

/* ====================================================================== */
/* 发送文件夹（未实现，返回 -1） */
/* ====================================================================== */
int transfer_send_folder(TransferManager *mgr,
                        const char *peer_id,
                        const char *peer_name,
                        const char *peer_addr,
                        const char *folder_path,
                        char **task_ids,
                        int *task_count) {
    (void)mgr;
    (void)peer_id;
    (void)peer_name;
    (void)peer_addr;
    (void)folder_path;
    (void)task_ids;
    (void)task_count;
    fprintf(stderr, "[TRANSFER] 文件夹传输尚未实现\n");
    return -1;
}

/* ====================================================================== */
/* 接受传输请求 */
/* ====================================================================== */
int transfer_accept_request(TransferManager *mgr,
                           const char *peer_id,
                           const char *peer_name,
                           const char *peer_addr,
                           const char *transfer_id,
                           const char *file_name,
                           int64_t file_size,
                           char *task_id) {
    if (!transfer_id || !file_name) {
        return -1;
    }

    pthread_mutex_lock(&mgr->mutex);
    int slot = acquire_task_slot(mgr, NULL);
    if (slot < 0) {
        pthread_mutex_unlock(&mgr->mutex);
        return -1;
    }

    /* 检查是否已存在相同任务 */
    for (int i = 0; i < mgr->task_count; i++) {
        if (i == slot) continue;
        if (strcmp(mgr->tasks[i].id, transfer_id) == 0) {
            if (task_id) {
                strncpy(task_id, transfer_id, MAX_TRANSFER_ID - 1);
            }
            pthread_mutex_unlock(&mgr->mutex);
            return 0;
        }
    }

    TransferTask *task = &mgr->tasks[slot];
    memset(task, 0, sizeof(TransferTask));

    strncpy(task->id, transfer_id, sizeof(task->id) - 1);
    strncpy(task->peer_id, peer_id ? peer_id : "", MAX_PEER_ID - 1);
    strncpy(task->peer_name, peer_name ? peer_name : "", MAX_PEER_NAME - 1);
    strncpy(task->peer_addr, peer_addr ? peer_addr : "", MAX_PEER_ADDR - 1);
    strncpy(task->file_name, file_name, MAX_FILE_NAME - 1);
    snprintf(task->file_path, sizeof(task->file_path), "%s/%s",
             mgr->default_save_path, file_name);

    task->file_size = file_size;
    task->bytes_transferred = 0;
    task->type = TRANSFER_TYPE_FILE;
    task->status = TRANSFER_PENDING;
    task->is_sender = false;
    task->start_time = time(NULL);
    task->end_time = 0;
    task->speed = 0.0;
    task->progress = 0.0;
    task->tcp_socket = -1;
    task->running = false;
    task->checkpoint = 0;

    if (task_id) {
        strncpy(task_id, task->id, MAX_TRANSFER_ID - 1);
    }

    pthread_mutex_unlock(&mgr->mutex);

    if (mgr->on_transfer_event) {
        mgr->on_transfer_event(task, "pending");
    }

    return 0;
}

/* ====================================================================== */
/* 取消传输 */
/* ====================================================================== */
int transfer_cancel(TransferManager *mgr, const char *task_id) {
    (void)mgr;
    pthread_mutex_lock(&mgr->mutex);

    for (int i = 0; i < mgr->task_count; i++) {
        if (strcmp(mgr->tasks[i].id, task_id) == 0) {
            TransferTask *task = &mgr->tasks[i];
            task->running = false;
            task->status = TRANSFER_CANCELLED;
            task->end_time = time(NULL);

            if (task->tcp_socket >= 0) {
                close(task->tcp_socket);
                task->tcp_socket = -1;
            }

            pthread_mutex_unlock(&mgr->mutex);

            if (mgr->on_transfer_event) {
                mgr->on_transfer_event(task, "cancel");
            }

            return 0;
        }
    }

    pthread_mutex_unlock(&mgr->mutex);
    return -1;
}

/* ====================================================================== */
/* 暂停传输 */
/* ====================================================================== */
int transfer_pause(TransferManager *mgr, const char *task_id) {
    pthread_mutex_lock(&mgr->mutex);

    for (int i = 0; i < mgr->task_count; i++) {
        if (strcmp(mgr->tasks[i].id, task_id) == 0) {
            TransferTask *task = &mgr->tasks[i];
            if (task->status != TRANSFER_TRANSFERRING) {
                pthread_mutex_unlock(&mgr->mutex);
                return -1;
            }
            task->status = TRANSFER_PAUSED;
            task->checkpoint = task->bytes_transferred;
            pthread_mutex_unlock(&mgr->mutex);

            if (mgr->on_transfer_event) {
                mgr->on_transfer_event(task, "pause");
            }
            return 0;
        }
    }

    pthread_mutex_unlock(&mgr->mutex);
    return -1;
}

/* ====================================================================== */
/* 恢复传输 */
/* ====================================================================== */
int transfer_resume(TransferManager *mgr, const char *task_id) {
    pthread_mutex_lock(&mgr->mutex);

    for (int i = 0; i < mgr->task_count; i++) {
        if (strcmp(mgr->tasks[i].id, task_id) == 0) {
            TransferTask *task = &mgr->tasks[i];
            if (task->status != TRANSFER_PAUSED && task->status != TRANSFER_FAILED) {
                pthread_mutex_unlock(&mgr->mutex);
                return -1;
            }
            task->status = TRANSFER_TRANSFERRING;
            pthread_mutex_unlock(&mgr->mutex);

            /* 重新创建线程继续传输 */
            pthread_create(&task->thread, NULL, send_file_thread, task);

            if (mgr->on_transfer_event) {
                mgr->on_transfer_event(task, "resume");
            }
            return 0;
        }
    }

    pthread_mutex_unlock(&mgr->mutex);
    return -1;
}

/* ====================================================================== */
/* 获取任务 */
/* ====================================================================== */
TransferTask* transfer_get_task(TransferManager *mgr, const char *task_id) {
    pthread_mutex_lock(&mgr->mutex);

    for (int i = 0; i < mgr->task_count; i++) {
        if (strcmp(mgr->tasks[i].id, task_id) == 0) {
            pthread_mutex_unlock(&mgr->mutex);
            return &mgr->tasks[i];
        }
    }

    pthread_mutex_unlock(&mgr->mutex);
    return NULL;
}

/* ====================================================================== */
/* 获取所有任务 */
/* ====================================================================== */
int transfer_get_all_tasks(TransferManager *mgr, TransferTask **tasks, int *count) {
    pthread_mutex_lock(&mgr->mutex);

    /* Drop finished tasks sitting at the end so they do not occupy the array
     * forever. */
    compact_finished_tail(mgr);

    *tasks = mgr->tasks;
    *count = mgr->task_count;

    pthread_mutex_unlock(&mgr->mutex);
    return 0;
}

/* ====================================================================== */
/* 获取活动任务 */
/* ====================================================================== */
int transfer_get_active_tasks(TransferManager *mgr, TransferTask **tasks, int *count) {
    static TransferTask snapshot[MAX_TASKS];
    pthread_mutex_lock(&mgr->mutex);

    int n = 0;
    for (int i = 0; i < mgr->task_count; i++) {
        if (mgr->tasks[i].status == TRANSFER_PENDING ||
            mgr->tasks[i].status == TRANSFER_TRANSFERRING ||
            mgr->tasks[i].status == TRANSFER_PAUSED) {
            snapshot[n++] = mgr->tasks[i];
        }
    }

    pthread_mutex_unlock(&mgr->mutex);

    *tasks = snapshot;
    *count = n;
    return 0;
}

/* ====================================================================== */
/* 获取失败任务 */
/* ====================================================================== */
int transfer_get_failed_tasks(TransferManager *mgr, TransferTask **tasks, int *count) {
    static TransferTask snapshot[MAX_TASKS];
    pthread_mutex_lock(&mgr->mutex);

    int n = 0;
    for (int i = 0; i < mgr->task_count; i++) {
        if (mgr->tasks[i].status == TRANSFER_FAILED) {
            snapshot[n++] = mgr->tasks[i];
        }
    }

    pthread_mutex_unlock(&mgr->mutex);

    *tasks = snapshot;
    *count = n;
    return 0;
}

/* ====================================================================== */
/* 设置事件回调 */
/* ====================================================================== */
void transfer_set_on_event(TransferManager *mgr, void (*callback)(TransferTask *, const char *)) {
    mgr->on_transfer_event = callback;
}

/* ====================================================================== */
/* 发送文件线程 */
/* ====================================================================== */
static void* send_file_thread(void *arg) {
    TransferTask *task = (TransferTask *)arg;

    /* 打开文件 */
    FILE *file = fopen(task->file_path, "rb");
    if (!file) {
        snprintf(task->error, sizeof(task->error), "无法打开文件: %s",
                 strerror(errno));
        task->status = TRANSFER_FAILED;
        task->end_time = time(NULL);
        return NULL;
    }

    /* 连接到对等节点 */
    int sock = socket(AF_INET, SOCK_STREAM, 0);
    if (sock < 0) {
        snprintf(task->error, sizeof(task->error), "创建 socket 失败");
        task->status = TRANSFER_FAILED;
        fclose(file);
        return NULL;
    }

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(DEFAULT_PORT);

    if (inet_pton(AF_INET, task->peer_addr, &addr.sin_addr) <= 0) {
        snprintf(task->error, sizeof(task->error), "无效的地址: %s",
                 task->peer_addr);
        task->status = TRANSFER_FAILED;
        close(sock);
        fclose(file);
        return NULL;
    }

    if (connect(sock, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        snprintf(task->error, sizeof(task->error), "连接失败: %s",
                 strerror(errno));
        task->status = TRANSFER_FAILED;
        close(sock);
        fclose(file);
        return NULL;
    }

    /* Go 兼容协议：不进行自定义握手，直接开始发送传输包 */
    task->tcp_socket = sock;
    task->status = TRANSFER_TRANSFERRING;
    task->start_time = time(NULL);
    task->bytes_transferred = 0;

    /* 分块发送 */
    unsigned char buffer[CHUNK_SIZE + MAX_HEADER_SIZE];
    int64_t total_chunks = (task->file_size + CHUNK_SIZE - 1) / CHUNK_SIZE;
    int64_t chunk_index = 0;

    while (task->running && chunk_index < total_chunks) {
        /* 检查暂停 */
        if (task->status == TRANSFER_PAUSED) {
            usleep(100000);
            continue;
        }

        /* 读取文件块 */
        size_t to_read = CHUNK_SIZE;
        if ((int64_t)to_read > task->file_size - task->bytes_transferred) {
            to_read = (size_t)(task->file_size - task->bytes_transferred);
        }

        size_t bytes_read = fread(buffer + MAX_HEADER_SIZE, 1, to_read, file);
        if (bytes_read == 0 && task->bytes_transferred < task->file_size) {
            snprintf(task->error, sizeof(task->error), "读取文件失败");
            task->status = TRANSFER_FAILED;
            break;
        }

        /* 构建并发送传输头 */
        int64_t chunk_offset = chunk_index * (int64_t)CHUNK_SIZE;
        TransferHeader header;
        init_transfer_header(&header, task->id, task->file_name,
                            bytes_read, task->file_size,
                            chunk_index, chunk_offset, total_chunks,
                            "", "");

        int packet_size = build_transfer_packet(&header,
                                                buffer + MAX_HEADER_SIZE,
                                                bytes_read,
                                                buffer,
                                                sizeof(buffer));
        if (packet_size < 0) {
            snprintf(task->error, sizeof(task->error), "构建数据包失败");
            task->status = TRANSFER_FAILED;
            break;
        }

        if (send_all(sock, buffer, packet_size) < 0) {
            snprintf(task->error, sizeof(task->error),
                     "发送块 %" PRId64 "/%" PRId64 " 失败 (已发送 %" PRId64 "/%" PRId64 " bytes): %s",
                     chunk_index, total_chunks, task->bytes_transferred, task->file_size,
                     strerror(errno));
            task->status = TRANSFER_FAILED;
            break;
        }

        task->bytes_transferred += bytes_read;
        chunk_index++;

        /* 计算进度和速度 */
        transfer_calculate_progress(task);
        double elapsed = difftime(time(NULL), task->start_time);
        if (elapsed > 0) {
            task->speed = task->bytes_transferred / elapsed;
        }
    }

    fclose(file);

    /* Only claim success when the whole file actually went out. The loop above
     * used to exit immediately because `running` was never set to true, which
     * marked every send COMPLETED while zero bytes had been transmitted. */
    if (task->status == TRANSFER_TRANSFERRING) {
        if (task->bytes_transferred == task->file_size) {
            task->status = TRANSFER_COMPLETED;
        } else {
            snprintf(task->error, sizeof(task->error),
                     "传输不完整: 已发送 %" PRId64 "/%" PRId64 " bytes",
                     task->bytes_transferred, task->file_size);
            task->status = TRANSFER_FAILED;
        }
    }

    task->end_time = time(NULL);
    task->running = false;

    /* Close before clearing the handle, otherwise the fd is never released and
     * the process leaks one connected socket per send. */
    if (task->tcp_socket >= 0) {
        close(task->tcp_socket);
        task->tcp_socket = -1;
    }

    return NULL;
}

/* ====================================================================== */
/* 接收文件线程 - Go 兼容版：动态创建任务，逐块接收 */
/* ====================================================================== */
static void* receive_file_thread(void *arg) {
    ReceiveArg *recv_arg = (ReceiveArg *)arg;
    TransferManager *mgr = recv_arg->mgr;
    int client_socket = recv_arg->client_socket;

    /* 客户端的 IP */
    char client_ip[INET_ADDRSTRLEN];
    inet_ntop(AF_INET, &recv_arg->client_addr.sin_addr, client_ip, sizeof(client_ip));
    free(recv_arg);

    /* 动态创建的传输任务 */
    TransferTask *task = NULL;
    int64_t file_size = 0;

    /* 从连接中连续读取传输包 */
    unsigned char buffer[CHUNK_SIZE + MAX_HEADER_SIZE + HEADER_SIZE];

    while (mgr->running) {
        /* 1. 读取 4 字节头部大小（大端序）*/
        unsigned char size_buf[HEADER_SIZE];
        if (recv_all(client_socket, size_buf, HEADER_SIZE) <= 0) {
            break;
        }

        uint32_t header_json_size = ((uint32_t)size_buf[0] << 24) |
                                    ((uint32_t)size_buf[1] << 16) |
                                    ((uint32_t)size_buf[2] << 8) |
                                    ((uint32_t)size_buf[3]);
        if (header_json_size == 0 || header_json_size > MAX_HEADER_SIZE) {
            break;
        }

        /* 2. 读取 JSON 头部 */
        memcpy(buffer, size_buf, HEADER_SIZE);
        if (recv_all(client_socket, buffer + HEADER_SIZE, header_json_size) <= 0) {
            break;
        }

        /* 3. 解析头部 */
        TransferHeader header;
        if (deserialize_header(buffer, HEADER_SIZE + header_json_size, &header) < 0) {
            break;
        }

        /* 4. 读取块数据 */
        size_t chunk_data_size = (size_t)header.file_size;
        if (chunk_data_size > 0) {
            size_t data_offset = HEADER_SIZE + header_json_size;
            if (data_offset + chunk_data_size > sizeof(buffer)) {
                break;
            }
            if (recv_all(client_socket, buffer + data_offset, chunk_data_size) <= 0) {
                break;
            }
        }

        /* 5. 如果是第一个块，动态创建任务 */
        if (task == NULL || strcmp(task->id, header.transfer_id) != 0) {
            /* 关闭上一个任务的 writer（如果有）*/
            if (task && task->status == TRANSFER_TRANSFERRING) {
                task->status = TRANSFER_COMPLETED;
                task->end_time = time(NULL);
                task->progress = 100.0;
                if (mgr->on_transfer_event) {
                    mgr->on_transfer_event(task, "complete");
                }
            }

            /* 计算保存路径 */
            char save_path[MAX_FILE_PATH];
            if (header.relative_path[0]) {
                snprintf(save_path, sizeof(save_path), "%s/%s",
                         mgr->default_save_path, header.relative_path);
                /* 创建子目录 */
                char dir[MAX_FILE_PATH];
                snprintf(dir, sizeof(dir), "%s", save_path);
                char *p = strrchr(dir, '/');
                if (p) {
                    *p = '\0';
                    transfer_create_directories(dir);
                }
            } else {
                snprintf(save_path, sizeof(save_path), "%s/%s",
                         mgr->default_save_path, header.file_name);
                transfer_get_unique_filename(save_path, save_path, sizeof(save_path));
            }

            file_size = header.total_file_size;

            /* 在新任务槽中创建任务（或在已存在的任务中复用）*/
            pthread_mutex_lock(&mgr->mutex);

            /* Reuses a live task with the same id, else recycles a finished
             * slot, else appends. */
            int task_idx = acquire_task_slot(mgr, header.transfer_id);

            if (task_idx < 0) {
                pthread_mutex_unlock(&mgr->mutex);
                fprintf(stderr, "[TRANSFER] 任务槽位已满，拒绝新传输\n");
                break;
            }

            task = &mgr->tasks[task_idx];

            /* 关闭旧的 socket（如果有）*/
            if (task->tcp_socket >= 0 && task->tcp_socket != client_socket) {
                close(task->tcp_socket);
            }

            /* 重试场景：保留 file_name/file_path 等元数据，只重置运行时状态 */
            /* 不要 memset 整个 task，否则会清空正在被其他线程读取的字段 */
            task->file_size = file_size;
            task->bytes_transferred = 0;
            task->status = TRANSFER_TRANSFERRING;
            task->start_time = time(NULL);
            task->end_time = 0;
            task->speed = 0.0;
            task->progress = 0.0;
            task->tcp_socket = client_socket;
            task->running = true;
            task->checkpoint = 0;
            task->error[0] = '\0';

            /* 首次初始化时设置元数据 */
            if (task->id[0] == '\0') {
                strncpy(task->id, header.transfer_id, sizeof(task->id) - 1);
                strncpy(task->peer_name, "", sizeof(task->peer_name) - 1);
                strncpy(task->peer_addr, client_ip, sizeof(task->peer_addr) - 1);
                strncpy(task->file_name, header.file_name, sizeof(task->file_name) - 1);
                strncpy(task->file_path, save_path, sizeof(task->file_path) - 1);
                strncpy(task->relative_path, header.relative_path, sizeof(task->relative_path) - 1);
                task->type = TRANSFER_TYPE_FILE;
                task->is_sender = false;
            }
            task->peer_addr[0] = '\0';
            strncpy(task->peer_addr, client_ip, sizeof(task->peer_addr) - 1);

            pthread_mutex_unlock(&mgr->mutex);

            if (mgr->on_transfer_event) {
                mgr->on_transfer_event(task, "start");
            }
        }

        /* 6. 写入块数据 */
        if (chunk_data_size > 0) {
            /* 新协议优先使用发送端给出的绝对偏移；老版本 Go 端不带该字段，
               回退到本地 chunk_size 推算。 */
            int64_t write_offset = header.chunk_offset;
            if (write_offset < 0) {
                write_offset = header.chunk_index * (int64_t)mgr->chunk_size;
            }

            /* "r+b" 而不是 "ab"：追加模式会让内核忽略 fseeko，把每个块都
               追加到文件末尾，乱序/续传时必然写坏文件。 */
            FILE *file = fopen(task->file_path, "r+b");
            bool created = false;
            if (!file) {
                file = fopen(task->file_path, "w+b");
                created = true;
            }
            if (!file) {
                snprintf(task->error, sizeof(task->error),
                         "无法打开文件: %s", strerror(errno));
                task->status = TRANSFER_FAILED;
                break;
            }
            (void)created;

            if (write_offset > 0 && fseeko(file, (off_t)write_offset, SEEK_SET) != 0) {
                snprintf(task->error, sizeof(task->error), "无法定位到偏移 %" PRId64, write_offset);
                task->status = TRANSFER_FAILED;
                fclose(file);
                break;
            }

            size_t written = fwrite(buffer + HEADER_SIZE + header_json_size,
                                    1, chunk_data_size, file);
            if (written != chunk_data_size) {
                snprintf(task->error, sizeof(task->error), "写入文件失败");
                task->status = TRANSFER_FAILED;
                fclose(file);
                break;
            }
            fclose(file);

            task->bytes_transferred += (int64_t)chunk_data_size;
        }

        task->tcp_socket = client_socket;
        task->running = true;

        /* 更新进度和速度 */
        transfer_calculate_progress(task);
        double elapsed = difftime(time(NULL), task->start_time);
        if (elapsed > 0) {
            task->speed = (double)task->bytes_transferred / elapsed;
        }

        /* 触发进度事件 */
        if (mgr->on_transfer_event) {
            mgr->on_transfer_event(task, "progress");
        }
    }

    /* 标记任务完成 */
    if (task) {
        /* 检查任务是否已被其他线程接管（重试场景），如果是则不做清理 */
        pthread_mutex_lock(&mgr->mutex);
        bool task_reused = (task->tcp_socket != client_socket);
        pthread_mutex_unlock(&mgr->mutex);

        if (task_reused) {
            /* 此 task 已被新连接复用，不做任何状态修改。
               旧线程持有的是自己那个 socket，fd 已经在新连接接管时被关掉了，
               再 close 一次会误关内核刚刚分配出去的无关 fd。 */
            return NULL;
        }

        if (task->status == TRANSFER_TRANSFERRING) {
            if (task->bytes_transferred >= task->file_size && task->file_size > 0) {
                task->status = TRANSFER_COMPLETED;
                task->progress = 100.0;
            } else {
                task->status = TRANSFER_FAILED;
                if (task->error[0] == '\0') {
                    snprintf(task->error, sizeof(task->error),
                             "连接中断：仅收到 %" PRId64 "/%" PRId64 " 字节",
                             task->bytes_transferred, task->file_size);
                }
                if (mgr->on_transfer_event) {
                    mgr->on_transfer_event(task, "error");
                }
            }
        }
        task->end_time = time(NULL);
        task->running = false;

        if (task->tcp_socket >= 0) {
            close(task->tcp_socket);
            task->tcp_socket = -1;
            /* This was our own socket; the cleanup below must not close it twice. */
            client_socket = -1;
        }

        if (mgr->on_transfer_event) {
            mgr->on_transfer_event(task,
                task->status == TRANSFER_COMPLETED ? "complete" : "error");
        }
    }

    if (client_socket >= 0) {
        close(client_socket);
    }
    return NULL;
}

/* ====================================================================== */
/* send_all - 确保全部数据发送完毕，带 EINTR 重试和连接断开检测 */
/* ====================================================================== */
static int send_all(int socket, const unsigned char *data, size_t size) {
    size_t total_sent = 0;
    int retry_count = 0;
    const int max_retries = 3;

    while (total_sent < size) {
        ssize_t sent = send(socket, data + total_sent, size - total_sent, MSG_NOSIGNAL);
        if (sent > 0) {
            total_sent += (size_t)sent;
            retry_count = 0;
            continue;
        }

        /* sent <= 0: 出错 */
        if (sent == 0) {
            /* 对端关闭连接 */
            fprintf(stderr, "[send_all] 对端关闭连接 (已发送 %zu/%zu bytes)\n",
                    total_sent, size);
            return -1;
        }

        /* sent < 0: 系统错误 */
        if (errno == EINTR) {
            /* 被信号中断，重试 */
            retry_count++;
            if (retry_count > max_retries) {
                fprintf(stderr, "[send_all] EINTR 重试 %d 次后放弃\n", max_retries);
                return -1;
            }
            continue;
        }

        if (errno == EAGAIN || errno == EWOULDBLOCK) {
            /* 非阻塞模式下缓冲区满，短暂等待后重试 */
            usleep(10000);
            retry_count++;
            if (retry_count > max_retries) {
                fprintf(stderr, "[send_all] EAGAIN 重试 %d 次后放弃\n", max_retries);
                return -1;
            }
            continue;
        }

        if (errno == EPIPE) {
            /* 连接已断开（broken pipe）*/
            fprintf(stderr, "[send_all] broken pipe: 对端断开连接 (已发送 %zu/%zu bytes)\n",
                    total_sent, size);
            return -1;
        }

        if (errno == ECONNRESET) {
            fprintf(stderr, "[send_all] connection reset: 对端重置连接\n");
            return -1;
        }

        /* 其他错误 */
        fprintf(stderr, "[send_all] send 失败: %s (errno=%d)\n",
                strerror(errno), errno);
        return -1;
    }

    return (int)total_sent;
}

/* ====================================================================== */
/* recv_all - 确保全部数据接收完毕，带 EINTR 重试 */
/* ====================================================================== */
static int recv_all(int socket, unsigned char *data, size_t size) {
    size_t total_recv = 0;
    int retry_count = 0;
    const int max_retries = 3;

    while (total_recv < size) {
        ssize_t n = recv(socket, data + total_recv, size - total_recv, 0);
        if (n > 0) {
            total_recv += (size_t)n;
            retry_count = 0;
            continue;
        }

        /* n == 0: 对端关闭连接 */
        if (n == 0) {
            fprintf(stderr, "[recv_all] 对端关闭连接 (已接收 %zu/%zu bytes)\n",
                    total_recv, size);
            return -1;
        }

        /* n < 0: 系统错误 */
        if (errno == EINTR) {
            retry_count++;
            if (retry_count > max_retries) {
                fprintf(stderr, "[recv_all] EINTR 重试 %d 次后放弃\n", max_retries);
                return -1;
            }
            continue;
        }

        if (errno == EAGAIN || errno == EWOULDBLOCK) {
            usleep(10000);
            retry_count++;
            if (retry_count > max_retries) {
                fprintf(stderr, "[recv_all] EAGAIN 重试 %d 次后放弃\n", max_retries);
                return -1;
            }
            continue;
        }

        if (errno == ECONNRESET) {
            fprintf(stderr, "[recv_all] connection reset: 对端重置连接\n");
            return -1;
        }

        fprintf(stderr, "[recv_all] recv 失败: %s (errno=%d)\n",
                strerror(errno), errno);
        return -1;
    }

    return (int)total_recv;
}

/* ====================================================================== */
/* 工具函数 */
/* ====================================================================== */

const char* transfer_status_str(TransferStatus status) {
    switch (status) {
        case TRANSFER_PENDING:      return "等待中";
        case TRANSFER_TRANSFERRING: return "传输中";
        case TRANSFER_COMPLETED:    return "已完成";
        case TRANSFER_FAILED:       return "失败";
        case TRANSFER_CANCELLED:    return "已取消";
        case TRANSFER_PAUSED:       return "已暂停";
        default:                    return "未知";
    }
}

const char* transfer_type_str(TransferType type) {
    switch (type) {
        case TRANSFER_TYPE_FILE:   return "文件";
        case TRANSFER_TYPE_FOLDER: return "文件夹";
        default:                   return "未知";
    }
}

void transfer_calculate_progress(TransferTask *task) {
    if (task->file_size > 0) {
        task->progress = (double)task->bytes_transferred / (double)task->file_size * 100.0;
        if (task->progress > 100.0) {
            task->progress = 100.0;
        }
    }
}

void transfer_calculate_speed(TransferTask *task, double elapsed_seconds) {
    if (elapsed_seconds > 0) {
        task->speed = task->bytes_transferred / elapsed_seconds;
    }
}

bool transfer_is_directory(const char *path) {
    struct stat st;
    if (stat(path, &st) != 0) {
        return false;
    }
    return S_ISDIR(st.st_mode);
}

int64_t transfer_get_file_size(const char *path) {
    struct stat st;
    if (stat(path, &st) != 0) {
        return -1;
    }
    return (int64_t)st.st_size;
}

int transfer_create_directories(const char *path) {
    char tmp[512];
    char *p;
    size_t len;

    snprintf(tmp, sizeof(tmp), "%s", path);
    len = strlen(tmp);
    if (tmp[len - 1] == '/') {
        tmp[len - 1] = '\0';
    }

    for (p = tmp + 1; *p; p++) {
        if (*p == '/') {
            *p = '\0';
            mkdir(tmp, 0755);
            *p = '/';
        }
    }
    return mkdir(tmp, 0755);
}

void transfer_get_unique_filename(const char *path, char *result, size_t result_size) {
    if (access(path, F_OK) != 0) {
        strncpy(result, path, result_size);
        return;
    }

    char base[512];
    char ext[128];
    strncpy(base, path, sizeof(base) - 1);

    char *dot = strrchr(base, '.');
    if (dot) {
        strncpy(ext, dot, sizeof(ext) - 1);
        *dot = '\0';
    } else {
        ext[0] = '\0';
    }

    for (int i = 1; i < 10000; i++) {
        char new_path[1024];
        snprintf(new_path, sizeof(new_path), "%s_%d%s", base, i, ext);
        if (access(new_path, F_OK) != 0) {
            strncpy(result, new_path, result_size);
            return;
        }
    }

    /* 使用时间戳作为最后的方案 */
    snprintf(result, result_size, "%s_%ld%s", base, time(NULL), ext);
}