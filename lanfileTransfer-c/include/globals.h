#ifndef GLOBALS_H
#define GLOBALS_H

#include "discovery.h"
#include "transfer.h"

/* 全局管理器实例 */
extern DiscoveryManager g_discovery;
extern TransferManager g_transfer;
extern volatile int g_running;

#endif /* GLOBALS_H */
