import yaml
import random
from datetime import datetime
import time
import os
import sys

class Pod:
    def __init__(self, start_time, gpu, run_time):
        self.start_time = start_time # seconds
        self.gpu = gpu
        self.run_time = run_time #minutes

def readTrace():
    pods = list()
    with open("./trace.txt", 'r') as f:
        lines = f.readlines()
        for line in lines:            
            line = line.replace('\n','')
            data = line.split("\t")
            pods.append(Pod(data[0], data[1], data[2]))
            #print(data)
    return pods

def buildPod(name, request, limit, run_time):
    command = f'echo The app is running! && sleep {run_time}'
    container = {
        'name': name,
        'image': 'busybox',
        'imagePullPolicy': 'IfNotPresent',
        'command': ['sh','-c', command],
    }
    #container = yaml.dump(container, Dumper=yaml.CDumper)
    #print(container)
    pod = {
        'apiVersion': 'v1',
        'kind': 'Pod',
        'metadata': {
            'name': name,
            'labels': {
                'sharedgpu/gpu_request': request,
                'sharedgpu/gpu_limit': limit,
            }, 
        },
        'spec': {
            'schedulerName': 'kubeshare-scheduler',
            'containers': [container],
            'restartPolicy': 'Never',
        }
    } 
    
    filename = f'{name}.yaml'
    with open(filename, 'w') as f:
        yaml.dump(pod, f)

def main():
    start_time = datetime.now()
    print('Start time: ', start_time)
    pods = readTrace()
    for pod in pods:
        limit = ""
        request = ""
        
        if int(pod.gpu) > 2:
            request = round(random.random(),2)
            limit = 1.0
        else:
            request = pod.gpu
            limit = pod.gpu
        
        name = f"gpu{pod.gpu}-{pod.start_time}-{pod.run_time}"
        print(f'{time.ctime()}: {name}')
        sys.stdout.flush()
        buildPod(name, str(request), str(limit), pod.run_time)
        current_time = datetime.now()
        #print('Current time: ', current_time)
        sys.stdout.flush()
        sleep_time = int(pod.start_time)
        # while (current_time - start_time).total_seconds() < sleep_time:
        #     current_time = datetime.now()
            
        time.sleep(sleep_time)
        print(os.system(f"kubectl apply -f {name}.yaml"))
        sys.stdout.flush()

    print()
if __name__ == '__main__':
    main()