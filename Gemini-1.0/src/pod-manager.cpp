/**
 * Copyright 2020 Hung-Hsin Chen, LSA Lab, National Tsing Hua University
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * From Kubernetes concepts: A Pod is the basic execution unit of a Kubernetes application–the
 * smallest and simplest unit in the Kubernetes object model that you create or deploy. A Pod
 * represents processes running on your cluster.
 *
 * This manager will run like a daemon in each Pod. User program will interact with this manager
 * when they call certain CUDA-related functions.
 */

#include <arpa/inet.h>
#include <execinfo.h>
#include <pthread.h>
#include <signal.h>
#include <sys/socket.h>
#include <unistd.h>
// #include <fcntl.h> 
#include <string>
#include <cassert>
#include <cerrno>
#include <chrono>
#include <climits>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <functional>
#include <map>
#include <queue>
#include <iostream>
#include <fstream>
#include "comm.h"
#include "debug.h"
#include "util.h"
std::ofstream myfile ("/tmp/pod.txt");
using std::chrono::duration_cast;
using std::chrono::microseconds;
using std::chrono::steady_clock;
using std::chrono::time_point;
using std::string;
// connection information, below are default values
// can be changed by environment vairables
char SCHEDULER_IP[20] = "127.0.0.1";
uint16_t SCHEDULER_PORT = 50051;
uint16_t POD_SERVER_PORT = 50052;

void sig_handler(int);

// thread interact with scheduler
void *scheduler_thread_send_func(void *sockfd);
void *scheduler_thread_recv_func(void *sockfd);
// service thread for each hook library
void *hook_thread_func(void *sockfd);

/* communication between scheduler thread and hook threads */
enum actions {
  KERNEL_LAUNCH,
};
struct request {
  reqid_t req_id;
  char *data;
};
std::queue<request> request_queue;
uint32_t req_cnt = 0;
pthread_mutex_t req_queue_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t req_queue_cond = PTHREAD_COND_INITIALIZER;

struct response {
  void *data;
};
std::map<reqid_t, response> response_map;
pthread_mutex_t rsp_map_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t rsp_map_cond = PTHREAD_COND_INITIALIZER;

/* global variables to store memory limit */
size_t gpu_mem_limit = 0, gpu_mem_used = 0;
std::map<int, size_t> allocation_map;  // memory usage of each connection
pthread_mutex_t mem_info_mutex = PTHREAD_MUTEX_INITIALIZER;

/* computation utilization */
typedef time_point<steady_clock> quota_tp;
double pod_overuse_ms = 0.0;
std::map<int, double> client_burst_map;
pthread_mutex_t client_stat_mutex = PTHREAD_MUTEX_INITIALIZER;
double pod_quota = 0.0;
quota_tp quota_updated_tp;
int quota_state = 0;  // 0 means usual state, 1 means someone is updating quota
pthread_mutex_t quota_state_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t quota_state_cond = PTHREAD_COND_INITIALIZER;

/* quota_state sync*/
int kernel_launch_count = 0, sleeping_count = 0;
pthread_mutex_t kernel_launch_count_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_mutex_t sleeping_count_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t sleeping_count_cond = PTHREAD_COND_INITIALIZER;

/*scheduler recv signal sync*/
int scheduler_recv_sync = 0;
pthread_mutex_t scheduler_recv_sync_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t scheduler_recv_sync_cond = PTHREAD_COND_INITIALIZER;


/* communication with scheduler */
size_t pod_name_len;
char pod_name[HOST_NAME_MAX];

// retrieve memory limit information from scheduler
int retrieve_mem_info(int sockfd, const int MAX_RETRY, const long RETRY_TIMEOUT) {
  int rc;
  char sbuf[REQ_MSG_LEN], rbuf[RSP_MSG_LEN], *attached;
  size_t pos = 0;

  // set socket timeout option
  struct timeval tv = {RETRY_TIMEOUT, 0};
  setsockopt(sockfd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));

  bzero(sbuf, REQ_MSG_LEN);
  prepare_request(sbuf, REQ_MEM_LIMIT);

  rc = multiple_attempt(
      [&]() -> int {
        if (send(sockfd, &sbuf, REQ_MSG_LEN, 0) == -1) return -1;
        if (recv(sockfd, rbuf, RSP_MSG_LEN, 0) == -1) return -1;
        return 0;
      },
      MAX_RETRY, 0);

  // disable timeout option
  tv.tv_sec = 0;
  setsockopt(sockfd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));

  if (rc != 0) return rc;  // failed to get memory info from scheduler

  // parse received data and get memory limit
  attached = parse_response(rbuf, nullptr);
  gpu_mem_used = get_msg_data<size_t>(attached, pos);  // should be 0
  gpu_mem_limit = get_msg_data<size_t>(attached, pos);
  assert(gpu_mem_used == (size_t)0);
  INFO("GPU memory limit: %lu bytes.", gpu_mem_limit);
  return 0;
}

int main(int argc, char *argv[]) {
  const int NET_OP_MAX_ATTEMPT = 5;  // maximum time retrying failed network operations
  const int NET_OP_RETRY_INTV = 10;  // seconds between two retries
  int rc;

  // for debugging
  signal(SIGSEGV, sig_handler);

  // use host name as Pod name
  char *name = getenv("POD_NAME");
  myfile<<"pod name: "<<name<<std::endl;
  if (name != NULL) {
    strcpy(pod_name, name);
  } else {
    gethostname(pod_name, HOST_NAME_MAX);
  }
  pod_name_len = strlen(pod_name);

  /* get connection information from environment variable */
  // Pod server
  char *pod_server_port_str = getenv("POD_MANAGER_PORT");
  if (pod_server_port_str != NULL) {
    POD_SERVER_PORT = atoi(pod_server_port_str);
  }
  INFO("Pod server port = %u.", POD_SERVER_PORT);

  // scheduler
  char *scheduler_ip_envstr = getenv("SCHEDULER_IP");
  if (scheduler_ip_envstr != NULL) {
    strcpy(SCHEDULER_IP, scheduler_ip_envstr);
  }
  char *scheduler_port_envstr = getenv("SCHEDULER_PORT");
  if (scheduler_port_envstr != NULL) {
    SCHEDULER_PORT = atoi(scheduler_port_envstr);
  }
  INFO("scheduler %s:%u", SCHEDULER_IP, SCHEDULER_PORT);

  /* establish connection with scheduler */
  // create socket
  int schd_sockfd = socket(PF_INET, SOCK_STREAM, 0);
  if (schd_sockfd == -1) {
    int err = errno;
    ERROR("failed to create socket: %s", strerror(err));
    exit(err);
  }

  // setup socket info
  struct sockaddr_in schd_info;
  bzero(&schd_info, sizeof(schd_info));
  schd_info.sin_family = AF_INET;
  schd_info.sin_addr.s_addr = inet_addr(SCHEDULER_IP);
  schd_info.sin_port = htons(SCHEDULER_PORT);

  // connect to scheduler
  rc = multiple_attempt(
      [&]() -> int {
        return connect(schd_sockfd, (struct sockaddr *)&schd_info, sizeof(schd_info));
      },
      NET_OP_MAX_ATTEMPT, NET_OP_RETRY_INTV);
  if (rc != 0) exit(rc);

  /* get memory limit for this pod */
  rc = retrieve_mem_info(schd_sockfd, NET_OP_MAX_ATTEMPT, NET_OP_RETRY_INTV);
  if (rc != 0) exit(rc);

  // initialize quota receiving time
  quota_updated_tp = steady_clock::now();

  /* accept connections from hook libraries */
  // create accept socket
  int accept_sockfd = socket(PF_INET, SOCK_STREAM, 0);
  if (accept_sockfd == -1) {
    ERROR("accept_socket == -1");
    exit(-1);
  }

  // setup accept socket info
  struct sockaddr_in server_info;
  bzero(&server_info, sizeof(server_info));
  server_info.sin_family = AF_INET;
  server_info.sin_addr.s_addr = INADDR_ANY;
  server_info.sin_port = htons(POD_SERVER_PORT);

  rc = multiple_attempt(
      [&]() -> int {
        return bind(accept_sockfd, (struct sockaddr *)&server_info, sizeof(server_info));
      },
      NET_OP_MAX_ATTEMPT, NET_OP_RETRY_INTV);
  if (rc != 0) exit(rc);
  listen(accept_sockfd, SOMAXCONN);

  // start scheduler threads
  pthread_t schd_send_tid, schd_recv_tid;
  pthread_create(&schd_send_tid, NULL, scheduler_thread_send_func, &schd_sockfd);
  pthread_create(&schd_recv_tid, NULL, scheduler_thread_recv_func, &schd_sockfd);
  pthread_detach(schd_send_tid);
  pthread_detach(schd_recv_tid);

  int client_sockfd = 0;
  struct sockaddr_in client_info;
  int addr_len = sizeof(client_info);

  // wait for incoming connections
  while ((client_sockfd =
              accept(accept_sockfd, (struct sockaddr *)&client_info, (socklen_t *)&addr_len))) {
    if (client_sockfd == -1) {
      ERROR("accept() return -1");
      break;
    }

    // create allocation accounting entry
    allocation_map.insert(std::make_pair(client_sockfd, 0));

    // create client statistics entries
    pthread_mutex_lock(&client_stat_mutex);
    client_burst_map.insert(std::make_pair(client_sockfd, 0.0));
    pthread_mutex_unlock(&client_stat_mutex);

    // create a thread for each client
    pthread_t tid;
    int *sockfd = new int;
    *sockfd = client_sockfd;
    pthread_create(&tid, NULL, hook_thread_func, (void *)sockfd);
    pthread_detach(tid);
  }

  return 0;
}

void sig_handler(int sig) {
  void *arr[10];
  size_t s = backtrace(arr, 10);
  ERROR("Received signal %d", sig);
  backtrace_symbols_fd(arr, s, STDERR_FILENO);
  exit(sig);
}

// update GPU memory usage information
int hook_update_memory_usage(size_t mem_size, int allocate, int sockfd) {
  int ok = 1;  // meets memory limit
  pthread_mutex_lock(&mem_info_mutex);
  if (allocate) {
    if (gpu_mem_used + mem_size > gpu_mem_limit) {
      ok = 0;
    } else {
      gpu_mem_used += mem_size;
      allocation_map[sockfd] += mem_size;
    }
  } else {
    gpu_mem_used -= mem_size;
    allocation_map[sockfd] -= mem_size;
  }
  DEBUG("GPU memory usage = %ld bytes.", gpu_mem_used);
  INFO("GPU memory usage = %ld bytes.", gpu_mem_used);
  pthread_mutex_unlock(&mem_info_mutex);
  return ok;
}

// handle kernel launch request, return remaining quota time (ms)
double hook_kernel_launch(int sockfd, double overuse_ms, double burst, char* client_name) {
  pthread_mutex_lock(&kernel_launch_count_mutex);
  kernel_launch_count+=1;
  INFO("[RIYACHU] %s kernel launch, # %d", client_name, kernel_launch_count);
  pthread_mutex_unlock(&kernel_launch_count_mutex);
  // wait if someone else is working with quota
  /*
  while (true) {
    pthread_mutex_lock(&quota_state_mutex);
    if (quota_state == 0) {
      pthread_mutex_unlock(&quota_state_mutex);
      break;
    } else {
      DEBUG("wait for quota operation complete.");
      pthread_cond_wait(&quota_state_cond, &quota_state_mutex);
      pthread_mutex_unlock(&quota_state_mutex);
    }
  }
 */
  pthread_mutex_lock(&quota_state_mutex);

  while(quota_state != 0) {

    pthread_mutex_lock(&sleeping_count_mutex);
    sleeping_count+=1;
    INFO("[RIYACHU] %s sleeping thread %d",client_name, sleeping_count);
    pthread_mutex_unlock(&sleeping_count_mutex);

    DEBUG("%s wait for quota operation complete. quota_state: %d", client_name, quota_state);
    INFO("%s wait for quota operation complete. quota_state: %d", client_name, quota_state);
    int id = pthread_cond_wait(&quota_state_cond, &quota_state_mutex);
    INFO("[RIYACHU] %s pthread_cond_wait(quota_state) %d", client_name, id);
    
    
  }

  pthread_mutex_unlock(&quota_state_mutex);
  // update Pod overuse time
  pod_overuse_ms = std::max(overuse_ms, pod_overuse_ms);

  // update statistics for this client
  pthread_mutex_lock(&client_stat_mutex);
  client_burst_map[sockfd] = burst;
  pthread_mutex_unlock(&client_stat_mutex);
 
  quota_tp now_tp = steady_clock::now();
  double elapsed_time = duration_cast<microseconds>(now_tp - quota_updated_tp).count() / 1e3;
  // ask scheduler for quota if we are expected to go over quota
  if (elapsed_time + burst > pod_quota) {
    /* expired, request quota from scheduler */
    char *sbuf;
    reqid_t req_id;
    bool complete = false;
    size_t rpos = 0;
    double max_burst = 0.0;

    // update quota state: updating quota
    pthread_mutex_lock(&quota_state_mutex);
    quota_state = 1;
    INFO("[RIYACHU] %s hook_kernel_launch: quota_state = 1, req_id %d", client_name, req_id);
    pthread_mutex_unlock(&quota_state_mutex);

    // calculate estimation values
    pthread_mutex_lock(&client_stat_mutex);
    for (auto x : client_burst_map) max_burst = std::max(x.second, max_burst);
    pthread_mutex_unlock(&client_stat_mutex);

    // place request into request queue
    pthread_mutex_lock(&req_queue_mutex);
    sbuf = new char[REQ_MSG_LEN];
    bzero(sbuf, REQ_MSG_LEN);
    req_id = prepare_request(sbuf, REQ_QUOTA, pod_overuse_ms, max_burst);
    request_queue.push({req_id, sbuf});
    // wake scheduler thread up  
    int ok = pthread_cond_signal(&req_queue_cond);
    INFO("[RIYACHU] %s send signal & req_queue_cond %d, req_id %d", client_name, ok, req_id);
    int check = pthread_mutex_unlock(&req_queue_mutex);
    
    INFO("[RIYACHU] %s unlock req_queue_mutex %d, req_id %d", client_name, check, req_id);

    INFO("[RIYACHU] %s before while complete %d, req_id %d", client_name, complete, req_id);
    // wait for response
    while (true) {
      pthread_mutex_lock(&rsp_map_mutex);

      INFO("[RIYACHU] %s in while wait complete %d, req_id %d", client_name, complete, req_id);

      pthread_mutex_lock(&scheduler_recv_sync_mutex);
      scheduler_recv_sync = 1;
      pthread_cond_signal(&scheduler_recv_sync_cond);
      INFO("[RIYACHU] %s scheduler_recv_sync %d, req_id %d", client_name, scheduler_recv_sync, req_id);
      pthread_mutex_unlock(&scheduler_recv_sync_mutex);

      pthread_cond_wait(&rsp_map_cond, &rsp_map_mutex);

      pthread_mutex_lock(&scheduler_recv_sync_mutex);
      scheduler_recv_sync = 0;
      INFO("[RIYACHU] %s scheduler_recv_sync after wait%d, req_id %d", client_name, scheduler_recv_sync, req_id);
      pthread_mutex_unlock(&scheduler_recv_sync_mutex);


      INFO("[RIYACHU] %s after wait complete %d, req_id %d", client_name, complete, req_id);
      if (response_map.find(req_id) != response_map.end()) {
        // request completed
        complete = true;  // exit while loop
        INFO("[RIYACHU] %s complete %d, req_id %d", client_name, complete, req_id);
        // update quota information
        pod_quota = get_msg_data<double>((char *)response_map[req_id].data, rpos);
        quota_updated_tp = steady_clock::now();
        elapsed_time = 0.0;
        pod_overuse_ms = 0.0;

        delete (double *)response_map[req_id].data;
        response_map.erase(req_id);
      }
      INFO("[RIYACHU] %s after if complete %d, req_id %d", client_name, complete,req_id);
      pthread_mutex_unlock(&rsp_map_mutex);
      if(complete) break;
  
    }
    INFO("[RIYACHU] %s after while true, %d", client_name, req_id);
    delete[] sbuf;

    // update quota state and notify threads waiting on quota state
    pthread_mutex_lock(&quota_state_mutex);

    pthread_mutex_lock(&sleeping_count_mutex);
    sleeping_count+=1;
    INFO("[RIYACHU] %s sleeping thread (special) %d, req_id %d", client_name, sleeping_count, req_id);
    pthread_mutex_unlock(&sleeping_count_mutex);

    quota_state = 0;  // usual state
    INFO("[RIYACHU] ............%s quota_state assign 0, req_id %d .........", client_name, req_id);
    
    pthread_mutex_lock(&sleeping_count_mutex);
    while(sleeping_count < kernel_launch_count){
      INFO("[RIYACHU] %s wait sleeping count >= kernel_launch_count, req_id %d", client_name, req_id);
      pthread_cond_wait(&sleeping_count_cond, &sleeping_count_mutex);
    }

    int id = pthread_cond_broadcast(&quota_state_cond);
    INFO("[RIYACHU] %s pthread_cond_broadcast %d, req_id %d", client_name, id, req_id);
    
    pthread_mutex_unlock(&sleeping_count_mutex);

    pthread_mutex_lock(&sleeping_count_mutex);
    sleeping_count = 0;
    INFO("[RIYACHU] %s sleeping thread clear %d", client_name, sleeping_count);
    pthread_mutex_unlock(&sleeping_count_mutex);
    
    pthread_mutex_unlock(&quota_state_mutex);
  }
  
  pthread_mutex_lock(&kernel_launch_count_mutex);
  kernel_launch_count -= 1;
  INFO("[RIYACHU] %s delete hook thread, remaing %d", client_name, kernel_launch_count);
  pthread_mutex_unlock(&kernel_launch_count_mutex);

  return pod_quota - elapsed_time;
}

// a thread interact with a hook library
void *hook_thread_func(void *args) {
  DEBUG("hook thread started.");
  INFO("hook thread started.");
  int sockfd = *((int *)args);
  char rbuf[REQ_MSG_LEN], sbuf[RSP_MSG_LEN];
  char  *client_name;
  // int flags = fcntl(sockfd, F_GETFL, 0);
  // if(fcntl(sockfd, F_SETFL, flags & ~O_NONBLOCK) < 0) {
  //     ERROR("fcntl() error");
  // }
  
  //bzero(rbuf, REQ_MSG_LEN);
  ssize_t rc;
  int recv_zero_times = 0;
  while (recv_zero_times <= 5) {
    if((rc = recv(sockfd, rbuf, REQ_MSG_LEN, 0)) <= 0){
      recv_zero_times++;
      INFO("[RIYACHU] %s (pod manager)hook_thread_func recv - len 0, cnt %ld", client_name, recv_zero_times);
      continue;
    } 
    comm_request_t req;
    reqid_t rid;
    size_t pos = 0;  // attached data reading position
    size_t len = 0;  // length of sending data
    
    char *attached = parse_request(rbuf, &client_name, nullptr, &rid, &req);

    bzero(sbuf, RSP_MSG_LEN);
    if (req == REQ_MEM_LIMIT) {
      // send gpu_mem_used and gpu_mem_limit to hook library
      len = prepare_response(sbuf, REQ_MEM_LIMIT, rid, gpu_mem_used, gpu_mem_limit);
      INFO("[RIYACHU] %s (pod manager)hook_thread_func recv - REQ_MEM_LIMIT, %ld", client_name, rid);
    } else if (req == REQ_MEM_UPDATE) {
      // update memory usage
      size_t mem_size = get_msg_data<size_t>(attached, pos);
      int allocate = get_msg_data<int>(attached, pos);
      int ok = hook_update_memory_usage(mem_size, allocate, sockfd);
      len = prepare_response(sbuf, REQ_MEM_UPDATE, rid, ok);
      INFO("[RIYACHU] %s (pod manager)hook_thread_func recv - REQ_MEM_UPDATE, %ld", client_name, rid);
    } else if (req == REQ_QUOTA) {
      // check if there is available quota
      double overuse_ms = get_msg_data<double>(attached, pos);
      double burst = get_msg_data<double>(attached, pos);
      
      double quota_remain = hook_kernel_launch(sockfd, overuse_ms, burst, client_name);
      
      INFO("[RIYACHU] %s (pod manager)hook_thread_func recv - REQ_QUOTA, %ld", client_name, rid);
      // return remaining quota time
      len = prepare_response(sbuf, REQ_QUOTA, rid, quota_remain);
    }
    
    if (len > 0) {
      // have message to send
      if (send(sockfd, sbuf, RSP_MSG_LEN, 0) == -1) {
        ERROR("failed to send message to hook library!");
      }
      INFO("[RIYACHU] %s (pod manager)hook_thread_func send, %ld", client_name ,rid);
    }
  }
  
  INFO("%s connetion closed by peer. recv() returns %ld.", client_name, rc);
  // since hook library close socket only when process terminated, we can use this as an indicator
  // of process termination, and recover memory usage
  pthread_mutex_lock(&mem_info_mutex);
  gpu_mem_used -= allocation_map[sockfd];
  allocation_map.erase(sockfd);
  DEBUG("GPU memory usage = %ld bytes.", gpu_mem_used);
  INFO("GPU memory usage = %ld bytes.", gpu_mem_used);
  pthread_mutex_unlock(&mem_info_mutex);

  pthread_mutex_lock(&client_stat_mutex);
  client_burst_map.erase(sockfd);
  pthread_mutex_unlock(&client_stat_mutex);

  close(sockfd);
  delete (int *)args;
  pthread_exit(NULL);
}

// forward requests to scheduler
void *scheduler_thread_send_func(void *args) {
  INFO("scheduler_thread_send_func");
  int sockfd = *((int *)args);
  ssize_t send_rc;
  /* waiting for request from hook threads */
  while (true) {
    pthread_mutex_lock(&req_queue_mutex);

    pthread_cond_wait(&req_queue_cond, &req_queue_mutex);
    if (!request_queue.empty()) {
      // process request
      request req = request_queue.front();
      request_queue.pop();

      if ((send_rc = send(sockfd, req.data, REQ_MSG_LEN, 0)) <= 0) {
        ERROR("failed to send request to scheduler! return code %ld.", send_rc);
      } else {
        INFO("[RIYACHU] (pod manager)scheduler_thread_send_func recv > 0, %ld", send_rc);
        DEBUG("send a kernel launch request, req_id: %d", req.req_id);
        INFO("send a kernel launch request, req_id: %d", req.req_id);
      }
    }
    pthread_mutex_unlock(&req_queue_mutex);
  }
  pthread_exit(NULL);
}

// receive response from scheduler and place responded data into response_map
void *scheduler_thread_recv_func(void *args) {
  int sockfd = *((int *)args);

  char buf[RSP_MSG_LEN], *attached;
  ssize_t rc;
  INFO("scheduler_thread_recv_func");
  // int flags = fcntl(sockfd, F_GETFL, 0);
  // if(fcntl(sockfd, F_SETFL, flags & ~O_NONBLOCK) < 0) {
  //     ERROR("fcntl() error");
  // }
  //bzero(buf, RSP_MSG_LEN);
  while (true){  
    while ((rc = recv(sockfd, buf, RSP_MSG_LEN, 0)) > 0) {
      
      // process response
      // read response data into another buffer
      reqid_t req_id;
      response rsp;

      attached = parse_response(buf, &req_id);
      rsp.data = new char[RSP_MSG_LEN - sizeof(reqid_t)];
      memcpy(rsp.data, attached, RSP_MSG_LEN - sizeof(reqid_t));
      INFO("[RIYACHU] (pod manager)scheduler_thread_recv_func recv > 0, req_id %ld", req_id);
      DEBUG("req_id %d complete.", req_id);
      INFO("req_id %d complete.", req_id);
      

      pthread_mutex_lock(&scheduler_recv_sync_mutex);
      while(scheduler_recv_sync == 0){
        INFO("[RIYACHU] (pod manager)scheduler_thread_recv_func scheduler_recv_sync == 0, req_id %ld",  req_id);
        pthread_cond_wait(&scheduler_recv_sync_cond, &scheduler_recv_sync_mutex);
      }
      pthread_mutex_unlock(&scheduler_recv_sync_mutex);

      // put response data into response_map and notify hook threads
      pthread_mutex_lock(&rsp_map_mutex);
      response_map.insert(std::make_pair(req_id, rsp));
      int ok = pthread_cond_signal(&rsp_map_cond);
      INFO("[RIYACHU] (pod manager)scheduler_thread_recv_func signal %d, req_id %ld", ok, req_id);
      pthread_mutex_unlock(&rsp_map_mutex);
    }
  }
  WARNING("connection closed by scheduler. recv() returns %ld.", rc);
  close(sockfd);
  pthread_exit(NULL);
}