#ifndef DISCOVERY_H
#define DISCOVERY_H

#include <stdint.h>
#include <stddef.h>
#include <pthread.h>
#include <stdbool.h>
#include <time.h>

#include "protocol.h"

#define MAX_PEERS 100
#define PEER_ID_LEN 64
#define DEVICE_NAME_LEN 128
#define IP_LEN 46  /* IPv6 max */
#define MAX_BROADCAST_INTERVAL 60
#define PEER_TIMEOUT_SEC 30
#define PEER_CLEANUP_SEC 300

/* Peer information */
typedef struct {
    char id[PEER_ID_LEN];
    char name[DEVICE_NAME_LEN];
    char ip[IP_LEN];
    int port;
    bool online;
    time_t last_seen;
} Peer;

/* Discovery manager */
typedef struct {
    int udp_socket;
    char device_id[PEER_ID_LEN];
    char device_name[DEVICE_NAME_LEN];
    int listen_port;
    int discovery_interval;
    
    Peer peers[MAX_PEERS];
    int peer_count;
    
    pthread_t udp_thread;
    pthread_t broadcast_thread;
    pthread_t cleanup_thread;
    
    volatile bool running;
    volatile bool started;
    
    pthread_mutex_t mutex;
    
    /* Callbacks */
    void (*on_peer_found)(Peer *peer);
    void (*on_peer_lost)(const char *peer_id);
    void (*on_control_msg)(const char *type, ControlMessage *msg);
} DiscoveryManager;

/* Initialize discovery manager */
int discovery_init(DiscoveryManager *mgr, 
                  const char *device_id,
                  const char *device_name,
                  int listen_port,
                  int discovery_interval);

/* Start discovery service */
int discovery_start(DiscoveryManager *mgr);

/* Stop discovery service */
void discovery_stop(DiscoveryManager *mgr);

/* Broadcast presence (send discovery message) */
int discovery_broadcast_presence(DiscoveryManager *mgr);

/* Send chat message to a peer */
int discovery_send_message(DiscoveryManager *mgr, const char *target_id, const char *message);

/* Send transfer request */
int discovery_send_transfer_request(DiscoveryManager *mgr, 
                                   const char *transfer_id,
                                   const char *file_name,
                                   int64_t file_size);

/* Send transfer control command */
int discovery_send_control(DiscoveryManager *mgr, 
                          const char *transfer_id,
                          const char *command);

/* Send system command (shutdown/restart) to a specific peer */
int discovery_send_system_command(DiscoveryManager *mgr,
                                 const char *peer_id,
                                 const char *command);

/* Get all peers */
int discovery_get_peers(DiscoveryManager *mgr, Peer **peers, int *count);

/* Get peer by ID */
Peer* discovery_get_peer(DiscoveryManager *mgr, const char *peer_id);

/* Refresh peers (broadcast and wait for responses) */
int discovery_refresh_peers(DiscoveryManager *mgr);

/* Set callbacks */
void discovery_set_on_peer_found(DiscoveryManager *mgr, void (*callback)(Peer *));
void discovery_set_on_peer_lost(DiscoveryManager *mgr, void (*callback)(const char *));
void discovery_set_on_control_msg(DiscoveryManager *mgr, void (*callback)(const char *, ControlMessage *));

/* Get local peer info */
void discovery_get_local_peer(DiscoveryManager *mgr, Peer *peer);

#endif /* DISCOVERY_H */
