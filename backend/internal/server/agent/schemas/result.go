package schemas

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"regexp"
	"strings"
	"sync"
)

type resultWithStage struct {
	stage  int
	flowID string
	out    *ExecutionResult
}

type resultStore struct {
	mu         sync.RWMutex
	byTask     map[string]resultWithStage // taskID -> result
	lastByFlow map[string]resultWithStage // flowID -> last result
}

func NewResultStore() *resultStore {
	return &resultStore{
		byTask:     map[string]resultWithStage{},
		lastByFlow: map[string]resultWithStage{},
	}
}

func (s *resultStore) Put(taskID, flowID string, stage int, out *ExecutionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := resultWithStage{stage: stage, flowID: flowID, out: out}
	s.byTask[taskID] = r
	s.lastByFlow[flowID] = r
}

func (s *resultStore) Get(taskID string) *ExecutionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.byTask[taskID]; ok {
		return v.out
	}
	return nil
}

func (s *resultStore) GetByFlow(flowID string) *ExecutionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.lastByFlow[flowID]; ok {
		return v.out
	}
	return nil
}

func (s *resultStore) GetMany(taskIDs []string) map[string]*ExecutionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(taskIDs) == 0 {
		return nil
	}
	ret := make(map[string]*ExecutionResult, len(taskIDs))
	for _, id := range taskIDs {
		if v, ok := s.byTask[id]; ok {
			ret[id] = v.out
		}
	}
	return ret
}

/*************************
 * ParamRefs 解析
 *************************/

// 形如：{{task.<idOrFlow>.(output|metadata).a.b.c}}
var refRe = regexp.MustCompile(`^\{\{\s*task\.([a-zA-Z0-9_\-]+)\.(output|metadata)\.([a-zA-Z0-9_\-\.]+)\s*\}\}$`)

func ResolveParamRef(ref string, store *resultStore, cur schemas.PlanTask) (any, bool, error) {
	ref = strings.TrimSpace(ref)
	m := refRe.FindStringSubmatch(ref)
	if len(m) != 4 {
		return nil, false, fmt.Errorf("bad ref format: %s", ref)
	}
	idOrFlow := m[1]
	section := m[2] // "output" | "metadata"
	path := m[3]    // "a.b.c"

	// 兼容：既支持按 taskID 引用，也支持按 flowID 引用“最近一次”
	var res *ExecutionResult
	if r := store.Get(idOrFlow); r != nil {
		res = r
	} else if r := store.GetByFlow(idOrFlow); r != nil {
		res = r
	}
	if res == nil {
		return nil, false, nil
	}
	// 取值
	switch section {
	case "output":
		return dig(map[string]any(res.Data), path)
	case "metadata":
		return dig(map[string]any(res.Metadata), path)
	default:
		return nil, false, fmt.Errorf("unknown section: %s", section)
	}
}

func dig(m map[string]any, path string) (any, bool, error) {
	if m == nil {
		return nil, false, nil
	}
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		asMap, ok := cur.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("non-object at %s", p)
		}
		nxt, ok := asMap[p]
		if !ok {
			return nil, false, nil
		}
		cur = nxt
	}
	return cur, true, nil
}
