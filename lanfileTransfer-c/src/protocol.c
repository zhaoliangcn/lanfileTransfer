#include "protocol.h"
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <time.h>
#include <inttypes.h>
#include <arpa/inet.h>

/* Simple JSON serialization without external library */
/* Note: For production, consider using cJSON or jansson */

static void escape_json_string(const char *src, char *dst, size_t dst_size) {
    size_t j = 0;
    for (size_t i = 0; src[i] && j < dst_size - 1; i++) {
        char c = src[i];
        if (c == '"' || c == '\\' || c == '\n' || c == '\r' || c == '\t') {
            if (j + 2 < dst_size) {
                dst[j++] = '\\';
                dst[j++] = c;
            }
        } else {
            dst[j++] = c;
        }
    }
    dst[j] = '\0';
}

void init_transfer_header(TransferHeader *header, 
                         const char *transfer_id, 
                         const char *file_name,
                         int64_t file_size,
                         int64_t total_file_size,
                         int64_t chunk_index,
                         int64_t chunk_offset,
                         int64_t total_chunks,
                         const char *checksum,
                         const char *relative_path) {
    memset(header, 0, sizeof(TransferHeader));
    if (transfer_id) strncpy(header->transfer_id, transfer_id, sizeof(header->transfer_id) - 1);
    if (file_name) strncpy(header->file_name, file_name, sizeof(header->file_name) - 1);
    header->file_size = file_size;
    header->total_file_size = total_file_size;
    header->chunk_index = chunk_index;
    header->chunk_offset = chunk_offset;
    header->total_chunks = total_chunks;
    if (checksum) strncpy(header->checksum, checksum, sizeof(header->checksum) - 1);
    if (relative_path) strncpy(header->relative_path, relative_path, sizeof(header->relative_path) - 1);
}

void init_control_message(ControlMessage *msg, 
                         const char *type,
                         const char *device_id,
                         const char *device_name,
                         int port) {
    memset(msg, 0, sizeof(ControlMessage));
    if (type) strncpy(msg->type, type, sizeof(msg->type) - 1);
    if (device_id) strncpy(msg->device_id, device_id, sizeof(msg->device_id) - 1);
    if (device_name) strncpy(msg->device_name, device_name, sizeof(msg->device_name) - 1);
    msg->port = port;
    msg->timestamp = time(NULL);
}

int serialize_header(const TransferHeader *header, unsigned char *buffer, size_t buffer_size) {
    /* Format: [4-byte size][JSON data] */
    if (buffer_size < HEADER_SIZE + 100) return -1;
    
    char json[4096];
    int len = snprintf(json, sizeof(json),
        "{\"transferId\":\"%s\",\"fileName\":\"%s\",\"fileSize\":%"PRId64","
        "\"totalFileSize\":%"PRId64",\"chunkIndex\":%"PRId64","
        "\"chunkOffset\":%"PRId64",\"totalChunks\":%"PRId64","
        "\"checksum\":\"%s\",\"relativePath\":\"%s\"}",
        header->transfer_id,
        header->file_name,
        header->file_size,
        header->total_file_size,
        header->chunk_index,
        header->chunk_offset,
        header->total_chunks,
        header->checksum,
        header->relative_path);
    
    if (len <= 0 || len >= (int)buffer_size - HEADER_SIZE) return -1;
    
    /* Write size header (big-endian) */
    uint32_t size = (uint32_t)len;
    buffer[0] = (size >> 24) & 0xFF;
    buffer[1] = (size >> 16) & 0xFF;
    buffer[2] = (size >> 8) & 0xFF;
    buffer[3] = size & 0xFF;
    
    memcpy(buffer + HEADER_SIZE, json, len);
    return HEADER_SIZE + len;
}

int deserialize_header(const unsigned char *buffer, size_t buffer_size, TransferHeader *header) {
    if (buffer_size < HEADER_SIZE) return -1;
    
    /* Read size header (big-endian) */
    uint32_t size = ((uint32_t)buffer[0] << 24) |
                    ((uint32_t)buffer[1] << 16) |
                    ((uint32_t)buffer[2] << 8) |
                    ((uint32_t)buffer[3]);
    
    if (size == 0 || size > MAX_HEADER_SIZE || size > buffer_size - HEADER_SIZE) return -1;
    
    /* Simple JSON parsing for TransferHeader */
    const char *json = (const char *)buffer + HEADER_SIZE;
    memset(header, 0, sizeof(TransferHeader));
    
    /* Parse fields using simple string search */
    char *temp = malloc(size + 1);
    if (!temp) return -1;
    memcpy(temp, json, size);
    temp[size] = '\0';
    
    char *ptr;
    
    ptr = strstr(temp, "\"transferId\":\"");
    if (ptr) {
        ptr += 14;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(header->transfer_id)) len = sizeof(header->transfer_id) - 1;
            memcpy(header->transfer_id, ptr, len);
            header->transfer_id[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"fileName\":\"");
    if (ptr) {
        ptr += 12;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(header->file_name)) len = sizeof(header->file_name) - 1;
            memcpy(header->file_name, ptr, len);
            header->file_name[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"fileSize\":");
    if (ptr) {
        header->file_size = strtoll(ptr + 11, NULL, 10);
    }
    
    ptr = strstr(temp, "\"totalFileSize\":");
    if (ptr) {
        header->total_file_size = strtoll(ptr + 16, NULL, 10);
    }
    
    ptr = strstr(temp, "\"chunkIndex\":");
    if (ptr) {
        header->chunk_index = strtoll(ptr + 13, NULL, 10);
    }

    /* chunkOffset was added alongside chunkIndex; older peers omit it, in which
     * case the receiver falls back to chunk_index * local chunk size. */
    ptr = strstr(temp, "\"chunkOffset\":");
    if (ptr) {
        header->chunk_offset = strtoll(ptr + 14, NULL, 10);
    } else {
        header->chunk_offset = -1;
    }
    
    ptr = strstr(temp, "\"totalChunks\":");
    if (ptr) {
        header->total_chunks = strtoll(ptr + 14, NULL, 10);
    }
    
    ptr = strstr(temp, "\"checksum\":\"");
    if (ptr) {
        ptr += 12;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(header->checksum)) len = sizeof(header->checksum) - 1;
            memcpy(header->checksum, ptr, len);
            header->checksum[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"relativePath\":\"");
    if (ptr) {
        ptr += 16;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(header->relative_path)) len = sizeof(header->relative_path) - 1;
            memcpy(header->relative_path, ptr, len);
            header->relative_path[len] = '\0';
        }
    }
    
    free(temp);
    return 0;
}

/* Append to buffer, tracking the remaining capacity. snprintf() returns the
 * length it *would* have written, so accumulating into len and then computing
 * `buffer_size - len` underflows (size_t wraps) as soon as one field is
 * truncated -- which then lets the next snprintf() write far past the buffer. */
static int append_json(char *buffer, size_t buffer_size, size_t *offset, const char *fmt, ...) {
    va_list args;
    int written;

    if (*offset >= buffer_size) return -1;

    va_start(args, fmt);
    written = vsnprintf(buffer + *offset, buffer_size - *offset, fmt, args);
    va_end(args);

    if (written < 0 || (size_t)written >= buffer_size - *offset) return -1;

    *offset += (size_t)written;
    return 0;
}

int serialize_control_message(const ControlMessage *msg, char *buffer, size_t buffer_size) {
    char escaped_name[256];
    char escaped_msg[1024];
    char escaped_file[256];
    char escaped_target[64];

    if (buffer_size == 0) return -1;

    escape_json_string(msg->device_name, escaped_name, sizeof(escaped_name));
    escape_json_string(msg->message, escaped_msg, sizeof(escaped_msg));
    escape_json_string(msg->file_name, escaped_file, sizeof(escaped_file));
    escape_json_string(msg->target_id, escaped_target, sizeof(escaped_target));

    size_t len = 0;

    if (append_json(buffer, buffer_size, &len, "{\"type\":\"%s\"", msg->type) < 0) return -1;

    if (msg->device_id[0] &&
        append_json(buffer, buffer_size, &len, ",\"deviceId\":\"%s\"", msg->device_id) < 0) return -1;
    if (msg->device_name[0] &&
        append_json(buffer, buffer_size, &len, ",\"deviceName\":\"%s\"", escaped_name) < 0) return -1;
    if (msg->port &&
        append_json(buffer, buffer_size, &len, ",\"port\":%d", msg->port) < 0) return -1;
    if (msg->timestamp &&
        append_json(buffer, buffer_size, &len, ",\"timestamp\":%" PRId64, msg->timestamp) < 0) return -1;
    if (msg->transfer_id[0] &&
        append_json(buffer, buffer_size, &len, ",\"transferId\":\"%s\"", msg->transfer_id) < 0) return -1;
    if (msg->file_name[0] &&
        append_json(buffer, buffer_size, &len, ",\"fileName\":\"%s\"", escaped_file) < 0) return -1;
    if (msg->file_size &&
        append_json(buffer, buffer_size, &len, ",\"fileSize\":%" PRId64, msg->file_size) < 0) return -1;
    if (msg->command[0] &&
        append_json(buffer, buffer_size, &len, ",\"command\":\"%s\"", msg->command) < 0) return -1;
    if (msg->message[0] &&
        append_json(buffer, buffer_size, &len, ",\"message\":\"%s\"", escaped_msg) < 0) return -1;
    if (msg->target_id[0] &&
        append_json(buffer, buffer_size, &len, ",\"targetId\":\"%s\"", escaped_target) < 0) return -1;
    if (msg->auth_token[0] &&
        append_json(buffer, buffer_size, &len, ",\"authToken\":\"%s\"", msg->auth_token) < 0) return -1;

    if (append_json(buffer, buffer_size, &len, "}") < 0) return -1;

    return (int)len;
}

int deserialize_control_message(const char *json_str, ControlMessage *msg) {
    memset(msg, 0, sizeof(ControlMessage));
    
    char *temp = strdup(json_str);
    if (!temp) return -1;
    
    char *ptr;
    
    ptr = strstr(temp, "\"type\":\"");
    if (ptr) {
        ptr += 8;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->type)) len = sizeof(msg->type) - 1;
            memcpy(msg->type, ptr, len);
            msg->type[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"deviceId\":\"");
    if (ptr) {
        ptr += 12;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->device_id)) len = sizeof(msg->device_id) - 1;
            memcpy(msg->device_id, ptr, len);
            msg->device_id[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"deviceName\":\"");
    if (ptr) {
        ptr += 14;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->device_name)) len = sizeof(msg->device_name) - 1;
            memcpy(msg->device_name, ptr, len);
            msg->device_name[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"port\":");
    if (ptr) {
        msg->port = (int)strtol(ptr + 7, NULL, 10);
    }
    
    ptr = strstr(temp, "\"timestamp\":");
    if (ptr) {
        msg->timestamp = strtoll(ptr + 12, NULL, 10);
    }
    
    ptr = strstr(temp, "\"transferId\":\"");
    if (ptr) {
        ptr += 14;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->transfer_id)) len = sizeof(msg->transfer_id) - 1;
            memcpy(msg->transfer_id, ptr, len);
            msg->transfer_id[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"fileName\":\"");
    if (ptr) {
        ptr += 12;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->file_name)) len = sizeof(msg->file_name) - 1;
            memcpy(msg->file_name, ptr, len);
            msg->file_name[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"fileSize\":");
    if (ptr) {
        msg->file_size = strtoll(ptr + 10, NULL, 10);
    }
    
    ptr = strstr(temp, "\"command\":\"");
    if (ptr) {
        ptr += 11;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->command)) len = sizeof(msg->command) - 1;
            memcpy(msg->command, ptr, len);
            msg->command[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"message\":\"");
    if (ptr) {
        ptr += 11;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->message)) len = sizeof(msg->message) - 1;
            memcpy(msg->message, ptr, len);
            msg->message[len] = '\0';
        }
    }
    
    ptr = strstr(temp, "\"targetId\":\"");
    if (ptr) {
        ptr += 12;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->target_id)) len = sizeof(msg->target_id) - 1;
            memcpy(msg->target_id, ptr, len);
            msg->target_id[len] = '\0';
        }
    }

    /* Added after the initial release; older peers simply omit it. */
    ptr = strstr(temp, "\"authToken\":\"");
    if (ptr) {
        ptr += 13;
        char *end = strchr(ptr, '"');
        if (end) {
            size_t len = end - ptr;
            if (len >= sizeof(msg->auth_token)) len = sizeof(msg->auth_token) - 1;
            memcpy(msg->auth_token, ptr, len);
            msg->auth_token[len] = '\0';
        }
    }
    
    free(temp);
    return 0;
}

int build_transfer_packet(const TransferHeader *header, 
                         const unsigned char *chunk_data,
                         size_t chunk_size,
                         unsigned char *buffer,
                         size_t buffer_size) {
    unsigned char header_buf[MAX_HEADER_SIZE];
    int header_len = serialize_header(header, header_buf, sizeof(header_buf));
    if (header_len < 0) return -1;
    
    size_t total = (size_t)header_len + chunk_size;
    if (total > buffer_size) return -1;
    
    /* Callers read the chunk straight into `buffer + MAX_HEADER_SIZE` and then
     * pass that same buffer as the destination, so source and destination
     * overlap. memcpy() is undefined behaviour in that case. */
    memmove(buffer, header_buf, header_len);
    memmove(buffer + header_len, chunk_data, chunk_size);
    
    return (int)total;
}

int parse_transfer_packet(const unsigned char *buffer, 
                         size_t buffer_size,
                         TransferHeader *header,
                         unsigned char *chunk_data,
                         size_t max_chunk_size) {
    if (buffer_size < HEADER_SIZE) return -1;

    /* The payload starts right after the header the sender actually put on the
     * wire. Re-serializing `header` to measure it produced a different length
     * (the Go side omits empty relativePath, the C side always emits it), which
     * silently shifted the payload by 19 bytes. */
    uint32_t wire_size = ((uint32_t)buffer[0] << 24) |
                         ((uint32_t)buffer[1] << 16) |
                         ((uint32_t)buffer[2] << 8) |
                         ((uint32_t)buffer[3]);
    if (wire_size == 0 || wire_size > MAX_HEADER_SIZE) return -1;

    if (deserialize_header(buffer, buffer_size, header) < 0) return -1;

    size_t header_len = HEADER_SIZE + (size_t)wire_size;
    if (header_len > buffer_size) return -1;

    if (header->file_size < 0) return -1;
    size_t chunk_size = (size_t)header->file_size;
    if (chunk_size > max_chunk_size) return -1;
    if (header_len + chunk_size > buffer_size) return -1;
    
    memcpy(chunk_data, buffer + header_len, chunk_size);
    return (int)chunk_size;
}
