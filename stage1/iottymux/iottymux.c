// Copyright 2016 The rkt Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>

// fcntl
#include <unistd.h>
#include <fcntl.h>

// mkdir
#include <sys/stat.h>
#include <sys/types.h>

// socket
#include <sys/socket.h>
#include <arpa/inet.h>

// pty
#include <stdlib.h>
#include <fcntl.h>

// tty
#include <termios.h>

// isprint
#include <ctype.h>

// mount
#include <sys/mount.h>

// sd_notify
#include <systemd/sd-daemon.h>

// sd_journal
#include <systemd/sd-journal.h>

// sd event loop
#include <systemd/sd-event.h>
#include <time.h>

static inline void freep(void *p) {
  free(*(void**) p);
}
#define _cleanup_(x) __attribute__((cleanup(x)))
#define _cleanup_free_ _cleanup_(freep)

static int exit_err;
#define exit_if(_cond, _fmt, _args...)              \
  exit_err++;                                       \
  if(_cond) {                                       \
    fprintf(stderr, "Error: " _fmt "\n", ##_args);  \
    exit(exit_err);                                 \
  }

#define pexit_if(_cond, _fmt, _args...)             \
  exit_if(_cond, _fmt ": %s", ##_args, strerror(errno))

#define sdexit_if(_errorno, _fmt, _args...)         \
  exit_if(_errorno < 0, _fmt ": %s", ##_args, strerror(_errorno))


#define TTYDIR "/rkt/tty/"

//TODO(lucab): stacksize/malloc story
#define PTSNAMELEN 512

// ICANON mode
#define BUFLEN 4096
static char in[BUFLEN] = {};
static sd_event *ev = NULL;

#define MAXCLIENTS 3

static struct clients_t {
  int *fd[MAXCLIENTS];
  int len;
} clients = {};

static int tty_out_handler(sd_event_source *src, int fd, uint32_t revents, void *ud) {
  int pty = *((int *) ud);
  int inlen = read(pty, in, BUFLEN-1);
  if (inlen > 0) {
    int i;
    for (i=0; i < clients.len; i++) {
          write(*(clients.fd[i]), in, inlen);
    }
    in[inlen+1] = '\0';
    puts(in);
    /*
    if (in[inlen] != '\n') {
      inlen += putchar('\n');
      fflush(stdout);
    }
    */
  }
  return abs(inlen);
}

// TODO: error handling
static int tcp_handler(sd_event_source *src, int sock, uint32_t revents, void *ud) {
  int pty = *((int *) ud);
  int outlen = 0;
  printf("reading data from client\n");
  int inlen = read(sock, in, BUFLEN-1);
  if (inlen > 0) {
    outlen = write(pty, in, inlen);
  }
  return abs(outlen);
}

// TODO: error handling
static int sock_handler(sd_event_source *src, int sock, uint32_t revents, void *ud) {
  sd_event_source *es = NULL;
  struct sockaddr_in *ca = malloc(sizeof(struct sockaddr_in));
  socklen_t calen = sizeof(struct sockaddr_in);
  bzero(&ca, sizeof(struct sockaddr_in));
  // TODO: cleaning
  int *conn = malloc(sizeof(int));
  *conn = accept(sock, (struct sockaddr *) &ca, &calen);
  if (*conn != -1){
    if (clients.len < MAXCLIENTS) {
      sd_journal_print(LOG_ERR, "Accepted client");
      fcntl(*conn, fcntl(*conn,F_GETFL)|O_NONBLOCK);
      clients.fd[clients.len++] = conn;
      sdexit_if(sd_event_add_io(ev, &es, *conn, EPOLLIN, tcp_handler, ud), "failed to add tcp handler");
    } else {
      close(*conn);
      free(conn);
      sd_journal_print(LOG_ERR, "Too many connected clients, maximum allowed %d", MAXCLIENTS);
    }
  } else {
    sd_journal_print(LOG_ERR, "Failed to accept client");
  }
  return abs(*conn);
}

int main(int argc, char *argv[])
{
  setvbuf (stdout, NULL, _IONBF, 0);
  _cleanup_free_ char* tty_stage1, *dir_stage2, *tty_stage2;
  _cleanup_free_ char* pts_name = malloc(PTSNAMELEN);
  exit_if(pts_name == NULL, "failed to allocate memory for pts name");
  char* app_name = getenv("RKT_APPNAME");
  exit_if(app_name == NULL, "failed to retrieve app name");

  // Setup pty
  struct termios tio;
  int pty = posix_openpt(O_RDWR|O_NOCTTY|O_CLOEXEC|O_NONBLOCK);
  pexit_if(pty == -1, "failed to create pty");
  pexit_if(tcgetattr(pty, &tio) == -1, "failed to set terminal");
  tio.c_lflag &= ~ECHO;
  pexit_if(tcsetattr(pty, TCSANOW, &tio) == -1, "failed to set terminal mode");
  pexit_if(ptsname_r(pty, pts_name, (size_t) PTSNAMELEN) != 0, "failed to get pty slave name");
  pexit_if(grantpt(pty) != 0, "failed to grant pty slave");
  pexit_if(unlockpt(pty) != 0, "failed to unlock pty slave");

  // TODO try using FD_STORE?

  // Bind-mount pts to stage1
  // TODO: conditional mkdir
  int r = asprintf(&tty_stage1, "%s%s", TTYDIR, app_name);
  exit_if(r == -1, "failed to assemble stage1 pts");
  pexit_if(mkdir(TTYDIR, 0755) != 0, "failed to create stage1 tty directory");
  pexit_if(creat(tty_stage1, 0600) == -1, "failed to create stage1 tty %s", tty_stage1);
  pexit_if(mount(pts_name, tty_stage1, NULL, MS_BIND, NULL) != 0, "failed to bindmount pts in stage1");

  // Bind-mount pts to stage2
  // TODO: move to prepare-app
  // TODO: move to /dev/console?
  r = asprintf(&dir_stage2, "/opt/stage2/%s/rootfs/dev/iottymux", app_name);
  exit_if(r == -1, "failed to assemble tty stage1 directory");
  pexit_if(mkdir(dir_stage2, 0755) != 0, "failed to create tty directory");
  r = asprintf(&tty_stage2, "%s/app", dir_stage2);
  exit_if(r == -1, "failed to assemble stage2 tty");
  pexit_if(creat(tty_stage2, 0600) == -1, "failed to create stage2 tty %s", tty_stage2);
  mount(pts_name, tty_stage2, NULL, MS_BIND, NULL);

  // Create socket for remote tty attaching by stage0
  struct sockaddr_in sa;
  bzero(&sa, sizeof(sa));
  sa.sin_family = AF_INET;
  sa.sin_port = 0;
  sa.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
  // TODO: move to AF_VSOCK
  int sock_stage0_tty = socket(AF_INET, SOCK_STREAM|SOCK_NONBLOCK|SOCK_CLOEXEC, 0);
  pexit_if(sock_stage0_tty == -1, "failed to create socket");
  pexit_if(bind(sock_stage0_tty, (struct sockaddr *) &sa, sizeof(sa)) == -1, "failed to bind socket");
  pexit_if(listen(sock_stage0_tty, SOMAXCONN) == -1, "failed to listen on socket");

  // Setup systemd event loop
  sd_event_source *es_ttyout = NULL, *es_sock = NULL;
  sdexit_if(sd_event_default(&ev), "unable to initialize systemd event loop");
  sdexit_if(sd_event_add_io(ev, &es_ttyout, pty, EPOLLIN, tty_out_handler, &pty), "failed to add tty handler");
  sdexit_if(sd_event_source_set_priority(es_ttyout, SD_EVENT_PRIORITY_IMPORTANT), "failed to set tty handler priority");
  sdexit_if(sd_event_add_io(ev, &es_sock, sock_stage0_tty, EPOLLIN, sock_handler, &pty), "failed to add socket handler");

  // Notify readiness, trigger app start
  sd_notify(false, "READY=1");
  sd_journal_print(LOG_DEBUG, "===== Starting log for %s =====", app_name);
  return sd_event_loop(ev);
}
