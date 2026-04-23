package filesearch

import (
	"context"
	"fmt"
	"os"
	"sync"
	"wox/util"
)

type Policy struct {
	ShouldIndexPath     func(root RootRecord, path string, isDir bool) bool
	ShouldProcessChange func(root RootRecord, change ChangeSignal) bool
}

type EngineOptions struct {
	Policy Policy
}

const runPlannerSplitPolicyVersionV1 = 1

// splitBudget keeps the version-1 run planner thresholds in code.
// The previous root-centric planner had no internal job sizing, so one huge
// root could stay as one opaque workload. Version 1 fixes that with constants
// first so progress semantics stay predictable before we consider any user
// settings or adaptive tuning.
type splitBudget struct {
	LeafEntryBudget     int64
	LeafWriteBudget     int64
	LeafMemoryBudget    int64
	// Version 1 keeps one direct-files job per directory so delete ownership
	// stays simple. The older planner split one directory into many jobs, which
	// made stale direct-file pruning ambiguous. The same limit now only caps the
	// internal staging batch size inside that single job.
	DirectFileBatchSize int
}

func defaultSplitBudget() splitBudget {
	return splitBudget{
		LeafEntryBudget:     4096,
		LeafWriteBudget:     4096,
		LeafMemoryBudget:    8 << 20,
		DirectFileBatchSize: 2048,
	}
}

func DefaultEngineOptions() EngineOptions {
	return EngineOptions{}
}

type policyState struct {
	mu     sync.RWMutex
	policy Policy
}

func newPolicyState(policy Policy) *policyState {
	return &policyState{policy: policy}
}

func (s *policyState) Set(policy Policy) {
	if s == nil {
		return
	}

	s.mu.Lock()
	s.policy = policy
	s.mu.Unlock()
}

func (s *policyState) shouldIndexPath(root RootRecord, path string, isDir bool) bool {
	if s == nil {
		return true
	}

	s.mu.RLock()
	callback := s.policy.ShouldIndexPath
	s.mu.RUnlock()
	if callback == nil {
		return true
	}

	return runPolicyCallback("ShouldIndexPath", func() bool {
		return callback(root, path, isDir)
	})
}

func (s *policyState) shouldProcessChange(root RootRecord, change ChangeSignal) bool {
	if s == nil {
		return true
	}

	s.mu.RLock()
	callback := s.policy.ShouldProcessChange
	s.mu.RUnlock()
	if callback == nil {
		return true
	}

	return runPolicyCallback("ShouldProcessChange", func() bool {
		return callback(root, change)
	})
}

func runPolicyCallback(name string, callback func() bool) (allowed bool) {
	allowed = true
	defer func() {
		if recovered := recover(); recovered != nil {
			util.GetLogger().Warn(context.Background(), fmt.Sprintf("filesearch policy %s panic recovered: %v", name, recovered))
			allowed = true
		}
	}()

	return callback()
}

func statPathType(path string) (bool, bool) {
	if path == "" {
		return false, false
	}

	if info, err := os.Stat(path); err == nil {
		return info.IsDir(), true
	}
	if info, err := os.Lstat(path); err == nil {
		return info.IsDir(), true
	}

	return false, false
}
