package downloader

import (
	"context"
	"sync"
)

type Pool struct {
	wg sync.WaitGroup
}

type Task func()

func NewPool(ctx context.Context) *Pool {
	return &Pool{}
}

func (p *Pool) newWorker(task Task) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		task()
	}()
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

func (p *Pool) Add(task Task) {
	p.newWorker(task)
}
