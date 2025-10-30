package client

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RequestTask struct {
	Method      string
	URL         string
	Headers     map[string]string
	Body        []byte
	Attempts    int
	MaxAttempts int
	Callback    func(*http.Response, []byte, error)
}

type RetryQueue struct {
	ch     chan *RequestTask
	client *http.Client
	wg     sync.WaitGroup
	quit   chan struct{}
}

func NewRetryQueue(size int, client *http.Client) *RetryQueue {
	q := &RetryQueue{
		ch:     make(chan *RequestTask, size),
		client: client,
		quit:   make(chan struct{}),
	}
	q.start()
	return q
}

func (q *RetryQueue) start() {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		for {
			select {
			case <-q.quit:
				return
			case task := <-q.ch:
				q.process(task)
			}
		}
	}()
}

func (q *RetryQueue) Stop() {
	close(q.quit)
	q.wg.Wait()
}

func (q *RetryQueue) Enqueue(task *RequestTask) {
	if task.MaxAttempts <= 0 {
		task.MaxAttempts = 3
	}
	q.ch <- task
}

func (q *RetryQueue) process(task *RequestTask) {
	req, _ := http.NewRequest(task.Method, task.URL, nil)
	if len(task.Body) > 0 {
		req.Body = io.NopCloser(strings.NewReader(string(task.Body)))
	}
	for k, v := range task.Headers {
		req.Header.Set(k, v)
	}

	resp, err := q.client.Do(req)
	var body []byte
	if err == nil && resp != nil && resp.Body != nil {
		defer resp.Body.Close()
		body, _ = io.ReadAll(resp.Body)
	}

	// Retry on 429 with backoff
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests && task.Attempts < task.MaxAttempts {
		wait := computeRetryAfter(resp.Header, task.Attempts)
		time.Sleep(wait)
		task.Attempts++
		q.Enqueue(task)
		return
	}

	if task.Callback != nil {
		task.Callback(resp, body, err)
	}
}

func computeRetryAfter(h http.Header, attempt int) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		// Retry-After: seconds or HTTP-date
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			return time.Until(t)
		}
	}
	// Try X-RateLimit-Reset as unix seconds or seconds-from-now
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			// Heuristic: treat small numbers as seconds-from-now, large as unix ts
			if n < 1_000_000_000 {
				return time.Duration(n) * time.Second
			}
			resetAt := time.Unix(n, 0)
			return time.Until(resetAt)
		}
	}
	// Exponential backoff fallback
	base := time.Second
	return base * time.Duration(1<<attempt)
}
