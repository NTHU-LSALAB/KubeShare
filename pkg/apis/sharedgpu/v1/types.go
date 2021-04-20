/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"bytes"
	"math/rand"
	"time"
	"unsafe"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

type SharePodPhase string

const (
	letterIdxBits = 5                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	letterBytes   = "abcdefghijklmnopqrstuvwxyz"
	// letterIdxBits = 6                    // 6 bits to represent a letter index
	// letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	// letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
	// letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// kubeshare constants
	KubeShareResourceGPURequest = "sharedgpu/gpu_request"
	KubeShareResourceGPULimit   = "sharedgpu/gpu_limit"
	KubeShareResourceGPUMemory  = "sharedgpu/gpu_mem"
	KubeShareResourceGPUID      = "sharedgpu/GPUID"
	KubeShareDummyPodName       = "sharedgpu-vgpu"
	KubeShareNodeName           = "sharedgpu/nodeName"
	KubeShareRole               = "sharedgpu/role"
	KubeShareNodeGPUInfo        = "sharedgpu/gpu_info"
	ResourceNVIDIAGPU           = "nvidia.com/gpu"

	// Phase : the condition of a sharepod at the current time
	SharePodPending   SharePodPhase = "Pending"
	SharePodFailed    SharePodPhase = "Failed"
	SharePodSucceeded SharePodPhase = "Succeeded"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SharePod struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +optional
	Status SharePodStatus `json:"status,omitempty"`
	// +optional
	Spec corev1.PodSpec `json:"spec,omitempty"`
}

type SharePodStatus struct {
	// The phase of a sharepod is align with pod phase
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-phase
	// +optional
	Phase SharePodPhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=PodPhase"`
	// A brief CamelCase message indicating details about why the pod is in this state.
	// e.g. 'Evicted'
	// +optional
	// Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about why the pod is in this condition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,3,opt,name=message"`

	/*
		ConfigFilePhase   ConfigFilePhase
		BoundDeviceID     string
		StartTime         *metav1.Time
		ContainerStatuses []corev1.ContainerStatus*/
	PodStatus      *corev1.PodStatus  `json:"podStatus,omitempty" protobuf:"bytes,2,opt,name=podStatus"`
	PodObjectMeta  *metav1.ObjectMeta `json:"podObjectMeta,omitempty" protobuf:"bytes,1,opt,name=podObjectMeta"`
	BoundDeviceID  string             `json:"boundDeviceID,omitempty" protobuf:"bytes,2,opt,name=boundDeviceID"`
	PodManagerPort int                `json:"port" protobuf:"varint,3,opt,name=podManagerPort"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TestTypeList is a top-level list type. The client methods for lists are automatically created.
// You are not supposed to create a separated client for this one.
type SharePodList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SharePod `json:"items"`
}

func (shp SharePod) Print() {
	var buf bytes.Buffer
	buf.WriteString("\n================= SharePod ==================")
	buf.WriteString("\nname: ")
	buf.WriteString(shp.ObjectMeta.Namespace)
	buf.WriteString("/")
	buf.WriteString(shp.ObjectMeta.Name)
	buf.WriteString("\nannotation:\n\tkubeshare/gpu_request: ")
	buf.WriteString(shp.ObjectMeta.Annotations["kubeshare/gpu_request"])
	if shp.Status.PodStatus != nil {
		buf.WriteString("\nstatus:\n\tPodStatus: ")
		buf.WriteString(string(shp.Status.PodStatus.Phase))
	}
	buf.WriteString("\n\tGPUID: ")
	buf.WriteString(shp.ObjectMeta.Annotations["kubeshare/GPUID"])
	buf.WriteString("\n\tBoundDeviceID: ")
	buf.WriteString(shp.Status.BoundDeviceID)
	buf.WriteString("\n=============================================")
	klog.Info(buf.String())
}

// https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/31832326#31832326
var src = rand.NewSource(time.Now().UnixNano())

func NewGPUID(n int) string {
	b := make([]byte, n)
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}
	return *(*string)(unsafe.Pointer(&b))
}
