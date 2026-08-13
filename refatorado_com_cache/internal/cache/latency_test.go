package cache

import (
	"testing"
	"time"
)

func TestLatencyWindowReportsHitAndMissPercentilesInMilliseconds(t *testing.T) {
	window := &LatencyWindow{}
	window.RecordHit(time.Millisecond)
	window.RecordHit(2 * time.Millisecond)
	window.RecordHit(3 * time.Millisecond)
	window.RecordMiss(10 * time.Millisecond)

	snapshot := window.Snapshot()
	if snapshot.HitCount != 3 || snapshot.HitP50MS != 2 || snapshot.HitP95MS != 3 {
		t.Fatalf("hit snapshot = %+v", snapshot)
	}
	if snapshot.MissCount != 1 || snapshot.MissP50MS != 10 || snapshot.MissP95MS != 10 {
		t.Fatalf("miss snapshot = %+v", snapshot)
	}
}
