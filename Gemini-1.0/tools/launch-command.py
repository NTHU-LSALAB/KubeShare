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

import os
import subprocess as sp
import argparse
import signal
import time
import warnings
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--name', help='pod name', required=True)
    parser.add_argument('--port', help='pod manager port', required=True)
    parser.add_argument('--ip', help='pod manager ip', default='127.0.0.1')
    parser.add_argument('--timeout', type=int, help='seconds to run')
    parser.add_argument('command', nargs='+')
    args = parser.parse_args()

    os.setpgrp()

    client_env = os.environ.copy()
    client_env['POD_MANAGER_IP'] = args.ip
    client_env['POD_MANAGER_PORT'] = args.port
    client_env['POD_NAME'] = args.name
    client_env['LD_PRELOAD'] = "{}/gemini/lib/libgemhook.so.1".format(Path.home())

    proc = sp.Popen(
        args.command, env=client_env, start_new_session=True, universal_newlines=True, bufsize=1
    )

    print("[launcher] run: {}".format(args.command))

    try:
        if args.timeout:
            time.sleep(args.timeout)
            proc.terminate()
            proc.wait()
            try:
                os.killpg(proc.pid, signal.SIGTERM)
            except OSError as e:
                warnings.warn(e)
        else:
            proc.wait()
    except KeyboardInterrupt:
        print("\n[launcher] kill everything")
        os.killpg(proc.pid, signal.SIGTERM)
        os.killpg(0, signal.SIGTERM)


if __name__ == '__main__':
    main()
