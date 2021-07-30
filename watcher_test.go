package main

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	logrTesting "github.com/go-logr/logr/testing"
)

type mockOnChange struct {
	count int64
}

func (m *mockOnChange) onChange() {
	atomic.AddInt64(&m.count, 1)
}

func (m *mockOnChange) get() int {
	return int(atomic.LoadInt64(&m.count))
}

func testWatcher(t *testing.T, onChange telegrafClassesOnChange) *telegrafClassesWatcher {
	logger := &logrTesting.TestLogger{T: t}

	w := &telegrafClassesWatcher{
		watcherEvents: make(chan fsnotify.Event, 100),
		logger:        logger,
		onChange:      onChange,
		eventChannel:  make(chan struct{}, 100),
		eventDelay:    50 * time.Millisecond,
	}

	w.createGoroutines()

	return w
}

func sendTestWatcherEvent(w *telegrafClassesWatcher) {
	w.watcherEvents <- fsnotify.Event{Name: "dummy", Op: fsnotify.Write}
}

func Test_Watcher_SingleEvent(t *testing.T) {
	mock := &mockOnChange{}
	watcher := testWatcher(t, mock.onChange)
	sendTestWatcherEvent(watcher)
	time.Sleep(watcher.eventDelay * 2)

	if want, got := 1, mock.get(); want != got {
		t.Errorf("want %v, got %v", want, got)
	}
}

func Test_Watcher_MultipleEvents(t *testing.T) {
	mock := &mockOnChange{}
	watcher := testWatcher(t, mock.onChange)
	sendTestWatcherEvent(watcher)
	sendTestWatcherEvent(watcher)
	sendTestWatcherEvent(watcher)
	time.Sleep(watcher.eventDelay * 2)

	if want, got := 1, mock.get(); want != got {
		t.Errorf("want %v, got %v", want, got)
	}
}

func Test_Watcher_EventsOverTime(t *testing.T) {
	mock := &mockOnChange{}
	watcher := testWatcher(t, mock.onChange)
	sendTestWatcherEvent(watcher)
	time.Sleep(watcher.eventDelay * 2)
	sendTestWatcherEvent(watcher)
	sendTestWatcherEvent(watcher)
	time.Sleep(watcher.eventDelay * 2)
	sendTestWatcherEvent(watcher)
	sendTestWatcherEvent(watcher)
	sendTestWatcherEvent(watcher)
	time.Sleep(watcher.eventDelay * 2)

	if want, got := 3, mock.get(); want != got {
		t.Errorf("want %v, got %v", want, got)
	}
}
