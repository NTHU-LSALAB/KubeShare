package devicemanager

import (
	"bufio"
	"bytes"
	"container/list"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	sharedgpuv1 "KubeShare/pkg/apis/sharedgpu/v1"
	"KubeShare/pkg/lib/bitmap"
)

type ClientStatus string

const (
	ClientReady    ClientStatus = "Ready"
	ClientNotReady ClientStatus = "NotReady"
)

type NodeStatus struct {
	Conn          net.Conn
	LastHeartbeat time.Time
	ClientStatus  ClientStatus
}

var (
	nodeStatus    map[string]*NodeStatus
	nodeStatusMux sync.Mutex

	kubeClient kubernetes.Interface
)

func init() {
	nodeStatus = make(map[string]*NodeStatus)
}

func StartConfigManager(stopCh <-chan struct{}, kc kubernetes.Interface) error {
	kubeClient = kc

	rand.Seed(time.Now().Unix())
	ksl.Infof("Start listening on 0.0.0.0:9797...")
	netListen, err := net.Listen("tcp", "0.0.0.0:9797")
	if err != nil {
		msg := fmt.Sprintf("Cannot listening to 0.0.0.0:9797: %s", err.Error())
		ksl.Errorf(msg)
		return fmt.Errorf(msg)
	}
	defer netListen.Close()

	timer := time.NewTicker(time.Second * 60)
	go periodicallyCheckHeartbeats(timer.C)

	ksl.Infof("Waiting for clients...")
	for {
		conn, err := netListen.Accept()
		if err != nil {
			ksl.Errorf("Error when connect to a client: %s", err.Error())
			continue
		}
		ksl.Infof("Connect to a client, addr:port=%s", conn.RemoteAddr().String())

		go clientHandler(conn)
	}
}

func clientHandler(conn net.Conn) {
	reader := bufio.NewReader(conn)
	defer conn.Close()

	hostname := ""
	hostnameMess, err := reader.ReadString('\n')
	if err != nil {
		ksl.Errorf("Error when receive hostname from client")
		return
	}
	if tmp := strings.Split(string(hostnameMess), ":"); tmp[0] != "hostname" {
		ksl.Errorf("Wrong identification when receving hostname")
		return
	} else {
		hostname = tmp[1][:len(tmp[1])-1]
	}

	podIP := strings.Split(conn.RemoteAddr().String(), ":")[0]

	nodeName := ""
	pod, err := kubeClient.CoreV1().Pods("kube-system").Get(hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		ksl.Errorf("ConfigFileClient Pod name: %s, not found!", hostname)
		return
	}
	nodeName = pod.Spec.NodeName

	nvidiaDviceModelMsg, err := reader.ReadString('\n')
	if err != nil {
		ksl.Errorf("Error when receive nvidia device model from client")
		return
	}
	var nvidiaDviceModelStr string
	if tmp := strings.Split(string(nvidiaDviceModelMsg), ":"); tmp[0] != "NvidiaDeviceModel" {
		ksl.Errorf("Wrong identification when receving nvidia device model\n")
		return
	} else {
		nvidiaDviceModelStr = tmp[1][:len(tmp[1])-1]
	}

	ksl.Infof("Receive device model from node: %s, model: %s", nodeName, nvidiaDviceModelStr)

	nvidiaDevicesMsg, err := reader.ReadString('\n')
	if err != nil {
		ksl.Errorf("Error when receive nvidia devices list from client")
		return
	}
	nvidiaDevicesStr := string(nvidiaDevicesMsg)
	ksl.Infof("Receive device list from node: %s, devices: %s", nodeName, nvidiaDevicesStr)
	deviceIDArr := strings.Split(nvidiaDevicesStr[:len(nvidiaDevicesStr)-1], ",")
	uuid2mem := make(map[string]string, len(deviceIDArr)-1)
	uuid2port := make(map[string]string, len(deviceIDArr)-1)
	port := 50051 // 49901
	for _, d := range deviceIDArr {
		if d == "" {
			continue
		}
		dArr := strings.Split(d, ":")
		id, mem := dArr[0], dArr[1]
		uuid2mem[id] = mem
		uuid2port[id] = strconv.Itoa(port)
		port += 1
	}

	nodesInfoMux.Lock()
	if node, ok := nodesInfo[nodeName]; !ok {
		bm := bitmap.NewRRBitmap(512)
		bm.Mask(0)
		nodesInfo[nodeName] = &NodeInfo{
			GPUID2GPU:            make(map[string]*GPUInfo),
			UUID2Port:            uuid2port,
			PodIP:                podIP,
			PodManagerPortBitmap: bm,
			GPUModel:             nvidiaDviceModelStr,
		}
	} else {
		node.UUID2Port = uuid2port
		node.PodIP = podIP
	}
	nodesInfoMux.Unlock()

	nodeStatusMux.Lock()
	if _, ok := nodeStatus[nodeName]; ok {
		// Node nodeName is already in nodeclients
		nodeStatus[nodeName].Conn = conn
		nodeStatus[nodeName].LastHeartbeat = time.Time{}
		nodeStatus[nodeName].ClientStatus = ClientReady
	} else {
		// Add new NodeClient to nodeclients
		nodeStatus[nodeName] = &NodeStatus{
			Conn:          conn,
			LastHeartbeat: time.Time{},
			ClientStatus:  ClientReady,
		}
	}
	nodeStatusMux.Unlock()

	UpdateNodeGPUInfo(nodeName, &uuid2mem, nvidiaDviceModelStr)

	nodesInfoMux.Lock()
	for _, gpuinfo := range nodesInfo[nodeName].GPUID2GPU {
		syncConfig(nodeName, gpuinfo.UUID, gpuinfo.PodList)
	}
	nodesInfoMux.Unlock()

	for {
		isError := false
		heartbeatMess, err := reader.ReadString('\n')
		if err != nil {
			ksl.Errorf("Error when receive heartbeat from: %s", nodeName)
			isError = true
		}
		if string(heartbeatMess) != "heartbeat!\n" {
			ksl.Errorf("Error when receive 'heartbeat!' string from: %s", nodeName)
			isError = true
		}
		nodeStatusMux.Lock()
		if isError {
			nodeStatus[nodeName].ClientStatus = ClientNotReady
		} else {
			nodeStatus[nodeName].LastHeartbeat = time.Now()
			nodeStatus[nodeName].ClientStatus = ClientReady
		}
		nodeStatusMux.Unlock()
		if isError {
			return
		} else {
			ksl.Infof("Receive heartbeat from node: %s", nodeName)
		}
	}
}

func UpdateNodeGPUInfo(nodeName string, uuid2mem *map[string]string, nvidiaDviceModel string) {
	var buf bytes.Buffer
	for id, d := range *uuid2mem {
		buf.WriteString(id)
		buf.WriteString(":")
		buf.WriteString(d)
		buf.WriteString(",")
	}
	node, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		ksl.Errorf("Error when Get node %s, :%s", nodeName, err)
	}
	newNode := node.DeepCopy()
	if newNode.ObjectMeta.Annotations == nil {
		newNode.ObjectMeta.Annotations = make(map[string]string)
	}
	gpuinfo := buf.String()
	newNode.ObjectMeta.Annotations[sharedgpuv1.KubeShareNodeGPUInfo] = gpuinfo
	ksl.Debugf("Update node %s GPU info: %s", nodeName, gpuinfo)
	newNode.ObjectMeta.Annotations[sharedgpuv1.KubeShareNodeGPUModel] = nvidiaDviceModel
	ksl.Debugf("Update node %s GPU Model info: %s", nodeName, nvidiaDviceModel)
	_, err = kubeClient.CoreV1().Nodes().Update(newNode)
	if err != nil {
		ksl.Errorf("Error when updating Node %s annotation, err: %s, spec: %v", nodeName, err, newNode)
	}
}

func periodicallyCheckHeartbeats(tick <-chan time.Time) {
	for {
		<-tick
		nodeStatusMux.Lock()
		now := time.Now()
		for _, node := range nodeStatus {
			if now.Sub(node.LastHeartbeat).Seconds() > 60 {
				node.ClientStatus = ClientNotReady
			}
		}
		nodeStatusMux.Unlock()
	}
}

func syncConfig(nodeName, UUID string, podList *list.List) error {
	nodeStatusMux.Lock()
	client, ok := nodeStatus[nodeName]
	nodeStatusMux.Unlock()

	if !ok {
		msg := fmt.Sprintf("No client node: %s", nodeName)
		ksl.Errorf(msg)
		return fmt.Errorf(msg)
	}
	if client.ClientStatus != ClientReady {
		msg := fmt.Sprintf("Node status is not ready when SyncConfig(): %s", nodeName)
		ksl.Errorf(msg)
		return fmt.Errorf(msg)
	}

	var buf bytes.Buffer

	buf.WriteString(UUID)
	buf.WriteString(":")

	if podList != nil {
		for pod := podList.Front(); pod != nil; pod = pod.Next() {
			buf.WriteString(pod.Value.(*PodRequest).Key)
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%f", pod.Value.(*PodRequest).Request))
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%f", pod.Value.(*PodRequest).Limit))
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%d", pod.Value.(*PodRequest).Memory))
			buf.WriteString(",")
		}
	}

	buf.WriteString(":")

	if podList != nil {
		for pod := podList.Front(); pod != nil; pod = pod.Next() {
			buf.WriteString(pod.Value.(*PodRequest).Key)
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%d", pod.Value.(*PodRequest).PodManagerPort))
			buf.WriteString(",")
		}
	}

	buf.WriteString("\n")

	ksl.Infof("Syncing to node '%s' with content: '%s'", nodeName, buf.String())
	if _, err := client.Conn.Write(buf.Bytes()); err != nil {
		msg := fmt.Sprintf("Error when write '%s' to Conn of node: %s", buf.String(), nodeName)
		ksl.Errorf(msg)
		return fmt.Errorf(msg)
	}

	return nil
}
