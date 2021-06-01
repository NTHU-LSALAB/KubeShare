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

#ifndef PREDICTOR_H
#define PREDICTOR_H

#include <pthread.h>

#include <chrono>
#include <deque>

typedef std::chrono::time_point<std::chrono::_V2::steady_clock> timepoint_t;

const int64_t PREDICT_MAX_KEEP = 3000;  // maximum time a record will be kept (in milliseconds)

class RecordKeeper {
 public:
  RecordKeeper(const int64_t);
  void add(const double, const timepoint_t);
  void drop_outdated(const timepoint_t);
  void clear();
  double get_max();

 private:
  const int64_t VALID_TIME;
  std::deque<std::pair<timepoint_t, double>> records_;
};

class Predictor {
 public:
  Predictor(const char *name = "", const double thres = 0.0);
  ~Predictor();
  void record_stop();
  void record_start();
  void interrupt();
  bool ongoing_unmerged();
  bool ongoing_merged();
  double predict_unmerged();
  double predict_merged();
  void set_upperbound(const double bound);
  void reset();

 private:
  const char *name_;
  // two consecutive period with interval less than this value will be merged
  const double MERGE_THRES;
  pthread_mutex_t mutex_;
  timepoint_t period_begin_;
  timepoint_t long_period_begin_, long_period_end_;
  RecordKeeper normal_records, long_records;
  double upperbound_;
};

#endif
