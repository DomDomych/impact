package metrics

import "time"

type Stats struct {
	Completed, Failed int64
	Histogram         [10001]int64
	FirstError        error
}

func (s *Stats) Record(d time.Duration) {
	s.Completed++
	i := int(d / time.Millisecond)
	if i > 10000 {
		i = 10000
	}
	s.Histogram[i]++
}
func (s *Stats) Fail(err error) {
	s.Failed++
	if s.FirstError == nil {
		s.FirstError = err
	}
}
func (s *Stats) Merge(other Stats) {
	s.Completed += other.Completed
	s.Failed += other.Failed
	for i, n := range other.Histogram {
		s.Histogram[i] += n
	}
	if s.FirstError == nil {
		s.FirstError = other.FirstError
	}
}
func (s Stats) Percentile(p int) time.Duration {
	if s.Completed == 0 {
		return 0
	}
	target := (s.Completed*int64(p) + 99) / 100
	var n int64
	for i, v := range s.Histogram {
		n += v
		if n >= target {
			return time.Duration(i) * time.Millisecond
		}
	}
	return 10 * time.Second
}
