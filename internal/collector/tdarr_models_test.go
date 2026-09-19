package collector

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestFlexInt_Unmarshal covers the JSON number shapes Tdarr emits for its count
// fields. The motivating case is issue #126: encoding/json rejects 847.0 when the
// target is a plain int, so flexInt must accept both integer and float encodings.
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
		{name: "negative float", in: `-12.0`, want: -12},
		{name: "zero", in: `0`, want: 0},
		{name: "exponent notation", in: `8.47e2`, want: 847},
		{name: "truncates fraction toward zero", in: `847.9`, want: 847},
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

// TestTdarrMetric_FloatEncodedCounters decodes the exact payload shape reported in
// issue #126, where Tdarr serializes the StatisticsJSONDB counters as floats.
func TestTdarrMetric_FloatEncodedCounters(t *testing.T) {
	t.Parallel()

	const body = `{
		"_id": "statistics",
		"totalFileCount": 2431,
		"totalTranscodeCount": 847.0,
		"totalHealthCheckCount": 1468.0,
		"sizeDiff": 86.57081179134545,
		"tdarrScore": "35.12",
		"healthCheckScore": "100.0",
		"table0Count": 0,
		"table1Count": 1570.0,
		"table2Count": 853.0,
		"table3Count": 6,
		"table4Count": 0,
		"table5Count": 743.0,
		"table6Count": 0
	}`

	var m TdarrMetric
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("json.Unmarshal(TdarrMetric): unexpected error: %v", err)
	}

	if m.TotalFileCount != 2431 {
		t.Errorf("TotalFileCount = %d, want 2431", m.TotalFileCount)
	}
	if m.TotalTranscodeCount != 847 {
		t.Errorf("TotalTranscodeCount = %d, want 847", m.TotalTranscodeCount)
	}
	if m.TotalHealthCheckCount != 1468 {
		t.Errorf("TotalHealthCheckCount = %d, want 1468", m.TotalHealthCheckCount)
	}
	if m.HoldQueue != 0 || m.TranscodeQueue != 1570 || m.TranscodeSuccess != 853 ||
		m.TranscodeFailed != 6 || m.HealthCheckQueue != 0 || m.HealthCheckSuccess != 743 ||
		m.HealthCheckFailed != 0 {
		t.Errorf("queue buckets decoded incorrectly: %+v", m)
	}
}

// TestTdarrPieStat_FloatEncodedCounters guards the same float encoding on the
// per-library get-pies payload, which shares the totalTranscodeCount /
// totalHealthCheckCount field names with the general-stats document.
func TestTdarrPieStat_FloatEncodedCounters(t *testing.T) {
	t.Parallel()

	const body = `{
		"totalFiles": 10.0,
		"totalTranscodeCount": 2.0,
		"totalHealthCheckCount": 1.0,
		"sizeDiff": 0.5
	}`

	var p TdarrPieStat
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("json.Unmarshal(TdarrPieStat): unexpected error: %v", err)
	}
	if p.TotalFiles != 10 || p.TotalTranscodeCount != 2 || p.TotalHealthCheckCount != 1 {
		t.Errorf("pie counts decoded incorrectly: %+v", p)
	}
}

// TestCollect_FloatEncodedCounts_Issue126 is an end-to-end regression test: a
// StatisticsJSONDB response whose counters are floats must not fail the scrape.
// Before flexInt this returned tdarr_up=0 and emitted no metrics at all.
func TestCollect_FloatEncodedCounts_Issue126(t *testing.T) {
	cfg := newTestConfig(t)
	api := newSuccessFakeAPI(cfg)
	api.setResponse(fakeKey{path: cfg.TdarrStatsPath, disc: "StatisticsJSONDB"}, []byte(`{
		"totalFileCount": 2431,
		"totalTranscodeCount": 847.0,
		"totalHealthCheckCount": 1468.0,
		"sizeDiff": 86.5,
		"tdarrScore": "35.12",
		"healthCheckScore": "100.0",
		"table1Count": 1570.0
	}`))
	c := newTdarrCollectorWithAPI(cfg, api)

	samples := collectSamples(t, func(ch chan<- prometheus.Metric) {
		c.Collect(ch)
	})

	if got := findOne(t, samples, "tdarr_up", nil).value; got != 1 {
		t.Errorf("tdarr_up = %v, want 1 (float-encoded counts must not fail the scrape)", got)
	}
	if got := findOne(t, samples, "tdarr_transcodes_completed", nil).value; got != 847 {
		t.Errorf("tdarr_transcodes_completed = %v, want 847", got)
	}
	if got := findOne(t, samples, "tdarr_health_checks_completed", nil).value; got != 1468 {
		t.Errorf("tdarr_health_checks_completed = %v, want 1468", got)
	}
}

// TestFlexInt_ErrorMentionsValue keeps the decode failure diagnosable if Tdarr
// ever sends something genuinely non-numeric.
func TestFlexInt_ErrorMentionsValue(t *testing.T) {
	t.Parallel()

	var got flexInt
	err := json.Unmarshal([]byte(`"abc"`), &got)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "abc") {
		t.Errorf("error %q should mention the offending value", err)
	}
}
