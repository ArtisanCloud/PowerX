package testenv

import (
	"context"
	"errors"
	"sync"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/google/uuid"
)

// SparseIndexStub 提供简单的内存实现，便于测试 BM25/FTS 查询与降级逻辑。
type SparseIndexStub struct {
	mu             sync.Mutex
	queryResponses map[uuid.UUID]ksvc.SparseQueryResponse
	lastQuery      ksvc.SparseQueryRequest
	queryFailures  int
	healthErr      error
}

func NewSparseIndexStub() *SparseIndexStub {
	return &SparseIndexStub{
		queryResponses: make(map[uuid.UUID]ksvc.SparseQueryResponse),
	}
}

func (s *SparseIndexStub) Query(_ context.Context, req ksvc.SparseQueryRequest) (ksvc.SparseQueryResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastQuery = req
	if s.queryFailures > 0 {
		s.queryFailures--
		return ksvc.SparseQueryResponse{}, errors.New("sparse index stub: forced query failure")
	}
	resp, ok := s.queryResponses[req.SpaceID]
	if !ok {
		return ksvc.SparseQueryResponse{}, nil
	}
	return resp, nil
}

func (s *SparseIndexStub) Health(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthErr
}

func (s *SparseIndexStub) SetQueryResponse(space uuid.UUID, resp ksvc.SparseQueryResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryResponses[space] = resp
}

func (s *SparseIndexStub) LastQuery() ksvc.SparseQueryRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastQuery
}

func (s *SparseIndexStub) SetQueryFailures(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < 0 {
		n = 0
	}
	s.queryFailures = n
}

func (s *SparseIndexStub) SetHealthError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthErr = err
}
