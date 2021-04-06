#!/usr/bin/env python3
"""
 Copyright 2020 Hung-Hsin Chen, LSA Lab, National Tsing Hua University

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
"""

import subprocess as sp
import os
import argparse
import shlex
import signal


def prepare_env(idx, name, ip, port, schd_port):
    client_env = os.environ.copy()
    client_env['SCHEDULER_IP'] = ip
    client_env['SCHEDULER_PORT'] = str(schd_port)
    client_env['POD_MANAGER_IP'] = ip
    client_env['POD_MANAGER_PORT'] = str(port)
    client_env['POD_NAME'] = name
    return client_env


def launch_scheduler(args):
    cfg_h, cfg_t = os.path.split(args.config)
    if cfg_h == '':
        cfg_h = os.getcwd()

    cmd = "{} -p {} -f {} -P {} -q {} -m {} -w {}".format(
        args.schd, cfg_h, cfg_t, args.port, args.base_quota, args.min_quota, args.window
    )
    proc = sp.Popen(shlex.split(cmd), universal_newlines=True, bufsize=1)
    return proc


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('schd', help='path to scheduler executable')
    parser.add_argument('pmgr', help='path to pod-manager executable')
    parser.add_argument('--ip', default='127.0.0.1', help='IP of scheduling system runs on')
    parser.add_argument('--port', type=int, default=50051, help='base port')
    parser.add_argument('--config', default='resource-config.txt', help='resource config file')
    parser.add_argument('--base_quota', type=float, default=300, help='base quota (ms)')
    parser.add_argument('--min_quota', type=float, default=20, help='minimum quota (ms)')
    parser.add_argument('--window', type=float, default=10000, help='time window (ms)')
    args = parser.parse_args()

    with open(args.config) as f:
        lines = f.read().splitlines()

    n = int(lines[0])  # clients
    childs = []

    schd_proc = launch_scheduler(args)
    childs.append((0, schd_proc))
    print(f"[launcher] scheduler started on {args.ip}:{args.port}")

    port = args.port + 1
    for idx in range(1, n + 1):
        name, req, lim, mem = lines[idx].split()
        proc = sp.Popen(
            shlex.split(args.pmgr),
            env=prepare_env(idx, name, args.ip, port, args.port),
            stderr=sp.DEVNULL,
            stdout=sp.PIPE
        )
        childs.append((idx, proc))
        print(f"[launcher] pod manager of {name} started on {args.ip}:{port}")
        port += 1

    for _, proc in childs:
        proc.wait()


if __name__ == '__main__':
    os.setpgrp()
    try:
        main()
    except KeyboardInterrupt:
        print("\n[launcher] kill all subprocesses")
        os.killpg(0, signal.SIGKILL)
