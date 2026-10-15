package monitor

import (
	"slices"
	"testing"
)

func TestRing(t *testing.T) {
	tests := []struct {
		name string
		size int
		add  []float64
		want []float64
	}{
		{"empty", 3, nil, []float64{}},
		{"partially filled keeps insertion order", 3, []float64{1, 2}, []float64{1, 2}},
		{"exactly full", 3, []float64{1, 2, 3}, []float64{1, 2, 3}},
		{"overwrites the oldest", 3, []float64{1, 2, 3, 4, 5}, []float64{3, 4, 5}},
		{"wraps more than once", 2, []float64{1, 2, 3, 4, 5, 6, 7}, []float64{6, 7}},
		{"size one", 1, []float64{8, 9}, []float64{9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRing(tt.size)
			for _, v := range tt.add {
				r.add(v)
			}
			if got := r.values(); !slices.Equal(got, tt.want) {
				t.Fatalf("values() = %v, want %v", got, tt.want)
			}
		})
	}
}
