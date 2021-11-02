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
	"github.com/sirupsen/logrus"
)

const (
	SchedulerIpPath                = "/kubeshare/library/schedulerIP.txt"
	SchedulerGPUConfigPath         = "/kubeshare/scheduler/config/"
	SchedulerGPUPodManagerPortPath = "/kubeshare/scheduler/podmanagerport/"

	SchedulerPodIpEnvName = "KUBESHARE_SCHEDULER_IP"
)

var (
	ksl *logrus.Logger
)

func Run(server string, kubeshareLogger *logrus.Logger) {
	ksl = kubeshareLogger

	f, err := os.Create(SchedulerIpPath)
	if err != nil {
		ksl.Errorf("Error when create scheduler ip file on path: %s", SchedulerIpPath)
	}
	f.WriteString(os.Getenv(SchedulerPodIpEnvName) + "\n")
	f.Sync()
	f.Close()

	os.MkdirAll(SchedulerGPUConfigPath, os.ModePerm)
	os.MkdirAll(SchedulerGPUPodManagerPortPath, os.ModePerm)

	hostname, err := os.Hostname()
	if err != nil {
		ksl.Fatalf("Error when get hostname!")
		panic(err)
	}

	conn, err := net.Dial("tcp", server)
	if err != nil {
		ksl.Fatalf("Error when connect to manager: %s", err)
		panic(err)
	}
	ksl.Info("Connect successed.")

	reader := bufio.NewReader(conn)

	writeStringToConn(conn, "hostname:"+hostname+"\n")

	registerGPUMode(conn)

	registerDevices(conn)

	timer := time.NewTicker(time.Second * 15)
	go sendHeartbeat(conn, timer.C)

	recvRequest(reader)
}

func registerGPUMode(conn net.Conn) {
	num, err := nvml.GetDeviceCount()
	if err != nil {
		ksl.Fatalf("Error when get nvidia device in GetDeviceCount(): %s", err)
	}
	if num >= 1 {
		d, err := nvml.NewDevice(0)
		if err != nil {
			ksl.Errorf("Error when get nvidia device's details: %s", err)
		}
		model := *d.Model
		ksl.Info("NvidiaDeviceModel:" + model)
		writeStringToConn(conn, "NvidiaDeviceModel:"+model+"\n")
	}
}

func registerDevices(conn net.Conn) {
	num, err := nvml.GetDeviceCount()
	if err != nil {
		ksl.Fatalf("Error when get nvidia device in GetDeviceCount(): %s", err)
	}

	var buf bytes.Buffer
	for i := uint(0); i < num; i++ {
		d, err := nvml.NewDevice(i)
		if err != nil {
			ksl.Errorf("Error when get nvidia device's details: %s", err)
		}
		buf.WriteString(d.UUID)
		buf.WriteString(":")
		buf.WriteString(strconv.FormatUint(*(d.Memory), 10))
		buf.WriteString(",")
	}
	buf.WriteString("\n")
	ksl.Infof("Registering nvidia device to server in registerDevices(), msg: %s", buf.String())
	conn.Write(buf.Bytes())
}

func recvRequest(reader *bufio.Reader) {
	for {
		requestMess, err := reader.ReadString('\n')
		if err != nil {
			ksl.Errorf("Error when receive request from manager")
			return
		}
		handleRequest(string(requestMess[:len(requestMess)-1]))
	}
}

func handleRequest(r string) {
	ksl.Infof("Receive request: %s", r)

	req_arr := strings.Split(r, ":")
	if len(req_arr) != 3 {
		ksl.Errorf("Error fmat of receiving message: %s", r)
		return
	}

	UUID, podlist, portmap := req_arr[0], req_arr[1], req_arr[2]

	gpu_config_f, err := os.Create(SchedulerGPUConfigPath + UUID)
	if err != nil {
		ksl.Errorf("Error when create config file on path: %s", SchedulerGPUConfigPath+UUID)
	}

	gpu_config_f.WriteString(fmt.Sprintf("%d\n", strings.Count(podlist, ",")))
	gpu_config_f.WriteString(strings.ReplaceAll(podlist, ",", "\n"))

	podmanager_port_f, err := os.Create(SchedulerGPUPodManagerPortPath + UUID)
	if err != nil {
		ksl.Errorf("Error when create config file on path: %s", SchedulerGPUPodManagerPortPath+UUID)
	}
	podmanager_port_f.WriteString(fmt.Sprintf("%d\n", strings.Count(portmap, ",")))
	podmanager_port_f.WriteString(strings.ReplaceAll(portmap, ",", "\n"))

	gpu_config_f.Sync()
	podmanager_port_f.Sync()
	podmanager_port_f.Close()
	gpu_config_f.Close()
}

func sendHeartbeat(conn net.Conn, tick <-chan time.Time) error {
	ksl.Infof("Send heartbeat: %s", time.Now().String())
	writeStringToConn(conn, "heartbeat!\n")
	for {
		<-tick
		ksl.Infof("Send heartbeat: %s", time.Now().String())
		writeStringToConn(conn, "heartbeat!\n")
	}
}

func writeStringToConn(conn net.Conn, s string) error {
	if _, err := conn.Write([]byte(s)); err != nil {
		ksl.Errorf("Error when send msg: %s, to server", s)
		return err
	}
	return nil
}
