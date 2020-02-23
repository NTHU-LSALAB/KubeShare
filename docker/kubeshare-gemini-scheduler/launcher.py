import argparse
import inotify.adapters
import os
import sys
import signal
import shlex
import subprocess as sp
import time

args = None
podlist = {}

def prepare_env(name, port, schd_port):
    client_env = os.environ.copy()
    client_env['SCHEDULER_IP'] = '127.0.0.1'
    client_env['SCHEDULER_PORT'] = str(schd_port)
    client_env['POD_MANAGER_IP'] = '0.0.0.0'
    client_env['POD_MANAGER_PORT'] = str(port)
    client_env['POD_NAME'] = name
    return client_env

def launch_scheduler():
    cfg_h, cfg_t = os.path.split(args.pod_list)
    if cfg_h == '':
        cfg_h = os.getcwd()

    cmd = "{} -p {} -f {} -P {} -q {} -m {} -w {}".format(
        args.schd, cfg_h, cfg_t, args.port, args.base_quota, args.min_quota, args.window
    )
    proc = sp.Popen(shlex.split(cmd), universal_newlines=True, bufsize=1)
    return proc

def update_podmanager(file):
    with open(file) as f:
        lines = f.readlines()
    if not lines:
        return
    podnum = int(lines[0])
    for _, val in podlist.items():
        val[0] = False
    for i in range(1, podnum+1):
        name, port = lines[i].split()
        name_port = lines[i][:-1]
        if name_port not in podlist:
            sys.stderr.write("[launcher] pod manager id '{}' port '{}' start running\n".format(name_port, port))
            sys.stderr.flush()
            proc = sp.Popen(
                shlex.split(args.pmgr),
                env=prepare_env(name, port, args.port),
                preexec_fn=os.setpgrp,
            )
            podlist[name_port] = [True, proc]
        else:
            podlist[name_port][0] = True
    del_list = []
    for n, val in podlist.items():
        if not val[0]:
            os.killpg(os.getpgid(val[1].pid), signal.SIGKILL)
            val[1].wait()
            sys.stderr.write("[launcher] pod manager id '{}' has been deleted\n".format(n))
            sys.stderr.flush()
            del_list.append(n)
    for n in del_list:
        del podlist[n]

def main():
    global args
    parser = argparse.ArgumentParser()
    parser.add_argument('schd', help='path to scheduler executable')
    parser.add_argument('pmgr', help='path to pod-manager executable')
    parser.add_argument('gpu_uuid', help='scheduling system GPU UUID')
    parser.add_argument('pod_list', help='path to pod list file')
    parser.add_argument('pmgr_port_dir', help='path to pod port dir')
    parser.add_argument('--port', type=int, default=49901, help='base port')
    parser.add_argument('--base_quota', type=float, default=300, help='base quota (ms)')
    parser.add_argument('--min_quota', type=float, default=20, help='minimum quota (ms)')
    parser.add_argument('--window', type=float, default=10000, help='time window (ms)')
    args = parser.parse_args()

    launch_scheduler()
    sys.stderr.write(f"[launcher] scheduler started on 0.0.0.0:{args.port}\n")
    sys.stderr.flush()

    ino = inotify.adapters.Inotify()
    ino.add_watch(args.pmgr_port_dir, inotify.constants.IN_CLOSE_WRITE)
    for event in ino.event_gen(yield_nones=False):
        (_, type_names, path, filename) = event
        try:
            if filename == args.gpu_uuid:
                update_podmanager(os.path.join(args.pmgr_port_dir, args.gpu_uuid))
        except: # file content may not correct
            sys.stderr.write("Catch exception in update_podmanager: {}\n".format(sys.exc_info()))
            sys.stderr.flush()

if __name__ == '__main__':
    os.setpgrp()
    try:
        main()
    except:
        sys.stderr.write("Catch exception: {}\n".format(sys.exc_info()))
        sys.stderr.flush()
    finally:
        for _, val in podlist.items():
            os.killpg(os.getpgid(val[1].pid), signal.SIGKILL)
        os.killpg(0, signal.SIGKILL)
