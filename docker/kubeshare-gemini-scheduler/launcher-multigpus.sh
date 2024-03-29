#!/bin/bash

command -v nvidia-smi
if [ $? -ne 0 ]; then
    echo "No GPU available, sleep forever"
    sleep infinity
fi

function trap_ctrlc ()
{
    echo "Ctrl-C caught...performing clean up"
    for pid in $pids; do
        kill $pid
    done
    echo "Doing cleanup"
    exit 0
}

trap "trap_ctrlc" 2

port=49901



for gpu in $(nvidia-smi --format=csv,noheader --query-gpu=uuid); do
    if [ -f $1/$gpu ]; then
        echo "config file exist"
    else
        echo "config file not find, create"
        echo 0 > $1/$gpu
    fi
    if [ -f $2/$gpu ]; then
        echo "port config file exist"
    else
        echo "port config file not find, create"
        echo 0 > $2/$gpu
    fi
    #echo 0 > $1/$gpu
    python3 /launcher.py /gem-schd /gem-pmgr $gpu $1/$gpu $2 --port $port 1>&2 &
    pids="$pids $!"
    port=$(($port+1))
done

wait