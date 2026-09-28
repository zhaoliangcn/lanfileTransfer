#ifndef TRANSFER_H
#define TRANSFER_H

#include <stdint.h>
#include <stddef.h>
#include <pthread.h>
#include <stdbool.h>
#include <time.h>

#include "protocol.h"

#define MAX_TRANSFER_ID 64
#define MAX_FILE_PATH 512
#define MAX_FILE_NAME 256
#define MAX_PEER_ID 64
#define MAX_PEER_NAME 128
#define MAX_PEER_ADDR 46
#define MAX_ERROR_MSG 256
#define MAX_TASKS 100
#define DEFAULT_CHUNK_SIZE (64 * 1024)

/* Forward declarations */
typedef struct TransferTask TransferTask;
typedef struct TransferManager TransferManager;

/* Transfer task */
struct TransferTask {
    char id[MAX_TRANSFER_ID];
    char peer_id[MAX_PEER_ID];
    char peer_name[MAX_PEER_NAME];
    char peer_addr[MAX_PEER_ADDR];
    char file_name[MAX_FILE_NAME];
    char file_path[MAX_FILE_PATH];
    char relative_path[MAX_FILE_PATH];
    int64_t file_size;
    int64_t bytes_transferred;
    TransferType type;
    TransferStatus status;
    bool is_sender;
    time_t start_time;
    time_t end_time;
    double speed;
    double progress;
    char error[MAX_ERROR_MSG];
    int64_t checkpoint;  /* For resume support */
    
    /* Internal fields */
    int tcp_socket;
    pthread_t thread;
    volatile bool running;
};

/* Transfer manager */
struct TransferManager {
    int tcp_server_socket;
    int listen_port;
    int chunk_size;
    int max_concurrent;
    char default_save_path[MAX_FILE_PATH];
    bool auto_receive;
    
    TransferTask tasks[MAX_TASKS];
    int task_count;
    
    pthread_t tcp_server_thread;
    pthread_mutex_t mutex;
    
    volatile bool running;
    volatile bool started;
    
    /* Callbacks */
    void (*on_transfer_event)(TransferTask *task, const char *event_type);
};

/* Initialize transfer manager */
int transfer_init(TransferManager *mgr, 
                 int listen_port,
                 int chunk_size,
                 int max_concurrent,
                 const char *save_path,
                 bool auto_receive);

/* Start TCP server for receiving transfers */
int transfer_start(TransferManager *mgr);

/* Stop transfer manager */
void transfer_stop(TransferManager *mgr);

/* Send a file */
int transfer_send_file(TransferManager *mgr,
                      const char *peer_id,
                      const char *peer_name,
                      const char *peer_addr,
                      const char *file_path,
                      char *task_id);

/* Send a folder (multiple files) */
int transfer_send_folder(TransferManager *mgr,
                        const char *peer_id,
                        const char *peer_name,
                        const char *peer_addr,
                        const char *folder_path,
                        char **task_ids,
                        int *task_count);

/* Accept incoming transfer request */
int transfer_accept_request(TransferManager *mgr,
                           const char *peer_id,
                           const char *peer_name,
                           const char *peer_addr,
                           const char *transfer_id,
                           const char *file_name,
                           int64_t file_size,
                           char *task_id);

/* Cancel a transfer */
int transfer_cancel(TransferManager *mgr, const char *task_id);

/* Pause a transfer */
int transfer_pause(TransferManager *mgr, const char *task_id);

/* Resume a paused/failed transfer */
int transfer_resume(TransferManager *mgr, const char *task_id);

/* Get task by ID */
TransferTask* transfer_get_task(TransferManager *mgr, const char *task_id);

/* Get all tasks */
int transfer_get_all_tasks(TransferManager *mgr, TransferTask **tasks, int *count);

/* Get active tasks */
int transfer_get_active_tasks(TransferManager *mgr, TransferTask **tasks, int *count);

/* Get failed tasks */
int transfer_get_failed_tasks(TransferManager *mgr, TransferTask **tasks, int *count);

/* Set callback for transfer events */
void transfer_set_on_event(TransferManager *mgr, void (*callback)(TransferTask*, const char*));

/* Utility functions */
const char* transfer_status_str(TransferStatus status);
const char* transfer_type_str(TransferType type);
void transfer_calculate_progress(TransferTask *task);
void transfer_calculate_speed(TransferTask *task, double elapsed_seconds);

/* Check if path is directory */
bool transfer_is_directory(const char *path);

/* Get file size */
int64_t transfer_get_file_size(const char *path);

/* Create directory recursively */
int transfer_create_directories(const char *path);

/* Generate unique file name if exists */
void transfer_get_unique_filename(const char *path, char *result, size_t result_size);

#endif /* TRANSFER_H */
