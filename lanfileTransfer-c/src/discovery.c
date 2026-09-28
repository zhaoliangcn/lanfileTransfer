#include "discovery.h"
#include "protocol.h"
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <errno.h>
#include <signal.h>
#include <ifaddrs.h>
#include <net/if.h>

#define BROADCAST_PORT 9876
#define UDP_BUFFER_SIZE 8192
#define BROADCAST_ADDR "255.255.255.255"

/* Get local IP address */
static int get_local_ip(char *ip_buffer, size_t buffer_size) {
    int sock = socket(AF_INET, SOCK_DGRAM, 0);
    if (sock < 0) return -1;
    
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = inet_addr("8.8.8.8");
    addr.sin_port = htons(53);
    
    if (connect(sock, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        close(sock);
        return -1;
    }
    
    struct sockaddr_in local;
    socklen_t len = sizeof(local);
    if (getsockname(sock, (struct sockaddr*)&local, &len) < 0) {
        close(sock);
        return -1;
    }
    
    inet_ntop(AF_INET, &local.sin_addr, ip_buffer, buffer_size);
    close(sock);
    return 0;
}

/* Handle incoming UDP message */
static void handle_udp_message(DiscoveryManager *mgr, unsigned char *data, size_t len, 
                              struct sockaddr_in *sender_addr) {
    ControlMessage msg;
    if (deserialize_control_message((char*)data, &msg) < 0) {
        fprintf(stderr, "[UDP] 忽略: 消息解析失败 (len=%zu)\n", len);
        return;
    }

    char sender_ip[INET_ADDRSTRLEN];
    inet_ntop(AF_INET, &sender_addr->sin_addr, sender_ip, sizeof(sender_ip));
    fprintf(stderr, "[UDP] 收到 type=%s from=%s deviceId=%s targetId=%s\n",
            msg.type, sender_ip, msg.device_id, msg.target_id);
    
    pthread_mutex_lock(&mgr->mutex);
    
    /* Ignore own messages */
    if (strcmp(msg.device_id, mgr->device_id) == 0) {
        fprintf(stderr, "[UDP] 忽略: 自己的消息 (deviceId=%s)\n", msg.device_id);
        pthread_mutex_unlock(&mgr->mutex);
        return;
    }
    
    /* Handle discovery message */
    if (strcmp(msg.type, CONTROL_TYPE_DISCOVERY) == 0) {
        time_t now = time(NULL);
        bool found_new = false;
        
        /* Find or create peer */
        Peer *peer = NULL;
        for (int i = 0; i < mgr->peer_count; i++) {
            if (strcmp(mgr->peers[i].id, msg.device_id) == 0) {
                peer = &mgr->peers[i];
                break;
            }
        }
        
        if (!peer) {
            if (mgr->peer_count < MAX_PEERS) {
                peer = &mgr->peers[mgr->peer_count++];
                memset(peer, 0, sizeof(Peer));
                found_new = true;
            } else {
                pthread_mutex_unlock(&mgr->mutex);
                return;
            }
        }
        
        /* Update peer info */
        strncpy(peer->id, msg.device_id, sizeof(peer->id) - 1);
        strncpy(peer->name, msg.device_name, sizeof(peer->name) - 1);
        peer->port = msg.port;
        peer->last_seen = now;
        peer->online = true;
        
        /* Get IP from sender address */
        inet_ntop(AF_INET, &sender_addr->sin_addr, peer->ip, sizeof(peer->ip));
        
        pthread_mutex_unlock(&mgr->mutex);
        
        /* Callback for new peer */
        if (found_new && mgr->on_peer_found) {
            mgr->on_peer_found(peer);
        }
    } else if (strcmp(msg.type, CONTROL_TYPE_CHAT_MESSAGE) == 0) {
    /* Go 兼容：聊天消息只发给匹配 target_id 的设备 */
    pthread_mutex_unlock(&mgr->mutex);
    if (msg.target_id[0] && strcmp(msg.target_id, mgr->device_id) == 0) {
        fprintf(stderr, "[UDP] chat_message 匹配 targetId=%s，回调\n", msg.target_id);
        if (mgr->on_control_msg) {
            mgr->on_control_msg(msg.type, &msg);
        }
    } else {
        fprintf(stderr, "[UDP] chat_message 忽略: targetId=%s 不匹配本机 %s\n",
                msg.target_id, mgr->device_id);
    }
} else if (mgr->on_control_msg) {
    pthread_mutex_unlock(&mgr->mutex);
    fprintf(stderr, "[UDP] type=%s 转发到 on_control_msg\n", msg.type);
    mgr->on_control_msg(msg.type, &msg);
} else {
    fprintf(stderr, "[UDP] type=%s 无回调处理\n", msg.type);
    pthread_mutex_unlock(&mgr->mutex);
}
}

/* UDP receive thread */
static void* udp_receive_loop(void *arg) {
    DiscoveryManager *mgr = (DiscoveryManager*)arg;
    unsigned char buffer[UDP_BUFFER_SIZE];
    
    while (mgr->running) {
        struct sockaddr_in sender_addr;
        socklen_t addr_len = sizeof(sender_addr);
        
        /* Set timeout */
        struct timeval tv;
        tv.tv_sec = 1;
        tv.tv_usec = 0;
        setsockopt(mgr->udp_socket, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
        
        int len = recvfrom(mgr->udp_socket, buffer, sizeof(buffer), 0, 
                          (struct sockaddr*)&sender_addr, &addr_len);
        
        if (len > 0) {
            /* recvfrom() does not append a NUL terminator, but the control
             * message parser strdup()s this buffer and walks it as a C string.
             * Without this the parse runs past the datagram into adjacent stack
             * memory, which then gets surfaced as the peer's device name. */
            if ((size_t)len >= sizeof(buffer)) len = (int)sizeof(buffer) - 1;
            buffer[len] = '\0';

            handle_udp_message(mgr, buffer, len, &sender_addr);
        } else if (errno != EAGAIN && errno != EWOULDBLOCK) {
            if (!mgr->running) break;
        }
    }
    
    return NULL;
}

/* Broadcast presence thread */
static void* broadcast_loop(void *arg) {
    DiscoveryManager *mgr = (DiscoveryManager*)arg;
    int remaining = mgr->discovery_interval;
    
    while (mgr->running) {
        sleep(1);
        remaining--;
        
        if (!mgr->running) break;
        
        if (remaining <= 0) {
            discovery_broadcast_presence(mgr);
            remaining = mgr->discovery_interval;
        }
    }
    
    return NULL;
}

/* Cleanup offline peers thread - Go 兼容：每10秒检查一次 */
static void* cleanup_loop(void *arg) {
    DiscoveryManager *mgr = (DiscoveryManager*)arg;
    int countdown = 10;
    
    while (mgr->running) {
        sleep(1);
        countdown--;
        
        if (!mgr->running) break;
        
        if (countdown > 0) continue;
        countdown = 10;
        
        pthread_mutex_lock(&mgr->mutex);
        time_t now = time(NULL);
        
        for (int i = 0; i < mgr->peer_count; i++) {
            time_t elapsed = now - mgr->peers[i].last_seen;
            
            if (elapsed > PEER_TIMEOUT_SEC) {
                if (mgr->peers[i].online) {
                    mgr->peers[i].online = false;
                    pthread_mutex_unlock(&mgr->mutex);
                    
                    if (mgr->on_peer_lost) {
                        mgr->on_peer_lost(mgr->peers[i].id);
                    }
                    
                    pthread_mutex_lock(&mgr->mutex);
                }
            }
            
            /* Remove peers not seen for 5 minutes */
            if (elapsed > PEER_CLEANUP_SEC) {
                /* Move last peer to this position and decrease count */
                mgr->peers[i] = mgr->peers[mgr->peer_count - 1];
                mgr->peer_count--;
                i--;
            }
        }
        
        pthread_mutex_unlock(&mgr->mutex);
    }
    
    return NULL;
}

int discovery_init(DiscoveryManager *mgr, 
                  const char *device_id,
                  const char *device_name,
                  int listen_port,
                  int discovery_interval) {
    memset(mgr, 0, sizeof(DiscoveryManager));
    
    strncpy(mgr->device_id, device_id, sizeof(mgr->device_id) - 1);
    strncpy(mgr->device_name, device_name, sizeof(mgr->device_name) - 1);
    mgr->listen_port = listen_port;
    mgr->discovery_interval = discovery_interval;
    
    if (mgr->discovery_interval <= 0 || mgr->discovery_interval > MAX_BROADCAST_INTERVAL) {
        mgr->discovery_interval = 5;
    }
    
    pthread_mutex_init(&mgr->mutex, NULL);
    
    return 0;
}

int discovery_start(DiscoveryManager *mgr) {
    if (mgr->started) return -1;
    
    /* Create UDP socket */
    mgr->udp_socket = socket(AF_INET, SOCK_DGRAM, 0);
    if (mgr->udp_socket < 0) {
        perror("socket");
        return -1;
    }
    
    /* Allow broadcast */
    int broadcast = 1;
    if (setsockopt(mgr->udp_socket, SOL_SOCKET, SO_BROADCAST, &broadcast, sizeof(broadcast)) < 0) {
        perror("setsockopt SO_BROADCAST");
        close(mgr->udp_socket);
        return -1;
    }
    
    /* Bind to port */
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    addr.sin_port = htons(mgr->listen_port);
    
    if (bind(mgr->udp_socket, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        perror("bind");
        close(mgr->udp_socket);
        return -1;
    }
    
    mgr->running = true;
    mgr->started = true;
    
    /* Start threads */
    if (pthread_create(&mgr->udp_thread, NULL, udp_receive_loop, mgr) != 0) {
        perror("pthread_create udp");
        discovery_stop(mgr);
        return -1;
    }
    
    if (pthread_create(&mgr->broadcast_thread, NULL, broadcast_loop, mgr) != 0) {
        perror("pthread_create broadcast");
        discovery_stop(mgr);
        return -1;
    }
    
    if (pthread_create(&mgr->cleanup_thread, NULL, cleanup_loop, mgr) != 0) {
        perror("pthread_create cleanup");
        discovery_stop(mgr);
        return -1;
    }
    
    /* Send initial broadcast */
    discovery_broadcast_presence(mgr);
    
    return 0;
}

void discovery_stop(DiscoveryManager *mgr) {
    if (!mgr->started) return;

    mgr->running = false;

    /* 原实现是 sendto(mgr->udp_socket, dummy, 1, 0, NULL, 0) —— to 为 NULL、
     * addrlen 为 0 是非法调用（返回 EDESTADDRREQ），并没有真正唤醒任何线程，
     * 接收循环只是靠 SO_RCVTIMEO 一秒一秒地轮询出来。
     * 改为向回环地址发一个真实报文，立刻唤醒阻塞中的 recvfrom。 */
    if (mgr->udp_socket >= 0) {
        struct sockaddr_in wake;
        char dummy = 0;
        memset(&wake, 0, sizeof(wake));
        wake.sin_family = AF_INET;
        wake.sin_port = htons(mgr->listen_port);
        wake.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
        sendto(mgr->udp_socket, &dummy, 1, 0,
               (struct sockaddr *)&wake, sizeof(wake));
    }

    /* Wait for threads. broadcast/cleanup loops poll `running` between sleeps,
     * so cap the wait rather than relying on their current sleep interval. */
    pthread_join(mgr->udp_thread, NULL);
    pthread_join(mgr->broadcast_thread, NULL);
    pthread_join(mgr->cleanup_thread, NULL);

    if (mgr->udp_socket >= 0) {
        close(mgr->udp_socket);
        mgr->udp_socket = -1;
    }
    mgr->started = false;

    pthread_mutex_destroy(&mgr->mutex);
}

int discovery_broadcast_presence(DiscoveryManager *mgr) {
    ControlMessage msg;
    init_control_message(&msg, CONTROL_TYPE_DISCOVERY, mgr->device_id, mgr->device_name, mgr->listen_port);

    char json[CONTROL_MSG_SIZE];
    int len = serialize_control_message(&msg, json, sizeof(json));
    if (len < 0) return -1;

    /* 发送到全局广播地址 255.255.255.255 */
    struct sockaddr_in broadcast_addr;
    memset(&broadcast_addr, 0, sizeof(broadcast_addr));
    broadcast_addr.sin_family = AF_INET;
    broadcast_addr.sin_addr.s_addr = inet_addr("255.255.255.255");
    broadcast_addr.sin_port = htons(mgr->listen_port);

    sendto(mgr->udp_socket, json, len, 0,
           (struct sockaddr*)&broadcast_addr, sizeof(broadcast_addr));

    /* Go 兼容：发送到每个网卡的广播地址 */
    struct ifaddrs *ifaddr, *ifa;
    if (getifaddrs(&ifaddr) == 0) {
        for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next) {
            if (ifa->ifa_addr == NULL || ifa->ifa_addr->sa_family != AF_INET) continue;
            if (!(ifa->ifa_flags & 0x1) || !(ifa->ifa_flags & 0x2)) continue;
            if (ifa->ifa_broadaddr == NULL) continue;

            struct sockaddr_in *baddr = (struct sockaddr_in *)ifa->ifa_broadaddr;
            baddr->sin_port = htons(mgr->listen_port);

            sendto(mgr->udp_socket, json, len, 0,
                   (struct sockaddr*)baddr, sizeof(*baddr));
        }
        freeifaddrs(ifaddr);
    }

    return 0;
}

int discovery_send_message(DiscoveryManager *mgr, const char *target_id, const char *message) {
    Peer *peer = discovery_get_peer(mgr, target_id);
    if (!peer || !peer->online) return -1;
    
    ControlMessage msg;
    init_control_message(&msg, CONTROL_TYPE_CHAT_MESSAGE, mgr->device_id, mgr->device_name, mgr->listen_port);
    strncpy(msg.target_id, target_id, sizeof(msg.target_id) - 1);
    strncpy(msg.message, message, sizeof(msg.message) - 1);
    
    char json[CONTROL_MSG_SIZE];
    int len = serialize_control_message(&msg, json, sizeof(json));
    if (len < 0) return -1;
    
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    inet_pton(AF_INET, peer->ip, &addr.sin_addr);
    addr.sin_port = htons(peer->port);
    
    return (sendto(mgr->udp_socket, json, len, 0, (struct sockaddr*)&addr, sizeof(addr)) >= 0) ? 0 : -1;
}

int discovery_send_transfer_request(DiscoveryManager *mgr, 
                                   const char *transfer_id,
                                   const char *file_name,
                                   int64_t file_size) {
    ControlMessage msg;
    init_control_message(&msg, CONTROL_TYPE_TRANSFER_REQUEST, mgr->device_id, mgr->device_name, mgr->listen_port);
    strncpy(msg.transfer_id, transfer_id, sizeof(msg.transfer_id) - 1);
    strncpy(msg.file_name, file_name, sizeof(msg.file_name) - 1);
    msg.file_size = file_size;
    
    char json[CONTROL_MSG_SIZE];
    int len = serialize_control_message(&msg, json, sizeof(json));
    if (len < 0) return -1;
    
    /* Broadcast transfer request */
    struct sockaddr_in broadcast_addr;
    memset(&broadcast_addr, 0, sizeof(broadcast_addr));
    broadcast_addr.sin_family = AF_INET;
    broadcast_addr.sin_addr.s_addr = inet_addr(BROADCAST_ADDR);
    broadcast_addr.sin_port = htons(mgr->listen_port);
    
    return (sendto(mgr->udp_socket, json, len, 0, 
                  (struct sockaddr*)&broadcast_addr, sizeof(broadcast_addr)) >= 0) ? 0 : -1;
}

int discovery_send_control(DiscoveryManager *mgr, 
                          const char *transfer_id,
                          const char *command) {
    ControlMessage msg;
    init_control_message(&msg, CONTROL_TYPE_TRANSFER_CONTROL, mgr->device_id, mgr->device_name, mgr->listen_port);
    strncpy(msg.transfer_id, transfer_id, sizeof(msg.transfer_id) - 1);
    strncpy(msg.command, command, sizeof(msg.command) - 1);
    
    char json[CONTROL_MSG_SIZE];
    int len = serialize_control_message(&msg, json, sizeof(json));
    if (len < 0) return -1;
    
    struct sockaddr_in broadcast_addr;
    memset(&broadcast_addr, 0, sizeof(broadcast_addr));
    broadcast_addr.sin_family = AF_INET;
    broadcast_addr.sin_addr.s_addr = inet_addr(BROADCAST_ADDR);
    broadcast_addr.sin_port = htons(mgr->listen_port);
    
    return (sendto(mgr->udp_socket, json, len, 0, 
                  (struct sockaddr*)&broadcast_addr, sizeof(broadcast_addr)) >= 0) ? 0 : -1;
}

int discovery_send_system_command(DiscoveryManager *mgr,
                                 const char *peer_id,
                                 const char *command) {
    ControlMessage msg;
    init_control_message(&msg, CONTROL_TYPE_SYSTEM_COMMAND, mgr->device_id, mgr->device_name, mgr->listen_port);
    strncpy(msg.command, command, sizeof(msg.command) - 1);
    strncpy(msg.target_id, peer_id, sizeof(msg.target_id) - 1);

    /* Find peer to get IP address */
    Peer *peer = discovery_get_peer(mgr, peer_id);
    if (!peer) {
        return -1;
    }

    char json[CONTROL_MSG_SIZE];
    int len = serialize_control_message(&msg, json, sizeof(json));
    if (len < 0) return -1;

    struct sockaddr_in target_addr;
    memset(&target_addr, 0, sizeof(target_addr));
    target_addr.sin_family = AF_INET;
    inet_pton(AF_INET, peer->ip, &target_addr.sin_addr);
    target_addr.sin_port = htons(peer->port);

    return (sendto(mgr->udp_socket, json, len, 0,
                  (struct sockaddr*)&target_addr, sizeof(target_addr)) >= 0) ? 0 : -1;
}

int discovery_get_peers(DiscoveryManager *mgr, Peer **peers, int *count) {
    pthread_mutex_lock(&mgr->mutex);
    
    *count = mgr->peer_count;
    *peers = mgr->peers;
    
    pthread_mutex_unlock(&mgr->mutex);
    return 0;
}

Peer* discovery_get_peer(DiscoveryManager *mgr, const char *peer_id) {
    pthread_mutex_lock(&mgr->mutex);
    
    for (int i = 0; i < mgr->peer_count; i++) {
        if (strcmp(mgr->peers[i].id, peer_id) == 0) {
            pthread_mutex_unlock(&mgr->mutex);
            return &mgr->peers[i];
        }
    }
    
    pthread_mutex_unlock(&mgr->mutex);
    return NULL;
}

int discovery_refresh_peers(DiscoveryManager *mgr) {
    discovery_broadcast_presence(mgr);
    sleep(1);  /* Wait for responses */
    return 0;
}

void discovery_set_on_peer_found(DiscoveryManager *mgr, void (*callback)(Peer *)) {
    mgr->on_peer_found = callback;
}

void discovery_set_on_peer_lost(DiscoveryManager *mgr, void (*callback)(const char *)) {
    mgr->on_peer_lost = callback;
}

void discovery_set_on_control_msg(DiscoveryManager *mgr, void (*callback)(const char *, ControlMessage *)) {
    mgr->on_control_msg = callback;
}

void discovery_get_local_peer(DiscoveryManager *mgr, Peer *peer) {
    memset(peer, 0, sizeof(Peer));
    strncpy(peer->id, mgr->device_id, sizeof(peer->id) - 1);
    strncpy(peer->name, mgr->device_name, sizeof(peer->name) - 1);
    peer->port = mgr->listen_port;
    peer->online = true;
    
    char ip[IP_LEN];
    if (get_local_ip(ip, sizeof(ip)) == 0) {
        strncpy(peer->ip, ip, sizeof(peer->ip) - 1);
    }
    
    peer->last_seen = time(NULL);
}
