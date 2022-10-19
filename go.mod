module KubeShare

go 1.16

require (
	github.com/NVIDIA/go-nvml v0.11.6-0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/googleapis/gnostic v0.3.1 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/onsi/gomega v1.10.1 // indirect
	github.com/prometheus/client_golang v1.0.0
	github.com/prometheus/common v0.4.1
	github.com/prometheus/procfs v0.0.11 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/viper v1.3.2
	go.uber.org/atomic v1.4.0 // indirect
	golang.org/x/text v0.3.3 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.18.10
	k8s.io/apimachinery v0.18.10
	k8s.io/client-go v0.18.10
	k8s.io/kubernetes v1.18.10
	k8s.io/utils v0.0.0-20200603063816-c1c6865ac451 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.18.10
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.18.10
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.11-rc.0
	k8s.io/apiserver => k8s.io/apiserver v0.18.10
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.18.10
	k8s.io/client-go => k8s.io/client-go v0.18.10
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.18.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.18.10
	k8s.io/code-generator => k8s.io/code-generator v0.18.18-rc.0
	k8s.io/component-base => k8s.io/component-base v0.18.10
	k8s.io/cri-api => k8s.io/cri-api v0.18.18-rc.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.18.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.18.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.18.10
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.18.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.18.10
	k8s.io/kubectl => k8s.io/kubectl v0.18.10
	k8s.io/kubelet => k8s.io/kubelet v0.18.10
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.18.10
	k8s.io/metrics => k8s.io/metrics v0.18.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.18.10
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.18.10
	k8s.io/sample-controller => k8s.io/sample-controller v0.18.10
)
