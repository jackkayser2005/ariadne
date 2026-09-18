package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkHARFullReport(b *testing.B) {
	for _, count := range []int{20, 10000} {
		b.Run(fmt.Sprintf("origins-%d", count), func(b *testing.B) {
			root := b.TempDir()
			input := filepath.Join(root, "capture.har")
			rules := filepath.Join(root, "rules.json")
			var raw strings.Builder
			raw.WriteString(`{"log":{"version":"1.2","entries":[`)
			for i := 0; i < count; i++ {
				if i > 0 {
					raw.WriteByte(',')
				}
				fmt.Fprintf(&raw, `{"request":{"url":"https://origin-%d.test/?x=test-person%%40example.test"},"comment":"%s"}`, i, strings.Repeat("x", 650))
			}
			raw.WriteString(`]}}`)
			if raw.Len() > maxHARBytes {
				b.Fatal("fixture exceeds input limit")
			}
			if err := os.WriteFile(input, []byte(raw.String()), 0600); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(rules, []byte(validRules), 0600); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(raw.Len()))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				report, err := ExportHARReport(input, "https://example.test", rules)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(report)), "output-B")
			}
		})
	}
}
