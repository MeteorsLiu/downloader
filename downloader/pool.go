package downloader

import (
	"context"
	"sync"
)

type Pool struct {
	wg     sync.WaitGroup
	sem    chan struct{}
	ctx    context.Context
	doStop context.CancelFunc
}

func NewPool(ctx context.Context, n int) *Pool {
	p := &Pool{
		sem: make(chan struct{}, n),
	}
	p.ctx, p.doStop = context.WithCancel(ctx)
	return p
}

func (p *Pool) IsRunning() bool { return len(p.sem) > 0 }

func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) Stop() {
	p.doStop()
}

func (p *Pool) Add(task func()) {
	p.newWorker(task, true)
}

func (p *Pool) newWorker(task func(), isAddtional ...bool) {
	p.wg.Add(1)
	addtional := false
	if len(isAddtional) > 0 {
		addtional = isAddtional[0]
	}
	go func() {
		defer func() {
			if !addtional {
				<-p.sem
			}
			p.wg.Done()
		}()
		task()
	}()
}

func (p *Pool) Publish(task func()) {
	select {
	case <-p.ctx.Done():
		return
	default:
	}
	select {
	case p.sem <- struct{}{}:
		p.newWorker(task)
	case <-p.ctx.Done():
	}
}
