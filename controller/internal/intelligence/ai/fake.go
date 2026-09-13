package ai

import (
	"context"
	"sync"
	"time"
)

type FakeProvider struct {
	mu       sync.Mutex
	Response Response
	Err      error
	Delay    time.Duration
	Requests []Request
}

func (f *FakeProvider) Analyze(ctx context.Context, request Request) (Response, error) {
	if f.Delay > 0 {
		timer := time.NewTimer(f.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-timer.C:
		}
	}
	f.mu.Lock()
	f.Requests = append(f.Requests, cloneRequest(request))
	response, err := f.Response, f.Err
	f.mu.Unlock()
	return response, err
}

func (f *FakeProvider) SnapshotRequests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Request, len(f.Requests))
	for i, request := range f.Requests {
		out[i] = cloneRequest(request)
	}
	return out
}
