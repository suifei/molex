package client

import "sync"

const maxConcurrentStreams = 256

type streamGroup struct {
	slots chan struct{}
	wait  sync.WaitGroup
}

func newStreamGroup(limit int) *streamGroup {
	return &streamGroup{slots: make(chan struct{}, limit)}
}

func (g *streamGroup) goIfAvailable(run func()) bool {
	select {
	case g.slots <- struct{}{}:
	default:
		return false
	}
	g.wait.Add(1)
	go func() {
		defer g.wait.Done()
		defer func() { <-g.slots }()
		run()
	}()
	return true
}

func (g *streamGroup) waitForAll() {
	g.wait.Wait()
}
