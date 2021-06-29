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
 * This is a per-GPU manager/scheduler.
 * Based on the information provided by clients, it decide which client to run
 * and give token to that client. This scheduler act as a daemon, accepting
 * connection and requests from pod manager or hook library directly.
 */

#include <iostream>
#include <fstream>
#include "scheduler.h"

#include <arpa/inet.h>
#include <errno.h>
#include <execinfo.h>
#include <getopt.h>
#include <linux/limits.h>
#include <netinet/in.h>
#include <pthread.h>
#include <signal.h>
#include <sys/inotify.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <sys/types.h>
#include <unistd.h>

#include <algorithm>
#include <chrono>
#include <climits>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>

#include <limits>
#include <list>
#include <map>
#include <string>
#include <thread>
#include <typeinfo>
#include <vector>

#include "debug.h"
#include "util.h"
#ifdef RANDOM_QUOTA
#include <random>
#endif

using std::string;
using std::chrono::duration_cast;
using std::chrono::microseconds;
using std::chrono::steady_clock;
std::ofstream myfile ("/tmp/Gemini/export.txt");
// signal handler
void sig_handler(int);
#ifdef _DEBUG
void dump_history(int);
#endif

// helper function for getting timespec
struct timespec get_timespec_after(double ms) {
  struct timespec ts;
  // now
  clock_gettime(CLOCK_MONOTONIC, &ts);

  double sec = ms / 1e3;
  ts.tv_sec += floor(sec);
  ts.tv_nsec += (sec - floor(sec)) * 1e9;
  ts.tv_sec += ts.tv_nsec / 1000000000;
  ts.tv_nsec %= 1000000000;
  return ts;
}

// all in milliseconds
double QUOTA = 250.0;
double MIN_QUOTA = 100.0;
double WINDOW_SIZE = 10000.0;
int verbosity = 0;

#define EVENT_SIZE sizeof(struct inotify_event)
#define BUF_LEN (1024 * (EVENT_SIZE + 16))
auto PROGRESS_START = steady_clock::now();
char limit_file_name[PATH_MAX] = "resource-config.txt";
char limit_file_dir[PATH_MAX] = ".";

std::list<History> history_list;
#ifdef _DEBUG
std::list<History> full_history;
#endif

// milliseconds since scheduler process started
inline double ms_since_start() {
  return duration_cast<microseconds>(steady_clock::now() - PROGRESS_START).count() / 1e3;
}

ClientInfo::ClientInfo(double baseq, double minq, double maxq, double minf, double maxf)
    : BASE_QUOTA(baseq), MIN_QUOTA(minq), MAX_QUOTA(maxq), MIN_FRAC(minf), MAX_FRAC(maxf) {
  quota_ = BASE_QUOTA;
  latest_overuse_ = 0.0;
  latest_actual_usage_ = 0.0;
  burst_ = 0.0;
};

ClientInfo::~ClientInfo(){};

void ClientInfo::set_burst(double estimated_burst) { burst_ = estimated_burst; }

void ClientInfo::update_return_time(double overuse) {
  double now = ms_since_start();
  for (auto it = history_list.rbegin(); it != history_list.rend(); it++) {
    if (it->name == this->name) {
      // client may not use up all of the allocated time
      it->end = std::min(now, it->end + overuse);
      latest_actual_usage_ = it->end - it->start;
      break;
    }
  }
  latest_overuse_ = overuse;
#ifdef _DEBUG
  for (auto it = full_history.rbegin(); it != full_history.rend(); it++) {
    if (it->name == this->name) {
      it->end = std::min(now, it->end + overuse);
      break;
    }
  }
#endif
}

void ClientInfo::Record(double quota) {
  History hist;
  hist.name = this->name;
  hist.start = ms_since_start();
  hist.end = hist.start + quota;
  history_list.push_back(hist);
#ifdef _DEBUG
  full_history.push_back(hist);
#endif
}

double ClientInfo::get_min_fraction() { return MIN_FRAC; }

double ClientInfo::get_max_fraction() { return MAX_FRAC; }

// self-adaptive quota algorithm
double ClientInfo::get_quota() {
  const double UPDATE_RATE = 0.5;  // how drastically will the quota changes
  if(!myfile)myfile.open ("/tmp/Gemini/export.txt", std::ofstream::out | std::ofstream::app);
  if (burst_ < 1e-9) {
    // special case when no burst data available, just fallback to static quota
    quota_ = BASE_QUOTA;
    DEBUG("%s: fallback to static quota, assign quota: %.3fms", name.c_str(), quota_);
  } else {
    quota_ = burst_ * UPDATE_RATE + quota_ * (1 - UPDATE_RATE);
    quota_ = std::max(quota_, MIN_QUOTA);  // lowerbound
    quota_ = std::min(quota_, MAX_QUOTA);  // upperbound
    DEBUG("%s: burst: %.3fms, assign quota: %.3fms", name.c_str(), burst_, quota_);
    myfile<<name.c_str()<<" "<<quota_<<std::endl;
  }
  return quota_;
}

// map container name to object
std::map<string, ClientInfo *> client_info_map;

std::list<candidate_t> candidates;
pthread_mutex_t candidate_mutex = PTHREAD_MUTEX_INITIALIZER;
pthread_cond_t candidate_cond;  // initialized with CLOCK_MONOTONIC in main()

void read_resource_config() {
  std::ifstream fin;
  ClientInfo *client_inf;
  char client_name[HOST_NAME_MAX], full_path[PATH_MAX];
  size_t gpu_memory_size;
  double gpu_min_fraction, gpu_max_fraction;
  int container_num;

  bzero(full_path, PATH_MAX);
  strncpy(full_path, limit_file_dir, PATH_MAX);
  if (limit_file_dir[strlen(limit_file_dir) - 1] != '/') full_path[strlen(limit_file_dir)] = '/';
  strncat(full_path, limit_file_name, PATH_MAX - strlen(full_path));

  // Read GPU limit usage
  fin.open(full_path, std::ios::in);
  if (!fin.is_open()) {
    ERROR("failed to open file %s: %s", full_path, strerror(errno));
    exit(1);
  }
  fin >> container_num;
  INFO("There are %d clients in the system...", container_num);
  for (int i = 0; i < container_num; i++) {
    fin >> client_name >> gpu_min_fraction >> gpu_max_fraction >> gpu_memory_size;
    client_inf = new ClientInfo(QUOTA, MIN_QUOTA, gpu_max_fraction * WINDOW_SIZE, gpu_min_fraction,
                                gpu_max_fraction);
    client_inf->name = client_name;
    client_inf->gpu_mem_limit = gpu_memory_size;
    if (client_info_map.find(client_name) != client_info_map.end())
      delete client_info_map[client_name];
    client_info_map[client_name] = client_inf;
    INFO("%s request: %.2f, limit: %.2f, memory limit: %lu bytes", client_name, gpu_min_fraction,
         gpu_max_fraction, gpu_memory_size);
  }
  fin.close();
}

void monitor_file(const char *path, const char *filename) {
  INFO("Monitor thread created.");
  int fd, wd;

  // Initialize Inotify
  fd = inotify_init();
  if (fd < 0) ERROR("Failed to initialize inotify");

  // add watch to starting directory
  wd = inotify_add_watch(fd, path, IN_CLOSE_WRITE);

  if (wd == -1)
    ERROR("Failed to add watch to '%s'.", path);
  else
    INFO("Watching '%s'.", path);

  // start watching
  while (1) {
    int i = 0;
    char buffer[BUF_LEN];
    int length = read(fd, buffer, BUF_LEN);
    if (length < 0) ERROR("Read error");

    while (i < length) {
      struct inotify_event *event = (struct inotify_event *)&buffer[i];

      if (event->len) {
        if (event->mask & IN_CLOSE_WRITE) {
          INFO("File %s modified with watch descriptor %d.", (const char *)event->name, event->wd);
          // if event is triggered by target file
          if (strcmp((const char *)event->name, filename) == 0) {
            INFO("Update containers' settings...");
            read_resource_config();
          }
        }
      }
      // update index to start of next event
      i += EVENT_SIZE + event->len;
    }
  }

  // Clean up
  // Supposed to be unreached.
  inotify_rm_watch(fd, wd);
  close(fd);
}

/**
 * Select a candidate whose current usage is less than its limit.
 * If no such candidates, calculate the time until time window content changes and sleep until then,
 * or until another candidate comes. If more than one candidate meets the requirement, select one
 * according to scheduling policy.
 * @return selected candidate
 */
candidate_t select_candidate() {
  while (true) {
    /* update history list and get usage in a time interval */
    double window_size = WINDOW_SIZE;
    int overlap_cnt = 0, i, j, k;
    std::map<string, double> usage;
    struct container_timestamp {
      string name;
      double time;
    };

    // start/end timestamp of history in this window
    std::vector<container_timestamp> timestamp_vector;
    // instances to check for overlap
    std::vector<string> overlap_check_vector;

    double now = ms_since_start();
    double window_start = now - WINDOW_SIZE;
    double current_time;
    if (window_start < 0) {
      // elapsed time less than a window size
      window_size = now;
    }

    auto beyond_window = [=](const History &h) -> bool { return h.end < window_start; };
    history_list.remove_if(beyond_window);
    for (auto h : history_list) {
      timestamp_vector.push_back({h.name, -h.start});
      timestamp_vector.push_back({h.name, h.end});
      usage[h.name] = 0;
      if (verbosity > 1) {
        printf("{'container': '%s', 'start': %.3f, 'end': %.3f},\n", h.name.c_str(), h.start / 1e3,
               h.end / 1e3);
      }
    }

    /* select the candidate to give token */

    // quick exit if the first one in candidates does not use GPU recently
    auto check_appearance = [=](const History &h) -> bool {
      return h.name == candidates.front().name;
    };
    if (std::find_if(history_list.begin(), history_list.end(), check_appearance) ==
        history_list.end()) {
      candidate_t selected = candidates.front();
      candidates.pop_front();
      return selected;
    }

    // sort by time
    auto ts_comp = [=](container_timestamp a, container_timestamp b) -> bool {
      return std::abs(a.time) < std::abs(b.time);
    };
    std::sort(timestamp_vector.begin(), timestamp_vector.end(), ts_comp);

    /* calculate overall utilization */
    current_time = window_start;
    for (k = 0; k < timestamp_vector.size(); k++) {
      if (std::abs(timestamp_vector[k].time) <= window_start) {
        // start before this time window
        overlap_cnt++;
        overlap_check_vector.push_back(timestamp_vector[k].name);
      } else {
        break;
      }
    }

    // i >= k: events in this time window
    for (i = k; i < timestamp_vector.size(); ++i) {
      // instances in overlap_check_vector overlaps,
      // overlapped interval = current_time ~ i-th timestamp
      for (j = 0; j < overlap_check_vector.size(); ++j)
        usage[overlap_check_vector[j]] +=
            (std::abs(timestamp_vector[i].time) - current_time) / overlap_cnt;

      if (timestamp_vector[i].time < 0) {
        // is a "start" timestamp
        overlap_check_vector.push_back(timestamp_vector[i].name);
        overlap_cnt++;
      } else {
        // is a "end" timestamp
        // remove instance from overlap_check_vector
        for (j = 0; j < overlap_check_vector.size(); ++j) {
          if (overlap_check_vector[j] == timestamp_vector[i].name) {
            overlap_check_vector.erase(overlap_check_vector.begin() + j);
            break;
          }
        }
        overlap_cnt--;
      }

      // advance current_time
      current_time = std::abs(timestamp_vector[i].time);
    }

    /* select the one to execute */
    std::vector<valid_candidate_t> vaild_candidates;

    for (auto it = candidates.begin(); it != candidates.end(); it++) {
      string name = it->name;
      double limit, require, missing, remaining;
      limit = client_info_map[name]->get_max_fraction() * window_size;
      require = client_info_map[name]->get_min_fraction() * window_size;
      missing = require - usage[name];
      remaining = limit - usage[name];
      if (remaining > 0)
        vaild_candidates.push_back({missing, remaining, usage[it->name], it->arrived_time, it});
    }

    if (vaild_candidates.size() == 0) {
      // all candidates reach usage limit
      auto ts = get_timespec_after(history_list.begin()->end - window_start);
      DEBUG("sleep until %ld.%03ld", ts.tv_sec, ts.tv_nsec / 1000000);
      // also wakes up if new requests come in
      pthread_cond_timedwait(&candidate_cond, &candidate_mutex, &ts);
      continue;  // go to begin of loop
    }

    std::sort(vaild_candidates.begin(), vaild_candidates.end(), schd_priority);

    auto selected_iter = vaild_candidates.begin()->iter;
    candidate_t result = *selected_iter;
    candidates.erase(selected_iter);
    return result;
  }
}

// Get the information from message
void handle_message(int client_sock, char *message) {
  reqid_t req_id;  // simply pass this req_id back to Pod manager
  comm_request_t req;
  size_t hostname_len, offset = 0;
  char sbuf[RSP_MSG_LEN];
  char *attached, *client_name;
  ClientInfo *client_inf;
  attached = parse_request(message, &client_name, &hostname_len, &req_id, &req);

  if (client_info_map.find(string(client_name)) == client_info_map.end()) {
    WARNING("Unknown client \"%s\". Ignore this request.", client_name);
    return;
  }
  client_inf = client_info_map[string(client_name)];
  bzero(sbuf, RSP_MSG_LEN);

  if (req == REQ_QUOTA) {
    double overuse, burst, window;
    overuse = get_msg_data<double>(attached, offset);
    burst = get_msg_data<double>(attached, offset);

    client_inf->update_return_time(overuse);
    client_inf->set_burst(burst);
    pthread_mutex_lock(&candidate_mutex);
    candidates.push_back({client_sock, string(client_name), req_id, ms_since_start()});
    pthread_cond_signal(&candidate_cond);  // wake schedule_daemon_func up
    pthread_mutex_unlock(&candidate_mutex);
    // select_candidate() will give quota later

  } else if (req == REQ_MEM_LIMIT) {
    prepare_response(sbuf, REQ_MEM_LIMIT, req_id, (size_t)0, client_inf->gpu_mem_limit);
    send(client_sock, sbuf, RSP_MSG_LEN, 0);
  } else if (req == REQ_MEM_UPDATE) {
    // ***for communication interface compatibility only***
    // memory usage is only tracked on hook library side
    WARNING("scheduler always returns true for memory usage update!");
    prepare_response(sbuf, REQ_MEM_UPDATE, req_id, 1);
    send(client_sock, sbuf, RSP_MSG_LEN, 0);
  } else {
    WARNING("\"%s\" send an unknown request.", client_name);
  }
}

void *schedule_daemon_func(void *) {
#ifdef RANDOM_QUOTA
  std::random_device rd;
  std::default_random_engine gen(rd());
  std::uniform_real_distribution<double> dis(0.4, 1.0);
#endif
  double quota;

  while (1) {
    pthread_mutex_lock(&candidate_mutex);
    if (candidates.size() != 0) {
      // remove an entry from candidates
      candidate_t selected = select_candidate();
      DEBUG("select %s, waiting time: %.3f ms", selected.name.c_str(),
            ms_since_start() - selected.arrived_time);

      quota = client_info_map[selected.name]->get_quota();
#ifdef RANDOM_QUOTA
      quota *= dis(gen);
#endif
      client_info_map[selected.name]->Record(quota);

      pthread_mutex_unlock(&candidate_mutex);

      // send quota to selected instance
      char sbuf[RSP_MSG_LEN];
      bzero(sbuf, RSP_MSG_LEN);
      prepare_response(sbuf, REQ_QUOTA, selected.req_id, quota);
      send(selected.socket, sbuf, RSP_MSG_LEN, 0);

      struct timespec ts = get_timespec_after(quota);

      // wait until the selected one's quota time out
      bool should_wait = true;
      pthread_mutex_lock(&candidate_mutex);
      while (should_wait) {
        int rc = pthread_cond_timedwait(&candidate_cond, &candidate_mutex, &ts);
        if (rc == ETIMEDOUT) {
          DEBUG("%s didn't return on time", selected.name.c_str());
          should_wait = false;
        } else {
          // check if the selected one comes back
          for (auto conn : candidates) {
            if (conn.name == selected.name) {
              should_wait = false;
              break;
            }
          }
        }
      }
      pthread_mutex_unlock(&candidate_mutex);
    } else {
      // wait for incoming connections
      DEBUG("no candidates");
      pthread_cond_wait(&candidate_cond, &candidate_mutex);
      pthread_mutex_unlock(&candidate_mutex);
    }
  }
}

// daemon function for Pod manager: waiting for incoming request
void *pod_client_func(void *args) {
  int pod_sockfd = *((int *)args);
  char *rbuf = new char[REQ_MSG_LEN];
  ssize_t recv_rc;
  while ((recv_rc = recv(pod_sockfd, rbuf, REQ_MSG_LEN, 0)) > 0) {
    handle_message(pod_sockfd, rbuf);
  }
  DEBUG("Connection closed by Pod manager. recv() returns %ld.", recv_rc);
  close(pod_sockfd);
  delete (int *)args;
  delete[] rbuf;
  pthread_exit(NULL);
}



int main(int argc, char *argv[]) {
  
  uint16_t schd_port = 50051;
  // parse command line options
  const char *optstring = "P:q:m:w:f:p:v:h";
  struct option opts[] = {{"port", required_argument, nullptr, 'P'},
                          {"quota", required_argument, nullptr, 'q'},
                          {"min_quota", required_argument, nullptr, 'm'},
                          {"window", required_argument, nullptr, 'w'},
                          {"limit_file", required_argument, nullptr, 'f'},
                          {"limit_file_dir", required_argument, nullptr, 'p'},
                          {"verbose", required_argument, nullptr, 'v'},
                          {"help", no_argument, nullptr, 'h'},
                          {nullptr, 0, nullptr, 0}};
  int opt;
  while ((opt = getopt_long(argc, argv, optstring, opts, NULL)) != -1) {
    switch (opt) {
      case 'P':
        schd_port = strtoul(optarg, nullptr, 10);
        break;
      case 'q':
        QUOTA = atof(optarg);
        break;
      case 'm':
        MIN_QUOTA = atof(optarg);
        break;
      case 'w':
        WINDOW_SIZE = atof(optarg);
        break;
      case 'f':
        strncpy(limit_file_name, optarg, PATH_MAX - 1);
        break;
      case 'p':
        strncpy(limit_file_dir, optarg, PATH_MAX - 1);
        break;
      case 'v':
        verbosity = atoi(optarg);
        break;
      case 'h':
        printf("usage: %s [options]\n", argv[0]);
        puts("Options:");
        puts("    -P [PORT], --port [PORT]");
        puts("    -q [QUOTA], --quota [QUOTA]");
        puts("    -m [MIN_QUOTA], --min_quota [MIN_QUOTA]");
        puts("    -w [WINDOW_SIZE], --window [WINDOW_SIZE]");
        puts("    -f [LIMIT_FILE], --limit_file [LIMIT_FILE]");
        puts("    -p [LIMIT_FILE_DIR], --limit_file_dir [LIMIT_FILE_DIR]");
        puts("    -v [LEVEL], --verbose [LEVEL]");
        puts("    -h, --help");
        return 0;
      default:
        break;
    }
  }

  if (verbosity > 0) {
    printf("Scheduler settings:\n");
    printf("    %-20s %.3f ms\n", "default quota:", QUOTA);
    printf("    %-20s %.3f ms\n", "minimum quota:", MIN_QUOTA);
    printf("    %-20s %.3f ms\n", "time window:", WINDOW_SIZE);
  }

  // register signal handler for debugging
  signal(SIGSEGV, sig_handler);
#ifdef _DEBUG
  if (verbosity > 0) signal(SIGINT, dump_history);
#endif

  // read configuration file
  read_resource_config();

  int rc;
  int sockfd = 0;
  int forClientSockfd = 0;
  struct sockaddr_in clientInfo;
  int addrlen = sizeof(clientInfo);

  // create a monitored thread
  std::thread t1(monitor_file, std::ref(limit_file_dir), std::ref(limit_file_name));
  t1.detach();

  sockfd = socket(AF_INET, SOCK_STREAM, 0);
  if (sockfd == -1) {
    ERROR("Fail to create a socket!");
    exit(-1);
  }

  struct sockaddr_in serverInfo;
  bzero(&serverInfo, sizeof(serverInfo));

  serverInfo.sin_family = PF_INET;
  serverInfo.sin_addr.s_addr = INADDR_ANY;
  serverInfo.sin_port = htons(schd_port);
  if (bind(sockfd, (struct sockaddr *)&serverInfo, sizeof(serverInfo)) < 0) {
    ERROR("cannot bind port");
    exit(-1);
  }
  listen(sockfd, SOMAXCONN);

  pthread_t tid;

  // initialize candidate_cond with CLOCK_MONOTONIC
  pthread_condattr_t attr_monotonic_clock;
  pthread_condattr_init(&attr_monotonic_clock);
  pthread_condattr_setclock(&attr_monotonic_clock, CLOCK_MONOTONIC);
  pthread_cond_init(&candidate_cond, &attr_monotonic_clock);

  rc = pthread_create(&tid, NULL, schedule_daemon_func, NULL);
  
  if (rc != 0) {
    ERROR("Return code from pthread_create(): %d", rc);
    exit(rc);
  }
  pthread_detach(tid);
  INFO("Waiting for incoming connection");

  while (
      (forClientSockfd = accept(sockfd, (struct sockaddr *)&clientInfo, (socklen_t *)&addrlen))) {
    INFO("Received an incoming connection.");
    pthread_t tid;
    int *pod_sockfd = new int;
    *pod_sockfd = forClientSockfd;
    // create a thread to service this Pod manager
    pthread_create(&tid, NULL, pod_client_func, pod_sockfd);
    pthread_detach(tid);
  }
  if (forClientSockfd < 0) {
    ERROR("Accept failed");
    return 1;
  }
  return 0;
}

void sig_handler(int sig) {
  void *arr[10];
  size_t s;
  s = backtrace(arr, 10);
  ERROR("Received signal %d", sig);
  backtrace_symbols_fd(arr, s, STDERR_FILENO);
  exit(sig);
}

#ifdef _DEBUG
// write full history to a json file
void dump_history(int sig) {
  char filename[20];
  sprintf(filename, "%ld.json", time(NULL));
  FILE *f = fopen(filename, "w");
  fputs("[\n", f);
  for (auto it = full_history.begin(); it != full_history.end(); it++) {
    fprintf(f, "\t{\"container\": \"%s\", \"start\": %.3lf, \"end\" : %.3lf}", it->name.c_str(),
            it->start / 1000.0, it->end / 1000.0);
    if (std::next(it) == full_history.end())
      fprintf(f, "\n");
    else
      fprintf(f, ",\n");
  }
  fputs("]\n", f);
  fclose(f);

  INFO("history dumped to %s", filename);
  exit(0);
}
#endif
