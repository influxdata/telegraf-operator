package main

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-logr/logr"
)

type telegrafClassesOnChange func()

// telegrafClassesWatcher allows monitoring a directory with telegraf classes using
// fsnotify package and batching multiple events to reduce number of Kubernetes API calls.
type telegrafClassesWatcher struct {
	logger        logr.Logger
	watcherEvents chan fsnotify.Event
	onChange      telegrafClassesOnChange

	eventCount   uint64
	eventChannel chan struct{}
	eventDelay   time.Duration
}

// newTelegrafClassesWatcher creates a new instance of telegrafClassesWatcher.
func newTelegrafClassesWatcher(logger logr.Logger, telegrafClassesDirectory string, onChange telegrafClassesOnChange) (*telegrafClassesWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// watching the contents of classes directory requires adding the directory as well as most child elements
	logger.Info("adding directory to watcher", "directory", telegrafClassesDirectory)
	watcher.Add(telegrafClassesDirectory)

	items, err := ioutil.ReadDir(telegrafClassesDirectory)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		name := item.Name()

		// Add all items in the classes directory except for current/previous secret contents that begin with "..", "." and ".."
		// explicitly add "..data" directory as this is the directory that maps current state of the secret.
		//
		// Example listing of classes directory:
		//
		// drwxrwxrwt 3 root root  100 Jul 29 12:27 .
		// drwxr-xr-x 1 root root 4096 Jul 29 12:26 ..
		// drwxr-xr-x 2 root root   60 Jul 29 12:27 ..2021_07_29_12_27_39.113045998
		// lrwxrwxrwx 1 root root   31 Jul 29 12:27 ..data -> ..2021_07_29_12_27_39.113045998
		// lrwxrwxrwx 1 root root   20 Jul 29 12:26 app -> ..data/app
		// lrwxrwxrwx 1 root root   20 Jul 29 12:26 basic -> ..data/basic
		//
		// in the above case, we want to match "..data", "app" and "basic", but skip ".", ".." and "..2021_07_29_12_27_39.113045998"
		if name == "..data" || (name != "." && !strings.HasPrefix(name, "..")) {
			p := filepath.Join(telegrafClassesDirectory, name)
			logger.Info("adding item to watch", "path", p)
			err = watcher.Add(p)
			if err != nil {
				return nil, err
			}
		}
	}

	w := &telegrafClassesWatcher{
		watcherEvents: watcher.Events,
		logger:        logger,
		onChange:      onChange,

		// allow large number of messages in the channel to avoid blocking
		eventChannel: make(chan struct{}, 100),

		// delay by 10 seconds to group multiple fsnotify events into single invocation of callback
		eventDelay: 10 * time.Second,
	}

	w.createGoroutines()

	return w, nil
}

// createGoroutines runs all required goroutines for the watcher.
func (w *telegrafClassesWatcher) createGoroutines() {
	go w.batchChanges()
	go w.monitorForChanges()
}

// batchChanges is a goroutine that batches invocations of onChange()
// based on events sent from monitorForChanges().
func (w *telegrafClassesWatcher) batchChanges() {
	var previousEventCount uint64
	for range w.eventChannel {

		currentEventCount := atomic.LoadUint64(&w.eventCount)

		// check if counter is same as last time events were processed,
		// only delay and batch if it is different
		if currentEventCount != previousEventCount {
			// delay processing of the event to batch multiple events from file
			time.Sleep(w.eventDelay)

			// update  the event counter again to latest, potentially different value
			currentEventCount = atomic.LoadUint64(&w.eventCount)

			w.onChange()

			previousEventCount = currentEventCount
		}
	}
}

// monitorForChanges helps batch events from fsnotify by incrementing a counter and
// sending events using an internal channel, then handled by batchChanges().
func (w *telegrafClassesWatcher) monitorForChanges() {
	for {
		_, ok := <-w.watcherEvents
		if ok {
			// increase the event counter and send a message to goroutine
			// that batches invocations of onChange()
			atomic.AddUint64(&w.eventCount, 1)
			w.eventChannel <- struct{}{}
		}
	}
}
