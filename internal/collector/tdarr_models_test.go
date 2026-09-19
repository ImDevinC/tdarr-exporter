package collector

import (
	"encoding/json"
	"testing"
)

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
