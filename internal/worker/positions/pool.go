package positions

import (
	"container/heap"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/chugun223/TripGo/internal/infra/telemetry"
	"github.com/chugun223/TripGo/internal/model"
)

type Settings struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	QueryTimeout   time.Duration
	RequestTimeout time.Duration
	Workers        int
	QueueSize      int
	BatchSize      int
}

type task struct {
	trip   model.StaleTrip
	link   trace.SpanContext
	cancel context.CancelFunc
	index  int
}

type Pool struct {
	repository tripRepository
	push       pushGateway
	settings   Settings
	metrics    *telemetry.PositionMetrics
	log        *slog.Logger
	newTicker  func(time.Duration) ticker
	queue      priorityQueue
	changed    chan struct{}
	closed     bool
	pending    map[uuid.UUID]*task
	mu         sync.Mutex
	active     atomic.Int64
	started    bool
	stopScan   context.CancelFunc
	stopWork   context.CancelFunc
	done       chan struct{}
}

func New(repository tripRepository, push pushGateway, settings Settings,
	metrics *telemetry.PositionMetrics, log *slog.Logger,
) (*Pool, error) {
	if settings.CheckInterval <= 0 || settings.StaleAfter <= 0 || settings.QueryTimeout <= 0 ||
		settings.RequestTimeout <= 0 || settings.Workers <= 0 || settings.QueueSize <= 0 || settings.BatchSize <= 0 {
		return nil, errors.New("position pool settings must be positive")
	}
	return &Pool{
		repository: repository, push: push, settings: settings, metrics: metrics, log: log,
		newTicker: func(interval time.Duration) ticker { return realTicker{time.NewTicker(interval)} },
		queue:     make(priorityQueue, 0, settings.QueueSize), changed: make(chan struct{}),
		pending: make(map[uuid.UUID]*task), done: make(chan struct{}),
	}, nil
}

func (p *Pool) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return errors.New("position pool is already started")
	}
	p.started = true
	workCtx, stopWork := context.WithCancel(ctx)
	scanCtx, stopScan := context.WithCancel(workCtx)
	p.stopScan, p.stopWork = stopScan, stopWork
	ticks := p.newTicker(p.settings.CheckInterval)
	scanDone := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < p.settings.Workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			p.work(workCtx)
		}()
	}
	go p.scanLoop(scanCtx, ticks, scanDone)
	go func() {
		<-scanDone
		workers.Wait()
		p.mu.Lock()
		clear(p.queue)
		p.queue = p.queue[:0]
		clear(p.pending)
		p.mu.Unlock()
		stopWork()
		p.log.InfoContext(ctx, "position worker pool stopped")
		close(p.done)
	}()
	p.log.InfoContext(ctx, "position worker pool started", slog.Int("workers", p.settings.Workers),
		slog.Int("queue_capacity", p.settings.QueueSize))
	return nil
}

func (p *Pool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}
	stopScan, stopWork := p.stopScan, p.stopWork
	p.mu.Unlock()
	stopScan()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		stopWork()
		return ctx.Err()
	}
}

func (p *Pool) Cancel(id uuid.UUID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if queued, ok := p.pending[id]; ok {
		delete(p.pending, id)
		if queued.index >= 0 {
			heap.Remove(&p.queue, queued.index)
		}
		if queued.cancel != nil {
			queued.cancel()
		}
	}
}

func (p *Pool) QueueSize() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return int64(p.queue.Len())
}

func (p *Pool) ActiveWorkers() int64 { return p.active.Load() }

type realTicker struct{ *time.Ticker }

func (t realTicker) Ticks() <-chan time.Time { return t.C }
