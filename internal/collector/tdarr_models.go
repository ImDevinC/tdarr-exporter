package collector

import (
	"fmt"
	"math"
	"strconv"
)

// flexInt is an integer count that tolerates Tdarr's inconsistent JSON number
// encoding. Tdarr (backed by MongoDB) sometimes serializes whole-number counters
// as floats, e.g. "totalTranscodeCount": 847.0, which encoding/json refuses to
// unmarshal into a plain int:
//
//	json: cannot unmarshal number 847.0 into Go value of type int
//
// flexInt accepts both encodings: it first tries a strict integer parse and, if
// that fails, parses as a float and truncates toward zero. Every field using
// flexInt is semantically a count, so truncation is lossless for integral values
// and the best available approximation for the malformed fractional case.
type flexInt int

// UnmarshalJSON implements json.Unmarshaler. It accepts JSON integers (847),
// floats (847.0, 8.47e2) and null (leaving the value untouched, matching
// encoding/json's native handling of null for numeric fields).
func (f *flexInt) UnmarshalJSON(data []byte) error {
	raw := string(data)
	if raw == "null" {
		return nil
	}
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
	Status  string `json:"status"`
	Version string `json:"version"`
	Os      string `json:"os"`
	Uptime  int64  `json:"uptime"`
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
	Average int64 `json:"average"`
	Highest int64 `json:"highest"`
	Total   int64 `json:"total"`
}

type TdarrStreamStats struct {
	Duration  TdarrStreamStatsObj `json:"duration"`
	BitRate   TdarrStreamStatsObj `json:"bit_rate"`
	NumFrames TdarrStreamStatsObj `json:"nb_frames"`
}

type TdarrResourceStats struct {
	Process struct {
		Uptime      int64  `json:"uptime"`
		HeapUsedMb  string `json:"heapUsedMB"`
		HeapTotalMb string `json:"heapTotalMB"`
	} `json:"process"`
	Os struct {
		CpuPercent string `json:"cpuPerc"`
		MemUsedGb  string `json:"memUsedGB"`
		MemTotalGb string `json:"memTotalGB"`
	} `json:"os"`
}

type TdarrNode struct {
	Id              string                      `json:"_id"`
	Name            string                      `json:"nodeName"`
	RemoteAddress   string                      `json:"remoteAddress"`
	Config          TdarrNodeConfig             `json:"config"`
	WorkerLimits    TdarrNodeJobs               `json:"workerLimits"`
	GpuSelect       string                      `json:"gpuSelect"`
	Paused          bool                        `json:"nodePaused"`
	Priority        int                         `json:"priority"`
	Workers         map[string]TdarrNodeWorkers `json:"workers"`
	ResourceStats   TdarrResourceStats          `json:"resStats"`
	QueueLengths    TdarrNodeJobs               `json:"queueLengths"`
	MaxGpuWorkers   int                         `json:"maxGpuWorkers"`
	ScheduleEnabled bool                        `json:"scheduleEnabled"`
	AllowGpuDoCpu   bool                        `json:"allowGpuDoCpu"`
}

type TdarrNodeConfig struct {
	ServerIp   string `json:"serverIP"`
	ServerPort string `json:"serverPort"`
	Priority   int    `json:"priority"`
	Pid        int    `json:"processPid"`
}

type TdarrNodeJobs struct {
	HealthCheckCpu int `json:"healthcheckcpu"`
	HealthCheckGpu int `json:"healthcheckgpu"`
	TranscodeCpu   int `json:"transcodecpu"`
	TranscodeGpu   int `json:"transcodegpu"`
}

type TdarrNodeWorkers struct {
	Id                 string  `json:"_id"`
	WorkerType         string  `json:"workerType"`
	FlowWorker         bool    `json:"isFlowWorker"`
	Idle               bool    `json:"idle"`
	File               string  `json:"file"`
	OriginalfileSizeGb float64 `json:"originalfileSizeInGbytes"`
	Percentage         float64 `json:"percentage"`
	Fps                int     `json:"fps"`
	Eta                string  `json:"ETA"`
	Status             string  `json:"status"`
	StatusTs           int64   `json:"statusTs"`
	Job                struct {
		Version   string `json:"version"`
		StartTime int64  `json:"start"`
		Type      string `json:"type"`
		JobId     string `json:"jobId"`
	} `json:"job"`
	Process struct {
		Connected bool   `json:"connected"`
		Pid       int    `json:"pid"`
		CliType   string `json:"cliType"`
	} `json:"process"`
	LastPluginDetails struct {
		Source         string `json:"source"`
		Id             string `json:"id"`
		PositionNumber string `json:"number"`
	} `json:"lastPluginDetails"`
	StartTime        int64   `json:"startTime"` // start time of current processing step (plugin or flow step)
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
