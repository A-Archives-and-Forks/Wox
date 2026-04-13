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
