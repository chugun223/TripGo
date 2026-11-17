package positions

import (
	"bytes"
	"container/heap"
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/chugun223/TripGo/internal/model"
)

type priorityQueue []*task

func (q priorityQueue) Len() int           { return len(q) }
func (q priorityQueue) Less(i, j int) bool { return older(q[i], q[j]) }
func (q priorityQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index, q[j].index = i, j
}

func (q *priorityQueue) Push(value any) {
	queued := value.(*task)
	queued.index = len(*q)
	*q = append(*q, queued)
}

func (q *priorityQueue) Pop() any {
	last := len(*q) - 1
	queued := (*q)[last]
	(*q)[last] = nil
	*q = (*q)[:last]
	queued.index = -1
	return queued
}

func older(left, right *task) bool {
	if !left.trip.LastActivityAt.Equal(right.trip.LastActivityAt) {
		return left.trip.LastActivityAt.Before(right.trip.LastActivityAt)
	}
	return bytes.Compare(left.trip.ID[:], right.trip.ID[:]) < 0
}

func (p *Pool) enqueue(trip model.StaleTrip, link trace.SpanContext) (string, *task) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return resultDropped, nil
	}
	if _, ok := p.pending[trip.ID]; ok {
		return resultDuplicate, nil
	}
	queued := &task{trip: trip, link: link, index: -1}
	var evicted *task
	if p.queue.Len() == p.settings.QueueSize {
		freshest := 0
		for i := 1; i < p.queue.Len(); i++ {
			if older(p.queue[freshest], p.queue[i]) {
				freshest = i
			}
		}
		if !older(queued, p.queue[freshest]) {
			return resultDropped, nil
		}
		evicted = heap.Remove(&p.queue, freshest).(*task)
		delete(p.pending, evicted.trip.ID)
	}
	heap.Push(&p.queue, queued)
	p.pending[trip.ID] = queued
	p.notify()
	return resultQueued, evicted
}

func (p *Pool) take(ctx context.Context) (*task, bool) {
	for {
		p.mu.Lock()
		if ctx.Err() != nil {
			p.mu.Unlock()
			return nil, false
		}
		if p.queue.Len() > 0 {
			queued := heap.Pop(&p.queue).(*task)
			p.mu.Unlock()
			return queued, true
		}
		if p.closed {
			p.mu.Unlock()
			return nil, false
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, false
		case <-changed:
		}
	}
}

func (p *Pool) closeQueue() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.notify()
}

func (p *Pool) notify() {
	close(p.changed)
	p.changed = make(chan struct{})
}
