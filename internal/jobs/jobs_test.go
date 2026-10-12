package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
)

func waitDone(t *testing.T, m *Manager, id string) model.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	j, err := m.WaitJob(ctx, id)
	if err != nil {
		t.Fatalf("wait %s: %v", id, err)
	}
	return j
}

func TestJobLifecycle(t *testing.T) {
	m := NewManager(context.Background(), nil)
	j := m.Start("p", "k", "ok", func(ctx context.Context, p Progress) (*model.JobResult, error) {
		p(0.5, "half")
		return &model.JobResult{Type: "run", ID: "r1"}, nil
	})
	if j.Status != StatusRunning || !j.Cancellable {
		t.Fatalf("new job: %+v", j)
	}
	done := waitDone(t, m, j.ID)
	if done.Status != StatusSucceeded || done.Progress != 1 || done.Result == nil || done.Result.ID != "r1" || done.Cancellable || done.FinishedAt == "" {
		t.Fatalf("finished job: %+v", done)
	}
}

func TestJobFailureAndPanic(t *testing.T) {
	m := NewManager(context.Background(), nil)
	failed := m.Start("p", "k", "fail", func(context.Context, Progress) (*model.JobResult, error) {
		return nil, errors.New("boom")
	})
	panicked := m.Start("p", "k", "panic", func(context.Context, Progress) (*model.JobResult, error) {
		panic("broken invariant")
	})
	if j := waitDone(t, m, failed.ID); j.Status != StatusFailed || j.Error != "boom" {
		t.Fatalf("failed job: %+v", j)
	}
	if j := waitDone(t, m, panicked.ID); j.Status != StatusFailed || j.Error != "internal error: broken invariant" {
		t.Fatalf("panicked job: %+v", j)
	}
}

func TestCancel(t *testing.T) {
	var ended model.Job
	endCh := make(chan struct{})
	m := NewManager(context.Background(), func(j model.Job) { ended = j; close(endCh) })
	started := make(chan struct{})
	j := m.Start("p", "k", "long", func(ctx context.Context, p Progress) (*model.JobResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	<-started
	got, err := m.Cancel(j.ID)
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("cancel: %+v %v", got, err)
	}
	<-endCh
	if ended.ID != j.ID || ended.Status != StatusCancelled {
		t.Fatalf("onEnd: %+v", ended)
	}
	if _, err := m.Cancel(j.ID); !errors.Is(err, ErrNotCancellable) {
		t.Fatalf("second cancel: %v", err)
	}
	if _, err := m.Cancel("job-unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestListNewestFirstAndRetention(t *testing.T) {
	m := NewManager(context.Background(), nil)
	var last string
	for i := 0; i < keep+10; i++ {
		j := m.Start("p", "k", "j", func(context.Context, Progress) (*model.JobResult, error) { return nil, nil })
		waitDone(t, m, j.ID)
		last = j.ID
	}
	m.Wait()
	list := m.List(0)
	if len(list) > keep+1 {
		t.Fatalf("retention: %d jobs kept", len(list))
	}
	if list[0].ID != last {
		t.Fatalf("newest first: got %s want %s", list[0].ID, last)
	}
	if got := m.List(3); len(got) != 3 {
		t.Fatalf("limit: %d", len(got))
	}
}
