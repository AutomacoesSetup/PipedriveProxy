package upstream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"pipedrive_api_service/internal/utils"
)

type Task struct {
	Method      string
	URL         string
	Headers     map[string]string
	Body        []byte
	Attempts    int
	MaxAttempts int
	respCh      chan taskResult
	createdAt   time.Time
}

type taskResult struct {
	resp *http.Response
	body []byte
	rate *utils.RateLimitInfo
	err  error
}

type UpstreamBroker struct {
	client     *http.Client
	queue      chan *Task
	workers    int
	quit       chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	pauseUntil time.Time
	lastRate   *utils.RateLimitInfo
}

// NewUpstreamBroker creates a broker with worker pool and bounded queue.
func NewUpstreamBroker(workers int, queueSize int) *UpstreamBroker {
	if workers <= 0 {
		workers = 4
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	b := &UpstreamBroker{
		client:  &http.Client{Timeout: 40 * time.Second},
		queue:   make(chan *Task, queueSize),
		workers: workers,
		quit:    make(chan struct{}),
	}
	b.start()
	return b
}

// Stop gracefully stops workers.
func (b *UpstreamBroker) Stop() {
	close(b.quit)
	b.wg.Wait()
}

// Execute enqueues a task and waits for result or ctx cancellation.
func (b *UpstreamBroker) Execute(ctx context.Context, method, url string, headers map[string]string, body []byte, maxAttempts int) (*http.Response, []byte, *utils.RateLimitInfo, error) {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	t := &Task{
		Method:      method,
		URL:         url,
		Headers:     headers,
		Body:        body,
		Attempts:    0,
		MaxAttempts: maxAttempts,
		respCh:      make(chan taskResult, 1),
		createdAt:   time.Now(),
	}

	// Try to enqueue but respect caller context.
	select {
	case b.queue <- t:
	case <-ctx.Done():
		return nil, nil, nil, ctx.Err()
	}

	select {
	case <-ctx.Done():
		return nil, nil, nil, ctx.Err()
	case res := <-t.respCh:
		return res.resp, res.body, res.rate, res.err
	}
}

func (b *UpstreamBroker) start() {
	for i := 0; i < b.workers; i++ {
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.workerLoop()
		}()
	}
}

func (b *UpstreamBroker) workerLoop() {
	for {
		select {
		case <-b.quit:
			return
		case task := <-b.queue:
			if task == nil {
				continue
			}
			if !b.waitIfNotPaused() {
				// quit signaled
				task.respCh <- taskResult{nil, nil, nil, errors.New("broker shutting down")}
				continue
			}
			b.processTask(task)
		}
	}
}

// waitIfNotPaused returns true if ok to proceed, false on quit.
func (b *UpstreamBroker) waitIfNotPaused() bool {
	for {
		b.mu.Lock()
		until := b.pauseUntil
		b.mu.Unlock()

		now := time.Now()
		if now.Before(until) {
			// wait until pause ends or quit
			select {
			case <-b.quit:
				return false
			case <-time.After(time.Until(until)):
				continue
			}
		}
		return true
	}
}

func (b *UpstreamBroker) processTask(t *Task) {
	// build request
	var bodyReader io.Reader
	if len(t.Body) > 0 {
		bodyReader = bytes.NewReader(t.Body)
	}
	req, err := http.NewRequest(t.Method, t.URL, bodyReader)
	if err != nil {
		t.respCh <- taskResult{nil, nil, nil, err}
		return
	}
	for k, v := range t.Headers {
		req.Header.Set(k, v)
	}

	// execute request
	resp, err := b.client.Do(req)
	if err != nil {
		// network error -> retry with backoff if allowed
		if t.Attempts < t.MaxAttempts {
			t.Attempts++
			go b.requeueWithBackoff(t, t.Attempts)
			return
		}
		t.respCh <- taskResult{nil, nil, nil, err}
		return
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// extract rate
	rate := utils.ExtractRateLimitFromHeaders(resp.Header)
	if rate != nil {
		b.mu.Lock()
		b.lastRate = rate
		b.mu.Unlock()
		if rate.Remaining <= 0 && rate.ResetAt > 0 {
			resetAt := time.Unix(rate.ResetAt, 0)
			if resetAt.After(time.Now()) {
				b.setPause(resetAt)
			}
		}
	}

	// Handle 429
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := computeRetryAfter(resp.Header, t.Attempts)
		b.setPause(time.Now().Add(wait))
		if t.Attempts < t.MaxAttempts {
			t.Attempts++
			go func(tt *Task, w time.Duration) {
				time.Sleep(w)
				select {
				case b.queue <- tt:
				default:
					tt.respCh <- taskResult{nil, nil, rate, errors.New("queue full while re-enqueue")}
				}
			}(t, wait)
			return
		}
		t.respCh <- taskResult{nil, bodyBytes, rate, errors.New("max attempts reached after 429")}
		return
	}

	// For server errors (5xx) we may retry
	if resp.StatusCode >= 500 && t.Attempts < t.MaxAttempts {
		t.Attempts++
		go b.requeueWithBackoff(t, t.Attempts)
		return
	}

	// success or client error, return result
	respCopy := &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Header:        resp.Header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(bodyBytes)),
		ContentLength: int64(len(bodyBytes)),
		Request:       resp.Request,
	}

	t.respCh <- taskResult{respCopy, bodyBytes, rate, nil}
}

func (b *UpstreamBroker) setPause(t time.Time) {
	b.mu.Lock()
	if t.After(b.pauseUntil) {
		b.pauseUntil = t
	}
	b.mu.Unlock()
}

func (b *UpstreamBroker) requeueWithBackoff(t *Task, attempt int) {
	delay := time.Second * time.Duration(1<<attempt)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	time.Sleep(delay)
	select {
	case b.queue <- t:
	default:
		t.respCh <- taskResult{nil, nil, nil, errors.New("queue full while retry")}
	}
}

func computeRetryAfter(h http.Header, attempt int) time.Duration {
	if v := h.Get(utils.HeaderRetryAfter); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			return time.Until(t)
		}
	}
	if v := h.Get(utils.HeaderXRateLimitReset); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			if n < 1_000_000_000 {
				return time.Duration(n) * time.Second
			}
			resetAt := time.Unix(n, 0)
			return time.Until(resetAt)
		}
	}
	base := time.Second
	wait := base * time.Duration(1<<attempt)
	if wait > 30*time.Second {
		wait = 30 * time.Second
	}
	return wait
}

// --- global singleton for broker ---

var (
	globalMu sync.Mutex
	global   *UpstreamBroker
)

// SetGlobalBroker registers a broker as global singleton.
func SetGlobalBroker(b *UpstreamBroker) {
	globalMu.Lock()
	global = b
	globalMu.Unlock()
}

// GlobalBroker returns the global broker instance.
func GlobalBroker() *UpstreamBroker {
	globalMu.Lock()
	defer globalMu.Unlock()
	return global
}
