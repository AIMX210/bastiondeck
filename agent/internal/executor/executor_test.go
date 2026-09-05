package executor

import (
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"

	"bd-agent/internal/proto"
)

func collect() (Sink, *[]proto.Frame, *sync.Mutex) {
	var mu sync.Mutex
	var frames []proto.Frame
	sink := Sink(func(f proto.Frame) {
		mu.Lock()
		frames = append(frames, f)
		mu.Unlock()
	})
	return sink, &frames, &mu
}

func TestExecSuccess(t *testing.T) {
	e := New()
	sink, frames, mu := collect()
	e.Run("1", "printf hello", 5000, sink)
	mu.Lock()
	defer mu.Unlock()
	var end *proto.Frame
	var out strings.Builder
	for i := range *frames {
		f := &(*frames)[i]
		if f.T == "exec_chunk" && f.Stream == "stdout" {
			b, _ := base64.StdEncoding.DecodeString(f.DataB64)
			out.Write(b)
		}
		if f.T == "exec_end" {
			end = f
		}
	}
	if end == nil {
		t.Fatal("no exec_end")
	}
	if end.Status != "success" || end.ExitCode != 0 {
		t.Fatalf("want success/0 got %s/%d", end.Status, end.ExitCode)
	}
	if out.String() != "hello" {
		t.Fatalf("stdout %q", out.String())
	}
}

func TestExecFailureExitCode(t *testing.T) {
	e := New()
	sink, frames, mu := collect()
	e.Run("2", "exit 3", 5000, sink)
	mu.Lock()
	defer mu.Unlock()
	last := (*frames)[len(*frames)-1]
	if last.T != "exec_end" || last.Status != "failed" || last.ExitCode != 3 {
		t.Fatalf("want failed/3 got %+v", last)
	}
}

func TestExecTimeout(t *testing.T) {
	e := New()
	sink, frames, mu := collect()
	start := time.Now()
	e.Run("3", "sleep 5", 200, sink)
	elapsed := time.Since(start)
	mu.Lock()
	defer mu.Unlock()
	last := (*frames)[len(*frames)-1]
	if last.Status != "timeout" {
		t.Fatalf("want timeout got %s", last.Status)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout did not bound execution: %s", elapsed)
	}
}

func TestExecCancel(t *testing.T) {
	e := New()
	sink, frames, mu := collect()
	go func() {
		time.Sleep(150 * time.Millisecond)
		e.Cancel("4")
	}()
	e.Run("4", "sleep 5", 5000, sink)
	mu.Lock()
	defer mu.Unlock()
	last := (*frames)[len(*frames)-1]
	if last.Status != "cancelled" {
		t.Fatalf("want cancelled got %s", last.Status)
	}
}
