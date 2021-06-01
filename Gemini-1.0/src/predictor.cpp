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
 * Kernel burst and window period measurement/prediction utilities.
 * There are two types of prediction result: plain (unmerged) and merged.
 * Plain (unmerged) results are results of a single period/burst.
 * Merged results are results of several consecutive periods/bursts.
 *
 * Measurement and prediction can be disabled by defining NO_PREDICT.
 */

#include "predictor.h"

#include "debug.h"

using std::make_pair;
using std::chrono::duration_cast;
using std::chrono::microseconds;
using std::chrono::milliseconds;
using std::chrono::steady_clock;

// RecordKeeper

RecordKeeper::RecordKeeper(const int64_t valid_time) : VALID_TIME(valid_time) {}

// maintains the decreasing property, for O(1) time complexity
void RecordKeeper::add(const double data, const timepoint_t tp) {
  while (!records_.empty() && records_.back().second < data) records_.pop_back();
  records_.push_back(make_pair(tp, data));
}

// remove records which are VALID_TIME milliseconds before tp
void RecordKeeper::drop_outdated(const timepoint_t tp) {
  while (!records_.empty() &&
         duration_cast<milliseconds>(tp - records_.front().first).count() > VALID_TIME)
    records_.pop_front();
}

double RecordKeeper::get_max() {
  if (records_.empty())
    return 0.0;
  else
    return records_.front().second;
}

void RecordKeeper::clear() { records_.clear(); }

// Predictor

Predictor::Predictor(const char *name, const double thres)
    : MERGE_THRES(thres), normal_records(PREDICT_MAX_KEEP), long_records(PREDICT_MAX_KEEP) {
  mutex_ = PTHREAD_MUTEX_INITIALIZER;
  period_begin_ = timepoint_t::max();
  long_period_begin_ = timepoint_t::max();
  long_period_end_ = timepoint_t::min();
  upperbound_ = std::numeric_limits<double>::max();
  name_ = name;
}

Predictor::~Predictor() { pthread_mutex_destroy(&mutex_); }

// Check whether we're in an active burst/period.
bool Predictor::ongoing_unmerged() { return period_begin_ != timepoint_t::max(); }

// Check whether we're in an active long-burst/long-period.
bool Predictor::ongoing_merged() { return long_period_begin_ != timepoint_t::max(); }

// Marks complete for a period
void Predictor::record_stop() {
#ifndef NO_PREDICT
  double duration;
  timepoint_t tp;

  pthread_mutex_lock(&mutex_);
  if (ongoing_unmerged()) {
    // record duration
    tp = steady_clock::now();
    duration = duration_cast<microseconds>(tp - period_begin_).count() / 1e3;
    normal_records.add(duration, tp);
    long_period_end_ = tp;
    long_records.add(
        duration_cast<microseconds>(long_period_end_ - long_period_begin_).count() / 1e3, tp);
    DEBUG("%s: record stop (length: %.3f ms)", name_, duration);
  }
  period_begin_ = timepoint_t::max();
  pthread_mutex_unlock(&mutex_);
#endif
}

// Marks begin for a period. Future calls until record_stop is called takes no effect.
void Predictor::record_start() {
#ifndef NO_PREDICT
  double intv;
  pthread_mutex_lock(&mutex_);
  if (!ongoing_unmerged()) {
    period_begin_ = steady_clock::now();

    intv = duration_cast<microseconds>(period_begin_ - long_period_end_).count() / 1e3;
    // long period did not started || last long period too long ago
    if (!ongoing_merged() || intv > MERGE_THRES) {
      long_period_begin_ = period_begin_;
      long_period_end_ = timepoint_t::min();
    }

    DEBUG("%s: record start", name_);
  }
  pthread_mutex_unlock(&mutex_);
#endif
}

// Invalidate currently running period.
// A new period begins with another record_start call afterwards.
void Predictor::interrupt() {
#ifndef NO_PREDICT
  pthread_mutex_lock(&mutex_);
  period_begin_ = timepoint_t::max();
  long_period_begin_ = timepoint_t::max();
  long_period_end_ = timepoint_t::min();
  DEBUG("%s: interrupted", name_);
  pthread_mutex_unlock(&mutex_);
#endif
}

// Get predicted length of an unmerged burst/period.
double Predictor::predict_unmerged() {
  double pred = 0.0;
  timepoint_t now;

#ifndef NO_PREDICT
  pthread_mutex_lock(&mutex_);
  normal_records.drop_outdated(steady_clock::now());
  pred = normal_records.get_max();
  pthread_mutex_unlock(&mutex_);
#endif
  return pred;
}

// Get predicted length of a (possibly) merged period.
double Predictor::predict_merged() {
  double pred = 0.0;

#ifndef NO_PREDICT
  pthread_mutex_lock(&mutex_);
  long_records.drop_outdated(steady_clock::now());
  pred = long_records.get_max();
  pthread_mutex_unlock(&mutex_);
#endif
  return pred;
}

void Predictor::set_upperbound(const double bound) {
#ifndef NO_PREDICT
  pthread_mutex_lock(&mutex_);
  upperbound_ = bound;
  pthread_mutex_unlock(&mutex_);
#endif
}

// Clear all past records and status
void Predictor::reset() {
#ifndef NO_PREDICT
  pthread_mutex_lock(&mutex_);
  normal_records.clear();
  long_records.clear();
  period_begin_ = timepoint_t::max();
  long_period_begin_ = timepoint_t::max();
  long_period_end_ = timepoint_t::min();
  pthread_mutex_unlock(&mutex_);
#endif
}
