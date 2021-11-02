package scheduler

/* ------------------- struct NodeResources start ------------------- */

// NodeResources: Available resources in cluster to schedule Training Jobs
type NodeResources map[string]*NodeResource

func (this *NodeResources) DeepCopy() *NodeResources {
	copy := make(NodeResources, len(*this))
	for k, v := range *this {
		copy[k] = v.DeepCopy()
	}
	return &copy
}

func (this *NodeResources) PrintMe() {
	for name, res := range *this {
		ksl.Debugf("============ Node: %s ============", name)
		ksl.Debugf("CpuTotal: %d", res.CpuTotal)
		ksl.Debugf("MemTotal: %d", res.MemTotal)
		ksl.Debugf("GpuTotal: %d", res.GpuTotal)
		ksl.Debugf("GpuMemTotal: %d", res.GpuMemTotal)
		ksl.Debugf("CpuFree: %d", res.CpuFree)
		ksl.Debugf("MemFree: %d", res.MemFree)
		ksl.Debugf("GpuFree: %d", res.GpuFreeCount)
		ksl.Debugf("GpuId:")
		for id, gpu := range res.GpuFree {
			ksl.Debugf("    %s: %d, %d", id, (*gpu).GPUFreeReq, (*gpu).GPUFreeMem)
		}
	}
	ksl.Debugf("============ Node Info End ============")
}

/* ------------------- struct NodeResources end ------------------- */

/* ------------------- struct NodeResource start ------------------- */

type NodeResource struct {
	CpuTotal int64
	MemTotal int64
	GpuTotal int
	// GpuMemTotal in bytes
	GpuMemTotal int64
	CpuFree     int64
	MemFree     int64
	/* Available GPU calculate */
	// Total GPU count - Pods using nvidia.com/gpu
	GpuFreeCount int
	// GPUs available usage (1.0 - SharePod usage)
	// GPUID to integer index mapping
	GpuFree map[string]*GPUInfo
}

func (this *NodeResource) DeepCopy() *NodeResource {
	gpuFreeCopy := make(map[string]*GPUInfo, len(this.GpuFree))
	for k, v := range this.GpuFree {
		gpuFreeCopy[k] = v.DeepCopy()
	}
	return &NodeResource{
		CpuTotal:     this.CpuTotal,
		MemTotal:     this.MemTotal,
		GpuTotal:     this.GpuTotal,
		GpuMemTotal:  this.GpuMemTotal,
		CpuFree:      this.CpuFree,
		MemFree:      this.MemFree,
		GpuFreeCount: this.GpuFreeCount,
		GpuFree:      gpuFreeCopy,
	}
}

/* ------------------- struct NodeResource end ------------------- */

/* ------------------- struct GPUInfo start ------------------- */

type GPUInfo struct {
	GPUFreeReq int64
	// GPUFreeMem in bytes
	GPUFreeMem int64

	GPUAffinityTags     []string
	GPUAntiAffinityTags []string
	// len(GPUExclusionTags) should be only one
	GPUExclusionTags []string
}

func (this *GPUInfo) DeepCopy() *GPUInfo {
	var tmpGPUAffinityTags []string
	var tmpGPUAntiAffinityTags []string
	var tmpGPUExclusionTags []string
	for _, v := range this.GPUAffinityTags {
		tmpGPUAffinityTags = append(tmpGPUAffinityTags, v)
	}
	for _, v := range this.GPUAntiAffinityTags {
		tmpGPUAntiAffinityTags = append(tmpGPUAntiAffinityTags, v)
	}
	for _, v := range this.GPUExclusionTags {
		tmpGPUExclusionTags = append(tmpGPUExclusionTags, v)
	}
	return &GPUInfo{
		GPUFreeReq:          this.GPUFreeReq,
		GPUFreeMem:          this.GPUFreeMem,
		GPUAffinityTags:     tmpGPUAffinityTags,
		GPUAntiAffinityTags: tmpGPUAntiAffinityTags,
		GPUExclusionTags:    tmpGPUExclusionTags,
	}
}

/* ------------------- struct GPUInfo end ------------------- */
