package webcompat

import "testing"

func TestNextHLSBatchStart(t *testing.T) {
	tests := []struct {
		name          string
		segment       int
		batchSegments int
		maxSegments   int
		want          int
		ok            bool
	}{
		{name: "first batch", segment: 0, batchSegments: 4, maxSegments: 11, want: 4, ok: true},
		{name: "middle of first batch", segment: 2, batchSegments: 4, maxSegments: 11, want: 4, ok: true},
		{name: "second batch", segment: 4, batchSegments: 4, maxSegments: 11, want: 8, ok: true},
		{name: "last partial batch", segment: 8, batchSegments: 4, maxSegments: 11, want: 0, ok: false},
		{name: "invalid batch size", segment: 1, batchSegments: 0, maxSegments: 11, want: 0, ok: false},
		{name: "invalid segment", segment: -1, batchSegments: 4, maxSegments: 11, want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nextHLSBatchStart(tt.segment, tt.batchSegments, tt.maxSegments)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("nextHLSBatchStart(%d, %d, %d) = (%d, %v), want (%d, %v)", tt.segment, tt.batchSegments, tt.maxSegments, got, ok, tt.want, tt.ok)
			}
		})
	}
}
