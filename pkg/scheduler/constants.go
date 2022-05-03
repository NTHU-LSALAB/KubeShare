package scheduler

const (
	domain = "sharedgpu/"
	// the name of a pod group that defines a coscheduling pod group.
	PodGroupName = domain + "group_name"
	// the minimum number of pods to be scheduled together in a pod group.
	PodGroupMinAvailable = domain + "min_available"
	// the priority of pod
	// Note: pod in the same PodGroup should have same priority.
	PodPriority = domain + "priority"
	// the upper limit percentage of time over the past sample period that one or more kernels of the pod are executed on the GPU
	PodGPULimit = domain + "gpu_limit"
	// the minimum request percentage of time over the past sample period that one or more kernels of the pod are executed on the GPU
	PodGPURequest = domain + "gpu_request"
	// the gpu memory request (in Byte)
	PodGPUMemory = domain + "gpu_mem"
	// the gpu model request
	PodGPUModel = domain + "gpu_model"

	PodGPUUUID     = domain + "gpu_uuid"
	PodCellID      = domain + "cell_id"
	PodManagerPort = domain + "gpu_manager_port"
)
