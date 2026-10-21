package v6_stats

import "fmt"

type cacheStats struct {
	hit  uint64
	miss uint64
	del  uint64
	upd  uint64
}

func (s *cacheStats) String() string {
	return fmt.Sprintf("hit=%d; miss=%d; del=%d; upd=%d; HitRate=%.2f; MissRate=%.2f\n", s.hit, s.miss, s.del, s.upd, s.hitRate(), s.missRate())
}

func (s *cacheStats) hitRate() float64 {
	if total := s.hit + s.miss; total > 0 {
		return float64(s.hit) / float64(total)
	}
	return 0
}

func (s *cacheStats) missRate() float64 {
	if total := s.hit + s.miss; total > 0 {
		return float64(s.miss) / float64(total)
	}
	return 0
}
