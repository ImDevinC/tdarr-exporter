package collector

import (
	"fmt"
	"math"
	"strconv"
)

// flexInt is an integer count that tolerates Tdarr's inconsistent JSON number
// encoding. Tdarr is MongoDB-backed, so a value's JSON encoding depends on its
// stored BSON type: some counters serialize as integers (847) and others as
// doubles (847.0). encoding/json refuses to decode a number with a decimal
// point into a plain int and aborts the whole decode:
//
//	json: cannot unmarshal number 847.0 into Go value of type int
//
// flexInt accepts either encoding. Every field using it is semantically a count,
// so truncation toward zero is lossless for integral values and the best
// available approximation for a malformed fractional one.
type flexInt int64

// UnmarshalJSON accepts JSON integers (847), floats (847.0, 8.47e2) and null
// (leaving the value untouched, matching encoding/json's handling of null for
// numeric fields).
func (f *flexInt) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if raw == "null" {
		return nil
	}
	// Fast path: a plain integer (the common case) parses without going through
	// float, so large counters keep full int64 precision.
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		*f = flexInt(n)
		return nil
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("flexInt: cannot parse %q as an integer: %w", raw, err)
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Errorf("flexInt: cannot parse %q as an integer", raw)
	}
	*f = flexInt(int64(n))
	return nil
}

type TdarrMetricRequest struct {
	Data TdarrDataRequest `json:"data"`
}

type TdarrDataRequest struct {
	Collection string         `json:"collection"`
	Mode       string         `json:"mode"`
	DocId      string         `json:"docID"`
	Obj        map[string]any `json:"obj"`
}

type TdarrPieDataRequest struct {
	Data struct {
		LibraryId   string `json:"libraryId"`
		libraryName string `json:"-"`
	} `json:"data"`
}

type TdarrPieSlice struct {
	Name  string  `json:"name"`
	Value flexInt `json:"value"`
}

// core metrics
type TdarrMetric struct {
	TotalFileCount        flexInt          `json:"totalFileCount"`
	TotalTranscodeCount   flexInt          `json:"totalTranscodeCount"`
	TotalHealthCheckCount flexInt          `json:"totalHealthCheckCount"`
	SizeDiff              float64          `json:"sizeDiff"`
	TdarrScore            string           `json:"tdarrScore"`
	HealthCheckScore      string           `json:"healthCheckScore"`
	AvgNumStreams         float64          `json:"avgNumberOfStreamsInVideo"`
	StreamStats           TdarrStreamStats `json:"streamStats"`
	// Per-bucket counts for cache invalidation. Returned by the StatisticsJSONDB cruddb endpoint.
	// These map to Tdarr UI buckets: table0=Hold, table1=Transcode queue,
	// table2=Transcode success+not required, table3=Transcode error+cancelled,
	// table4=Health check queue, table5=Health check healthy, table6=Health check error+cancelled.
	// Older Tdarr versions may omit these fields; Go's JSON decoder defaults them to 0,
	// which means 0==0 comparisons never trigger spurious refetches (graceful degradation).
	HoldQueue          flexInt `json:"table0Count"`
	TranscodeQueue     flexInt `json:"table1Count"`
	TranscodeSuccess   flexInt `json:"table2Count"` // includes "not required" per Tdarr UI grouping
	TranscodeFailed    flexInt `json:"table3Count"` // includes "cancelled"
	HealthCheckQueue   flexInt `json:"table4Count"`
	HealthCheckSuccess flexInt `json:"table5Count"`
	HealthCheckFailed  flexInt `json:"table6Count"` // includes "cancelled"
}

// TdarrServerStatus decodes GET /api/v2/status. Only the fields surfaced as
// metrics/labels are mapped; isProduction/buildDate are intentionally omitted.
// uptime is Tdarr's Node.js process.uptime(), i.e. seconds.
type TdarrServerStatus struct {
	Status  string  `json:"status"`
	Version string  `json:"version"`
	Os      string  `json:"os"`
	Uptime  flexInt `json:"uptime"`
}

// new api `api/v2/stats/get-pies` support
type TdarrLibraryInfo struct {
	LibraryId string `json:"_id"`
	Name      string `json:"name"`
}

type TdarrPieStats struct {
	PieStats    TdarrPieStat `json:"pieStats"`
	libraryName string
	libraryId   string
	// NormalizedTranscodes maps cleaned transcode status labels to counts.
	// Populated by normalizePieStatuses after fetch; covers the full known enum (zeros included).
	NormalizedTranscodes map[string]int
	// NormalizedHealthChecks maps cleaned health check status labels to counts.
	// Populated by normalizePieStatuses after fetch; covers the full known enum (zeros included).
	NormalizedHealthChecks map[string]int
}

type TdarrPieStat struct {
	TotalFiles            flexInt             `json:"totalFiles"`
	TotalTranscodeCount   flexInt             `json:"totalTranscodeCount"`
	SizeDiff              float64             `json:"sizeDiff"`
	TotalHealthCheckCount flexInt             `json:"totalHealthCheckCount"`
	Status                TdarrPieStatusSlice `json:"status"`
	Video                 TdarrPieVideoSlice  `json:"video"`
	Audio                 TdarrPieVideoSlice  `json:"audio"`
}

type TdarrPieStatusSlice struct {
	Transcode   []TdarrPieSlice `json:"transcode"`
	HealthCheck []TdarrPieSlice `json:"healthCheck"`
}

type TdarrPieVideoSlice struct {
	Codecs      []TdarrPieSlice `json:"codecs"`
	Containers  []TdarrPieSlice `json:"containers"`
	Resolutions []TdarrPieSlice `json:"resolutions"`
}

type TdarrStreamStatsObj struct {
	Average flexInt `json:"average"`
	Highest flexInt `json:"highest"`
	Total   flexInt `json:"total"`
}

type TdarrStreamStats struct {
	Duration  TdarrStreamStatsObj `json:"duration"`
	BitRate   TdarrStreamStatsObj `json:"bit_rate"`
	NumFrames TdarrStreamStatsObj `json:"nb_frames"`
}

type TdarrResourceStats struct {
	Process TdarrProcessStats `json:"process"`
	Os      TdarrOsStats      `json:"os"`
}

// TdarrProcessStats is the per-node process resource block (resStats.process).
type TdarrProcessStats struct {
	Uptime      flexInt `json:"uptime"`
	HeapUsedMb  string  `json:"heapUsedMB"`
	HeapTotalMb string  `json:"heapTotalMB"`
}

// TdarrOsStats is the per-node host OS resource block (resStats.os). Values are
// numeric strings (Tdarr serializes them as strings), parsed at emit time.
type TdarrOsStats struct {
	CpuPercent string `json:"cpuPerc"`
	MemUsedGb  string `json:"memUsedGB"`
	MemTotalGb string `json:"memTotalGB"`
}

type TdarrNode struct {
	Id              string                      `json:"_id"`
	Name            string                      `json:"nodeName"`
	RemoteAddress   string                      `json:"remoteAddress"`
	Config          TdarrNodeConfig             `json:"config"`
	WorkerLimits    TdarrNodeJobs               `json:"workerLimits"`
	GpuSelect       string                      `json:"gpuSelect"`
	Paused          bool                        `json:"nodePaused"`
	Priority        flexInt                     `json:"priority"`
	Workers         map[string]TdarrNodeWorkers `json:"workers"`
	ResourceStats   TdarrResourceStats          `json:"resStats"`
	QueueLengths    TdarrNodeJobs               `json:"queueLengths"`
	MaxGpuWorkers   flexInt                     `json:"maxGpuWorkers"`
	ScheduleEnabled bool                        `json:"scheduleEnabled"`
	AllowGpuDoCpu   bool                        `json:"allowGpuDoCpu"`
}

type TdarrNodeConfig struct {
	ServerIp   string  `json:"serverIP"`
	ServerPort string  `json:"serverPort"`
	Priority   flexInt `json:"priority"`
	Pid        flexInt `json:"processPid"`
}

type TdarrNodeJobs struct {
	HealthCheckCpu flexInt `json:"healthcheckcpu"`
	HealthCheckGpu flexInt `json:"healthcheckgpu"`
	TranscodeCpu   flexInt `json:"transcodecpu"`
	TranscodeGpu   flexInt `json:"transcodegpu"`
}

type TdarrNodeWorkers struct {
	Id                 string  `json:"_id"`
	WorkerType         string  `json:"workerType"`
	FlowWorker         bool    `json:"isFlowWorker"`
	Idle               bool    `json:"idle"`
	File               string  `json:"file"`
	OriginalfileSizeGb float64 `json:"originalfileSizeInGbytes"`
	Percentage         float64 `json:"percentage"`
	Fps                flexInt `json:"fps"`
	Eta                string  `json:"ETA"`
	Status             string  `json:"status"`
	StatusTs           flexInt `json:"statusTs"`
	Job                struct {
		Version   string  `json:"version"`
		StartTime flexInt `json:"start"`
		Type      string  `json:"type"`
		JobId     string  `json:"jobId"`
	} `json:"job"`
	Process struct {
		Connected bool    `json:"connected"`
		Pid       flexInt `json:"pid"`
		CliType   string  `json:"cliType"`
	} `json:"process"`
	LastPluginDetails struct {
		Source         string `json:"source"`
		Id             string `json:"id"`
		PositionNumber string `json:"number"`
	} `json:"lastPluginDetails"`
	StartTime        flexInt `json:"startTime"` // start time of current processing step (plugin or flow step)
	OutputFileSizeGb float64 `json:"outputFileSizeInGbytes"`
	EstSizeGb        float64 `json:"estSize"`
}

type tdarrCacheTotals struct {
	totalFileCount        int
	totalTranscodeCount   int
	totalHealthCheckCount int
	holdQueue             int
	transcodeQueue        int
	transcodeSuccess      int
	transcodeFailed       int
	healthCheckQueue      int
	healthCheckSuccess    int
	healthCheckFailed     int
}
