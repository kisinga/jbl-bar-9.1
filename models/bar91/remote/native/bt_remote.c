/* SPDX-License-Identifier: MIT
 * Copyright (c) 2026 kisinga
 * Fixed, callback-free client for the owner's native MediaTek Bluetooth RPC.
 * No controller/stack setup, notification subscriptions, power, or pairing calls.
 * --check connects and closes only. Media actions require explicit arguments.
 */
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

/* Proven native ARM32 ABIs; init's configuration argument is always NULL. */
extern int bt_rpc_init(const void *configuration);
extern int c_rpc_start_mtk_bt_service_client(void);
extern int bt_rpc_close_client(int connection);
extern int a_mtkapi_avrcp_send_passthrough_cmd(const char *address, int command, int key_state);

static int valid_address(const char *p) {
    if (strlen(p) != 17) return 0;
    for (int i=0; i<17; ++i) {
        unsigned char c=(unsigned char)p[i];
        if (i%3==2) { if (c != ':') return 0; }
        else if (!((c>='0' && c<='9') || (c>='a' && c<='f') || (c>='A' && c<='F'))) return 0;
    }
    return 1;
}
static int action_id(const char *s) {
    if (!strcmp(s,"play")) return 0;
    if (!strcmp(s,"pause")) return 1;
    if (!strcmp(s,"next")) return 2;
    if (!strcmp(s,"previous")) return 3;
    if (!strcmp(s,"stop")) return 6;
    return -1;
}
static void report(int out, int initialized, int connected, int closed, int press, int release, int check) {
    char b[320];
    int n=snprintf(b,sizeof(b),"{\"check_only\":%s,\"init_result\":%d,\"connected\":%s,\"close_result\":%d,\"press_result\":%d,\"release_result\":%d,\"callbacks_registered\":false}\n",check?"true":"false",initialized,connected?"true":"false",closed,press,release);
    if (n>0 && (size_t)n<sizeof(b)) {
        size_t done=0;
        while (done<(size_t)n) {
            ssize_t k=write(out,b+done,(size_t)n-done);
            if (k<0 && errno==EINTR) continue;
            if (k<=0) break;
            done+=(size_t)k;
        }
    }
}
int main(int argc,char **argv) {
    int check=(argc==2 && !strcmp(argv[1],"--check"));
    int command=-1;
    if (!check && (argc!=3 || (command=action_id(argv[1]))<0 || !valid_address(argv[2]))) {
        static const char usage[]="Usage: bt-remote --check | {play|pause|stop|next|previous} PEER_MAC\n";
        write(STDERR_FILENO,usage,sizeof(usage)-1);
        return 64;
    }
    /* Keep native printf diagnostics out of the one-line JSON result. */
    int out=dup(STDOUT_FILENO);
    if (out<0 || dup2(STDERR_FILENO,STDOUT_FILENO)<0) return 70;
    alarm(8);
    int init=bt_rpc_init(NULL);
    if (init!=0) { report(out,init,0,-1,-1,-1,check); return 71; }
    int connection=c_rpc_start_mtk_bt_service_client();
    if (connection<=0) { report(out,init,0,-1,-1,-1,check); return 72; }
    int press=0,release=0;
    if (!check) {
        press=a_mtkapi_avrcp_send_passthrough_cmd(argv[2],command,0);
        /* Always attempt key-up, including an uncertain press failure. No replay. */
        release=a_mtkapi_avrcp_send_passthrough_cmd(argv[2],command,1);
    }
    int closed=bt_rpc_close_client(connection);
    report(out,init,1,closed,press,release,check);
    alarm(0);
    close(out);
    return (press==0 && release==0 && closed==0)?0:73;
}
