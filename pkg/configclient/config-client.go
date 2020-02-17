package configclient

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
	"k8s.io/klog"
)

const (
	SchedulerIpPath = "/kubeshare/library/schedulerIP.txt"
	SchedulerGPUConfigPath = "/kubeshare/scheduler/config/"
	SchedulerGPUPodManagerPortPath = "/kubeshare/scheduler/podmanagerport/"

	SchedulerPodIpEnvName = "KUBESHARE_SCHEDULER_IP"
)

func Run(server string) {
	f, err := os.Create(SchedulerIpPath)
	if err != nil {
		klog.Errorf("Error when create scheduler ip file on path: %s", SchedulerIpPath)
	}
	f.WriteString(os.Getenv(SchedulerPodIpEnvName) + "\n")
	f.Sync()
	f.Close()

	os.MkdirAll(SchedulerGPUConfigPath, os.ModePerm)
	os.MkdirAll(SchedulerGPUPodManagerPortPath, os.ModePerm)

	hostname, err := os.Hostname()
	if err != nil {
		klog.Fatalf("Error when get hostname!")
		panic(err)
	}

	conn, err := net.Dial("tcp", server)
	if err != nil {
		klog.Fatalf("Error when connect to manager: %s", err)
		panic(err)
	}
	klog.Infof("Connect successed.")

	reader := bufio.NewReader(conn)

	writeStringToConn(conn, "hostname:"+hostname+"\n")

	registerDevices(conn)

	timer := time.NewTicker(time.Second * 15)
	go sendHeartbeat(conn, timer.C)

	recvRequest(reader)
}

func registerDevices(conn net.Conn) {
	num, err := nvml.GetDeviceCount()
	if err != nil {
		klog.Fatalf("Error when get nvidia device in GetDeviceCount(): %s", err)
	}

	var buf bytes.Buffer
	for i := uint(0); i < num; i++ {
		d, err := nvml.NewDevice(i)
		if err != nil {
			klog.Errorf("Error when get nvidia device's details: %s", err)
		}
		buf.WriteString(d.UUID)
		buf.WriteString(":")
		buf.WriteString(strconv.FormatUint(*(d.Memory), 10))
		buf.WriteString(",")
	}
	buf.WriteString("\n")
	klog.Infof("Registering nvidia device to server in registerDevices(), msg: %s", buf.String())
	conn.Write(buf.Bytes())
}

func recvRequest(reader *bufio.Reader) {
	for {
		requestMess, err := reader.ReadString('\n')
		if err != nil {
			klog.Errorf("Error when receive request from manager")
			return
		}
		handleRequest(string(requestMess[:len(requestMess)-1]))
	}
}

func handleRequest(r string) {
	klog.Infof("Receive request: %s", r)

	req_arr := strings.Split(r, ":")
	if len(req_arr) != 3 {
		klog.Errorf("Error fmat of receiving message: %s", r)
		return
	}

	UUID, podlist, portmap := req_arr[0], req_arr[1], req_arr[2]

	gpu_config_f, err := os.Create(SchedulerGPUConfigPath + UUID)
	if err != nil {
		klog.Errorf("Error when create config file on path: %s", SchedulerGPUConfigPath + UUID)
	}
	
	podmanager_port_f, err := os.Create(SchedulerGPUPodManagerPortPath + UUID)
	if err != nil {
		klog.Errorf("Error when create config file on path: %s", SchedulerGPUPodManagerPortPath + UUID)
	}

	gpu_config_f.WriteString(fmt.Sprintf("%d\n", strings.Count(podlist, ",")))
	gpu_config_f.WriteString(strings.ReplaceAll(podlist, ",", "\n"))

	podmanager_port_f.WriteString(fmt.Sprintf("%d\n", strings.Count(portmap, ",")))
	podmanager_port_f.WriteString(strings.ReplaceAll(portmap, ",", "\n"))

	gpu_config_f.Sync()
	podmanager_port_f.Sync()
	gpu_config_f.Close()
	podmanager_port_f.Close()
}

func sendHeartbeat(conn net.Conn, tick <-chan time.Time) error {
	klog.Infof("Send heartbeat: %s", time.Now().String())
	writeStringToConn(conn, "heartbeat!\n")
	for {
		<-tick
		klog.Infof("Send heartbeat: %s", time.Now().String())
		writeStringToConn(conn, "heartbeat!\n")
	}
}

func writeStringToConn(conn net.Conn, s string) error {
	if _, err := conn.Write([]byte(s)); err != nil {
		klog.Errorf("Error when send msg: %s, to server", s)
		return err
	}
	return nil
}
