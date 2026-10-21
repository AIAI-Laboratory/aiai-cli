package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AIAI-Laboratory/aiai-cli/internal/monitor"
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
)

type fakeMetrics struct{ calls int }

func (f *fakeMetrics) Collect(context.Context) (monitor.Snapshot, error) {
	f.calls++
	return monitor.Snapshot{System: monitor.System{OS: "test", Architecture: "arm64"}, Processes: []monitor.Process{}}, nil
}

func TestSystemJSONAndMonitorValidation(t *testing.T) {
	for _, args := range [][]string{{"system", "info", "--json"}, {"monitor", "--json"}, {"monitor", "--interval", "10ms"}, {"monitor"}} {
		var out, stderr bytes.Buffer
		collector := &fakeMetrics{}
		code := Run(context.Background(), args, Dependencies{In: strings.NewReader(""), Out: &out, Err: &stderr, Monitor: collector, Interactive: func() bool { return false }})
		if args[0] == "system" {
			var envelope output.Envelope
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if code != 0 || envelope.System == nil || envelope.System.System.OS != "test" || collector.calls != 2 {
				t.Fatalf("JSON snapshot: code%d out%s", code, out.String())
			}
		} else if code == 0 || collector.calls != 0 {
			t.Fatalf("monitor validation: %v code%d", args, code)
		}
	}
}
