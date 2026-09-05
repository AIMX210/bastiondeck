package audit_test

import (
	"context"
	"sync"
	"testing"

	"bastiondeck/internal/audit"
)

// 并发写入下哈希链不得分叉：每个 Write 必须原子地「读 prevHash + 追加」。
// 此前 SELECT 与 INSERT 分离，两个并发 Write 会读到同一条 prevHash，
// 导致合法写入被 Verify 判定为篡改。
func TestChainConcurrentWritesVerify(t *testing.T) {
	s, closeFn := newSvc(t)
	defer closeFn()
	ctx := context.Background()

	const n = 25
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Write(ctx, audit.Actor{ID: "u", Name: "alice"},
				"test.action", "thing", "id", "success", map[string]any{"i": i})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	rep, err := s.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("chain broken under concurrency: brokenAt=%d reason=%s", rep.BrokenAt, rep.Reason)
	}
	if rep.Checked != n {
		t.Fatalf("checked %d, want %d", rep.Checked, n)
	}
}
