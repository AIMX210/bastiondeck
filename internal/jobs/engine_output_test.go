package jobs_test

import (
	"context"
	"testing"
	"time"

	"bastiondeck/internal/jobs"
	"bastiondeck/internal/testutil"
)

// 回归：SSH 执行的产物文件不得重复输出。
// sshlite.Client 在 OnOutput 回调里已把全部输出交给引擎落盘；
// 此前引擎又把 res.Stdout 追加一次，导致 stdout/stderr 文件内容翻倍。
func TestRunArtifactOutputNotDuplicated(t *testing.T) {
	eng, repo, h := newEngine(t)
	srv := testutil.NewFakeSSH(t, "pw", "", func(string) ([]byte, []byte, int) {
		return []byte("line-one\nline-two\n"), nil, 0
	})
	defer srv.Close()
	addr, port := srv.Addr()
	cred := h.MustCredential("c", "pw")
	h1 := h.MustHost("a", addr, port, "tester", cred)

	runID, err := eng.StartRun(context.Background(), jobs.StartInput{
		Command: "echo x", TargetIDs: []string{h1.ID}, Concurrency: 1,
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := waitTerminal(t, repo, runID)
	if run.Status != jobs.StatusSuccess {
		t.Fatalf("want success got %s: %+v", run.Status, run.Targets)
	}
	if len(run.Targets) != 1 {
		t.Fatalf("targets %+v", run.Targets)
	}
	data, _, err := eng.ReadOutput(context.Background(), runID, run.Targets[0].ID, "stdout", 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := "line-one\nline-two\n"; string(data) != want {
		t.Fatalf("artifact stdout = %q, want %q (duplication?)", data, want)
	}
}
