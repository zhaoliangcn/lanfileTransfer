#ifndef PROTOCOL_H
#define PROTOCOL_H

#include <stdint.h>
#include <stddef.h>

#define HEADER_SIZE 4
#define MAX_HEADER_SIZE 4096
#define CONTROL_MSG_SIZE 1024
#define CHUNK_SIZE (64 * 1024)
#define DEFAULT_PORT 9876

/* Control message types */
#define CONTROL_TYPE_DISCOVERY       "discovery"
#define CONTROL_TYPE_TRANSFER_REQUEST "transfer_request"
#define CONTROL_TYPE_TRANSFER_CONTROL "transfer_control"
#define CONTROL_TYPE_CHAT_MESSAGE    "chat_message"
#define CONTROL_TYPE_SYSTEM_COMMAND  "system_command"

/* Transfer control commands */
#define CONTROL_COMMAND_CANCEL "cancel"
#define CONTROL_COMMAND_PAUSE  "pause"
#define CONTROL_COMMAND_RESUME "resume"

/* System commands */
#define CONTROL_COMMAND_SHUTDOWN "shutdown"
#define CONTROL_COMMAND_RESTART  "restart"

/* Transfer status */
typedef enum {
    STATUS_PENDING,
    STATUS_TRANSFERRING,
    STATUS_COMPLETED,
    STATUS_FAILED,
    STATUS_CANCELLED,
    STATUS_PAUSED
} TransferStatus;

/* Compatibility aliases (transfer.h used TRANSFER_ prefix) */
#define TRANSFER_PENDING STATUS_PENDING
#define TRANSFER_TRANSFERRING STATUS_TRANSFERRING
#define TRANSFER_COMPLETED STATUS_COMPLETED
#define TRANSFER_FAILED STATUS_FAILED
#define TRANSFER_CANCELLED STATUS_CANCELLED
#define TRANSFER_PAUSED STATUS_PAUSED

/* Transfer type */
typedef enum {
    TRANSFER_TYPE_FILE,
    TRANSFER_TYPE_FOLDER
} TransferType;

/* Transfer header - sent before each chunk */
typedef struct {
    char transfer_id[64];
    char file_name[256];
    int64_t file_size;
    int64_t total_file_size;
    int64_t chunk_index;
    int64_t chunk_offset;   /* absolute byte offset of this chunk, -1 if absent */
    int64_t total_chunks;
    char checksum[64];
    char relative_path[512];
} TransferHeader;

/* Control message for discovery and control */
typedef struct {
    char type[32];
    char device_id[64];
    char device_name[128];
    int port;
    int64_t timestamp;
    char transfer_id[64];
    char file_name[256];
    int64_t file_size;
    char command[32];
    char message[1024];
    char target_id[64];
    char auth_token[65];
} ControlMessage;

/* Initialize a transfer header */
void init_transfer_header(TransferHeader *header, 
                         const char *transfer_id, 
                         const char *file_name,
                         int64_t file_size,
                         int64_t total_file_size,
                         int64_t chunk_index,
                         int64_t chunk_offset,
                         int64_t total_chunks,
                         const char *checksum,
                         const char *relative_path);

/* Initialize a control message */
void init_control_message(ControlMessage *msg, 
                         const char *type,
                         const char *device_id,
                         const char *device_name,
                         int port);

/* Serialize transfer header to binary packet */
/* Returns packet size, or -1 on error */
int serialize_header(const TransferHeader *header, unsigned char *buffer, size_t buffer_size);

/* Deserialize transfer header from binary packet */
/* Returns 0 on success, -1 on error */
int deserialize_header(const unsigned char *buffer, size_t buffer_size, TransferHeader *header);

/* Serialize control message to JSON string */
/* Returns string length, or -1 on error */
int serialize_control_message(const ControlMessage *msg, char *buffer, size_t buffer_size);

/* Deserialize control message from JSON string */
/* Returns 0 on success, -1 on error */
int deserialize_control_message(const char *json_str, ControlMessage *msg);

/* Build transfer packet (header + chunk data) */
/* Returns total packet size, or -1 on error */
int build_transfer_packet(const TransferHeader *header, 
                         const unsigned char *chunk_data,
                         size_t chunk_size,
                         unsigned char *buffer,
                         size_t buffer_size);

/* Parse transfer packet from buffer */
/* Returns chunk data size, or -1 on error */
int parse_transfer_packet(const unsigned char *buffer, 
                         size_t buffer_size,
                         TransferHeader *header,
                         unsigned char *chunk_data,
                         size_t max_chunk_size);

#endif /* PROTOCOL_H */
