from prometheus_client import start_http_server, Summary, Gauge
from prometheus_client.core import GaugeMetricFamily, CounterMetricFamily, REGISTRY

import random
import time

quota = 0
name = ""
class CustomCollector(object):
    def __init__(self):
        pass
    def collect(self):
        line = file.readline()
        x = ["no client",1]
        global name
        if(line):
            x = line.strip().split(" ")
            global quota 
            quota = float(x[1])
            name = x[0]
        value = GaugeMetricFamily("quota", 'Help text', labels=['client'])
        value.add_metric([name], quota)
        yield value


if __name__ == '__main__':
    start_http_server(8000)         ## port where metrics need to be exposed.
    file = open("/tmp/Gemini/export.txt", "r")

    REGISTRY.register(CustomCollector())
    while True:
        time.sleep(0.1)
