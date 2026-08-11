package client

import (
	"testing"
	"time"
)

func TestStreamGroupBoundsAndDrainsConcurrentWork(t *testing.T) {
	group := newStreamGroup(1)
	release := make(chan struct{})
	started := make(chan struct{})
	if !group.goIfAvailable(func() {
		close(started)
		<-release
	}) {
		t.Fatal("first stream was rejected")
	}
	<-started
	if group.goIfAvailable(func() {}) {
		t.Fatal("stream group exceeded its configured limit")
	}
	close(release)
	done := make(chan struct{})
	go func() {
		group.waitForAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream group did not drain")
	}
}
