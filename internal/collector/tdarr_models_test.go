package collector

import (
	"encoding/json"
	"testing"
)

// TestFlexInt_Unmarshal covers the JSON number shapes Tdarr emits for its count
// fields. Tdarr is MongoDB-backed, so the same logical count can arrive as an
// integer or a double depending on the stored BSON type; encoding/json rejects
// the double form when the target is a plain int. flexInt must accept both.
func TestFlexInt_Unmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    flexInt
		wantErr bool
	}{
		{name: "integer", in: `847`, want: 847},
		{name: "float with zero fraction", in: `847.0`, want: 847},
		{name: "negative float", in: `-1.0`, want: -1},
		{name: "zero float", in: `0.0`, want: 0},
		{name: "exponent notation", in: `8.47e2`, want: 847},
		{name: "truncates fraction toward zero", in: `847.9`, want: 847},
		{name: "large int64 keeps precision", in: `1790015339208`, want: 1790015339208},
		{name: "null leaves zero value", in: `null`, want: 0},
		{name: "non-numeric string error", in: `"nope"`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got flexInt
			err := json.Unmarshal([]byte(tc.in), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("json.Unmarshal(%s): want error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("json.Unmarshal(%s): unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("json.Unmarshal(%s) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestTdarrMetric_FloatEncodedCounters is a regression test for issue #126:
// Tdarr serializes totalTranscodeCount / totalHealthCheckCount as JSON floats
// (e.g. 847.0), which encoding/json cannot decode into an int. Decoding the
// exact reported payload must succeed.
func TestTdarrMetric_FloatEncodedCounters(t *testing.T) {
	t.Parallel()

	const body = `{
		"totalFileCount": 2431,
		"totalTranscodeCount": 847.0,
		"totalHealthCheckCount": 1468.0,
		"sizeDiff": 86.57081179134545,
		"tdarrScore": "35.12",
		"healthCheckScore": "100.0",
		"table1Count": 1570,
		"table2Count": 853,
		"table3Count": 6,
		"table5Count": 743
	}`

	var m TdarrMetric
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("json.Unmarshal(TdarrMetric): unexpected error: %v", err)
	}
	if m.TotalTranscodeCount != 847 {
		t.Errorf("TotalTranscodeCount = %v, want 847", m.TotalTranscodeCount)
	}
	if m.TotalHealthCheckCount != 1468 {
		t.Errorf("TotalHealthCheckCount = %v, want 1468", m.TotalHealthCheckCount)
	}
	if m.TotalFileCount != 2431 {
		t.Errorf("TotalFileCount = %d, want 2431", m.TotalFileCount)
	}
}

// TestTdarrPieStats_FloatEncodedCounters is a regression test for the pie-stats
// decode failure against Tdarr 2.87: /api/v2/stats/get-pies serializes
// totalTranscodeCount and totalHealthCheckCount as doubles (e.g. 891.0), which
// aborted the whole decode and dropped every per-library series.
func TestTdarrPieStats_FloatEncodedCounters(t *testing.T) {
	t.Parallel()

	const body = `{
		"pieStats": {
			"totalFiles": 500,
			"totalTranscodeCount": 891.0,
			"sizeDiff": 0.0,
			"totalHealthCheckCount": 13.0,
			"status": {
				"transcode": [{"name": "Transcode success", "value": 700}],
				"healthCheck": [{"name": "Health check success", "value": 450}]
			}
		}
	}`

	var p TdarrPieStats
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("json.Unmarshal(TdarrPieStats): unexpected error: %v", err)
	}
	if p.PieStats.TotalTranscodeCount != 891 {
		t.Errorf("TotalTranscodeCount = %v, want 891", p.PieStats.TotalTranscodeCount)
	}
	if p.PieStats.TotalHealthCheckCount != 13 {
		t.Errorf("TotalHealthCheckCount = %v, want 13", p.PieStats.TotalHealthCheckCount)
	}
	if p.PieStats.TotalFiles != 500 {
		t.Errorf("TotalFiles = %v, want 500", p.PieStats.TotalFiles)
	}
}

// TestTdarrNode_FloatEncodedNumbers is a regression test for the node decode
// failure against Tdarr 2.87: /api/v2/get-nodes serializes config.priority and
// the top-level priority as doubles (-1.0, 0.0), which aborted the whole decode
// and dropped every node series.
func TestTdarrNode_FloatEncodedNumbers(t *testing.T) {
	t.Parallel()

	const body = `{
		"CGCeQWt": {
			"_id": "CGCeQWt",
			"nodeName": "worker",
			"config": {"priority": -1.0, "processPid": 360},
			"priority": 0.0,
			"maxGpuWorkers": 100,
			"workerLimits": {"transcodecpu": 1, "transcodegpu": 1.0, "healthcheckcpu": 1, "healthcheckgpu": 1},
			"queueLengths": {"transcodecpu": 20.0, "transcodegpu": 0},
			"resStats": {"process": {"uptime": 238074.0, "heapUsedMB": "128.5", "heapTotalMB": "256.0"}},
			"workers": {
				"w1": {"_id": "w1", "fps": 333.0, "statusTs": 1790015339208.0, "startTime": 1790014655109.0}
			}
		}
	}`

	var nodes map[string]TdarrNode
	if err := json.Unmarshal([]byte(body), &nodes); err != nil {
		t.Fatalf("json.Unmarshal(map[string]TdarrNode): unexpected error: %v", err)
	}
	node, ok := nodes["CGCeQWt"]
	if !ok {
		t.Fatal("node CGCeQWt missing from decoded map")
	}
	if node.Config.Priority != -1 {
		t.Errorf("Config.Priority = %v, want -1", node.Config.Priority)
	}
	if node.Priority != 0 {
		t.Errorf("Priority = %v, want 0", node.Priority)
	}
	if node.Config.Pid != 360 {
		t.Errorf("Config.Pid = %v, want 360", node.Config.Pid)
	}
	if node.ResourceStats.Process.Uptime != 238074 {
		t.Errorf("Process.Uptime = %v, want 238074", node.ResourceStats.Process.Uptime)
	}
	if node.QueueLengths.TranscodeCpu != 20 {
		t.Errorf("QueueLengths.TranscodeCpu = %v, want 20", node.QueueLengths.TranscodeCpu)
	}
	w, ok := node.Workers["w1"]
	if !ok {
		t.Fatal("worker w1 missing from decoded map")
	}
	if w.Fps != 333 {
		t.Errorf("Fps = %v, want 333", w.Fps)
	}
	if w.StatusTs != 1790015339208 {
		t.Errorf("StatusTs = %v, want 1790015339208", w.StatusTs)
	}
	if w.StartTime != 1790014655109 {
		t.Errorf("StartTime = %v, want 1790014655109", w.StartTime)
	}
}
