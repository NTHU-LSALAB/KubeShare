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
	SchedulerGPULimitPath = "/kubeshare/scheduler/GPU_limit.txt"

	SchedulerPodIpEnvName = "KUBESHARE_SCHEDULER_IP"
)

var (
	// GPUID => pods request string
	requests map[string]*requestData
)

type requestData struct {
	RequestStr string
	Num        int
}

func init() {
	requests = make(map[string]*requestData)
}

func Run(server string) {
	f, err := os.Create(SchedulerIpPath)
	if err != nil {
		klog.Errorf("Error when create scheduler ip file on path: %s", SchedulerIpPath)
	}
	f.WriteString(os.Getenv(SchedulerPodIpEnvName) + "\n")
	f.Sync()
	f.Close()

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

	getGPUID := func(s string) (GPUID, pods string, err error) {
		GPUID_POS_END := 0
		for pos, char := range s[4:] {
			if char == ':' {
				GPUID_POS_END = pos
			}
		}
		if GPUID_POS_END == 0 {
			err = fmt.Errorf("Error format of receiving message: %s", r)
			klog.Error(err)
			return "", "", err
		}
		GPUID = s[4 : GPUID_POS_END+4]
		return GPUID, s[GPUID_POS_END+5:], nil
	}

	if r[:4] == "ADD:" { // "ADD:GPUID:"
		if GPUID, r, err := getGPUID(r); err != nil {
			return
		} else {
			requests[GPUID] = &requestData{
				RequestStr: strings.ReplaceAll(r, ",", "\n"),
				Num:        strings.Count(r, ","),
			}
		}
	} else if r[:4] == "DEL:" { // "DEL:GPUID:"
		if GPUID, _, err := getGPUID(r); err != nil {
			return
		} else {
			// it's ok to delete a non-exist key
			delete(requests, GPUID)
		}
	} else {
		klog.Errorf("Error format of receiving message: %s", r)
		return
	}

	f, err := os.Create(SchedulerGPULimitPath)
	if err != nil {
		klog.Errorf("Error when create config file on path: %s", SchedulerGPULimitPath)
	}
	defer f.Close()

	num := 0
	for _, data := range requests {
		num += data.Num
	}
	f.WriteString(fmt.Sprintf("%d", num) + "\n")

	for _, data := range requests {
		f.WriteString(data.RequestStr)
	}

	f.Sync()
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
