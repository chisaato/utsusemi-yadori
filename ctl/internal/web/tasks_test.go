package web

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTaskLifecycle(t *testing.T) {
	tm := NewTaskManager()
	var wg sync.WaitGroup
	wg.Add(1)
	tm.Start("t1", func(upd func(phase, detail string) error) error {
		defer wg.Done()
		if err := upd("download", "official 17.2.14"); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	wg.Wait()
	time.Sleep(20 * time.Millisecond) // 等 goroutine 收尾写终态
	task, ok := tm.Get("t1")
	if !ok || task.State != "done" {
		t.Fatalf("want done, got %+v ok=%v", task, ok)
	}
	if task.Phase != "download" {
		t.Fatalf("want phase download preserved, got %q", task.Phase)
	}
}

func TestTaskError(t *testing.T) {
	tm := NewTaskManager()
	var wg sync.WaitGroup
	wg.Add(1)
	tm.Start("t2", func(upd func(phase, detail string) error) error {
		defer wg.Done()
		return errors.New("boom")
	})
	wg.Wait()
	time.Sleep(20 * time.Millisecond)
	task, _ := tm.Get("t2")
	if task.State != "error" || task.Error != "boom" {
		t.Fatalf("want error/boom, got %+v", task)
	}
}

func TestTaskProgressVisibleWhileRunning(t *testing.T) {
	tm := NewTaskManager()
	block := make(chan struct{})
	started := make(chan struct{})
	tm.Start("t3", func(upd func(phase, detail string) error) error {
		close(started)
		_ = upd("decompress", "xz")
		<-block
		return nil
	})
	<-started
	task, _ := tm.Get("t3")
	if task.State != "running" || task.Phase != "decompress" || task.Detail != "xz" {
		t.Fatalf("want running/decompress/xz, got %+v", task)
	}
	close(block)
	time.Sleep(20 * time.Millisecond)
	task, _ = tm.Get("t3")
	if task.State != "done" {
		t.Fatalf("want done, got %+v", task)
	}
}

func TestGetMissing(t *testing.T) {
	tm := NewTaskManager()
	if _, ok := tm.Get("nope"); ok {
		t.Fatal("want miss for unknown id")
	}
}
