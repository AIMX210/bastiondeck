package jobs

import "testing"

func TestTransitionLegality(t *testing.T) {
	if !CanTransitionTarget(StatusPending, StatusRunning) {
		t.Fatal("pending->running legal")
	}
	if CanTransitionTarget(StatusSuccess, StatusRunning) {
		t.Fatal("success is terminal")
	}
	if CanTransitionTarget(StatusRunning, StatusPending) {
		t.Fatal("cannot move backwards")
	}
}

func TestAggregate(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"all ok", []string{"success", "success"}, "success"},
		{"one fail", []string{"success", "failed"}, "failed"},
		{"timeout wins over fail priority? failed first", []string{"failed", "timeout"}, "timeout"},
		{"all cancelled", []string{"cancelled", "cancelled"}, "cancelled"},
		{"lost", []string{"success", "lost"}, "lost"},
		{"still running", []string{"success", "running"}, "running"},
		{"skipped only ok", []string{"success", "skipped"}, "success"},
	}
	for _, c := range cases {
		if got := Aggregate(c.in); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
	s := Summarise([]string{"success", "failed", "failed", "lost"})
	if s.Success != 1 || s.Failed != 2 || s.Lost != 1 || s.Total != 4 {
		t.Fatalf("summary wrong %+v", s)
	}
}
