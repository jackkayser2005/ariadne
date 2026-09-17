package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func benchmarkTrace(large bool) Document {
	d := Document{SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete, Events: []Event{}}
	if !large {
		return completeTrace()
	}
	for _, s := range []string{"android", "browser", "desktop", "proxy"} {
		for _, c := range []string{"app-storage", "cookie", "network", "web-storage"} {
			for _, k := range []string{"beacon", "cookie-write", "request", "response", "storage-write"} {
				for _, to := range []string{"advertising", "analytics", "crash-reporting", "first-party", "unknown"} {
					d.Events = append(d.Events, Event{s, c, k, to, []string{"account-id", "advertising-id", "consent", "cookie-id", "device-id", "email", "ip-address", "location", "phone", "region", "session-id", "unknown", "user-agent"}})
				}
			}
		}
	}
	return d
}
func BenchmarkTraceCompare(b *testing.B) {
	for _, large := range []bool{false, true} {
		b.Run(fmt.Sprintf("large=%t", large), func(b *testing.B) {
			a, c := benchmarkTrace(large), benchmarkTrace(large)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Compare(a, c); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkTraceVerify(b *testing.B) {
	for _, large := range []bool{false, true} {
		b.Run(fmt.Sprintf("large=%t", large), func(b *testing.B) {
			p := filepath.Join(b.TempDir(), "trace.json")
			data, _ := json.Marshal(benchmarkTrace(large))
			if err := os.WriteFile(p, data, 0600); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Verify(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkCaseAssembly(b *testing.B) {
	root := b.TempDir()
	archive, round := writeCaseArchive(b, root)
	plan := CaseAssemblyPlan{SchemaVersion: 1, OrderBasis: "caller", Entries: []CaseAssemblyPlanEntry{{Kind: CaseEntryTraceArchive, ArtifactPath: archive, QuestionRoundPath: round}}}
	data, _ := json.Marshal(plan)
	p := filepath.Join(root, "plan.json")
	os.WriteFile(p, data, 0600)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := AssembleCase(p, filepath.Join(root, fmt.Sprintf("out-%d", i))); err != nil {
			b.Fatal(err)
		}
	}
}
