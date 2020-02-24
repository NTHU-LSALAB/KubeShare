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
	"k8s.io/klog"

	kubesharev1 "github.com/NTHU-LSALAB/KubeShare/pkg/apis/kubeshare/v1"
	"github.com/NTHU-LSALAB/KubeShare/pkg/lib/bitmap"
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
	klog.Infof("Start listening on 0.0.0.0:9797...")
	netListen, err := net.Listen("tcp", "0.0.0.0:9797")
	if err != nil {
		msg := fmt.Sprintf("Cannot listening to 0.0.0.0:9797: %s", err.Error())
		klog.Errorf(msg)
		return fmt.Errorf(msg)
	}
	defer netListen.Close()

	timer := time.NewTicker(time.Second * 60)
	go periodicallyCheckHeartbeats(timer.C)

	klog.Infof("Waiting for clients...")
	for {
		conn, err := netListen.Accept()
		if err != nil {
			klog.Errorf("Error when connect to a client: %s", err.Error())
			continue
		}
		klog.Infof("Connect to a client, addr:port=%s", conn.RemoteAddr().String())

		go clientHandler(conn)
	}
}

func clientHandler(conn net.Conn) {
	reader := bufio.NewReader(conn)
	defer conn.Close()

	hostname := ""
	hostnameMess, err := reader.ReadString('\n')
	if err != nil {
		klog.Errorf("Error when receive hostname from client")
		return
	}
	if tmp := strings.Split(string(hostnameMess), ":"); tmp[0] != "hostname" {
		klog.Errorf("Wrong identification when receving hostname")
		return
	} else {
		hostname = tmp[1][:len(tmp[1])-1]
	}

	podIP := strings.Split(conn.RemoteAddr().String(), ":")[0]

	nodeName := ""
	pod, err := kubeClient.CoreV1().Pods("kube-system").Get(hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		klog.Errorf("ConfigFileClient Pod name: %s, not found!", hostname)
		return
	}
	nodeName = pod.Spec.NodeName

	nvidiaDevicesMsg, err := reader.ReadString('\n')
	if err != nil {
		klog.Errorf("Error when receive nvidia devices list from client")
		return
	}
	nvidiaDevicesStr := string(nvidiaDevicesMsg)
	klog.Infof("Receive device list from node: %s, devices: %s", nodeName, nvidiaDevicesStr)
	deviceIDArr := strings.Split(nvidiaDevicesStr[:len(nvidiaDevicesStr)-1], ",")
	uuid2mem := make(map[string]string, len(deviceIDArr)-1)
	uuid2port := make(map[string]string, len(deviceIDArr)-1)
	port := 50051
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

	UpdateNodeGPUInfo(nodeName, &uuid2mem)

	nodesInfoMux.Lock()
	for _, gpuinfo := range nodesInfo[nodeName].GPUID2GPU {
		syncConfig(nodeName, gpuinfo.UUID, gpuinfo.PodList)
	}
	nodesInfoMux.Unlock()

	for {
		isError := false
		heartbeatMess, err := reader.ReadString('\n')
		if err != nil {
			klog.Errorf("Error when receive heartbeat from: %s", nodeName)
			isError = true
		}
		if string(heartbeatMess) != "heartbeat!\n" {
			klog.Errorf("Error when receive 'heartbeat!' string from: %s", nodeName)
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
			klog.Infof("Receive heartbeat from node: %s", nodeName)
		}
	}
}

func UpdateNodeGPUInfo(nodeName string, uuid2mem *map[string]string) {
	var buf bytes.Buffer
	for id, d := range *uuid2mem {
		buf.WriteString(id)
		buf.WriteString(":")
		buf.WriteString(d)
		buf.WriteString(",")
	}
	node, err := kubeClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error when Get node %s, :%s", nodeName, err)
	}
	newNode := node.DeepCopy()
	if newNode.ObjectMeta.Annotations == nil {
		newNode.ObjectMeta.Annotations = make(map[string]string)
	}
	gpuinfo := buf.String()
	newNode.ObjectMeta.Annotations[kubesharev1.KubeShareNodeGPUInfo] = gpuinfo
	klog.Infof("Update node %s GPU info: %s", nodeName, gpuinfo)
	_, err = kubeClient.CoreV1().Nodes().Update(newNode)
	if err != nil {
		klog.Errorf("Error when updating Node %s annotation, err: %s, spec: %v", nodeName, err, newNode)
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
		klog.Errorf(msg)
		return fmt.Errorf(msg)
	}
	if client.ClientStatus != ClientReady {
		msg := fmt.Sprintf("Node status is not ready when SyncConfig(): %s", nodeName)
		klog.Errorf(msg)
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

	klog.Infof("Syncing to node '%s' with content: '%s'", nodeName, buf.String())
	if _, err := client.Conn.Write(buf.Bytes()); err != nil {
		msg := fmt.Sprintf("Error when write '%s' to Conn of node: %s", buf.String(), nodeName)
		klog.Errorf(msg)
		return fmt.Errorf(msg)
	}

	return nil
}
