package localfs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"main/internal/domain/device"
)

func TestCachedRevocationList_Revoke(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := NewRevocationRepository(tempDir)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	ctx := context.Background()
	rl, err := NewCachedRevocationList(ctx, repo)
	if err != nil {
		t.Fatalf("failed to create CachedRevocationList: %v", err)
	}

	tokenID := "test-token-123"
	expiry := time.Now().Add(1 * time.Hour)

	// 1. 第一次 Prepare 应该成功，且 Commit 应该成功
	commitFn, err := rl.PrepareRevoke(ctx, tokenID, expiry)
	if err != nil {
		t.Fatalf("expected nil error on first prepare, got %v", err)
	}
	if err := commitFn(); err != nil {
		t.Errorf("expected nil error on first commit, got %v", err)
	}

	// 2. 已经被吊销后的第二次 Prepare 应该直接返回 ErrTokenAlreadyRevoked
	_, err = rl.PrepareRevoke(ctx, tokenID, expiry)
	if !errors.Is(err, device.ErrTokenAlreadyRevoked) {
		t.Errorf("expected ErrTokenAlreadyRevoked on second prepare, got %v", err)
	}

	// 3. 测试重启加载（使用新的 CachedRevocationList 实例读取同一个目录）
	rl2, err := NewCachedRevocationList(ctx, repo)
	if err != nil {
		t.Fatalf("failed to reload CachedRevocationList: %v", err)
	}

	_, err = rl2.PrepareRevoke(ctx, tokenID, expiry)
	if !errors.Is(err, device.ErrTokenAlreadyRevoked) {
		t.Errorf("expected ErrTokenAlreadyRevoked after reload, got %v", err)
	}
}

func TestCachedRevocationList_Revoke_Concurrent(t *testing.T) {
	tempDir := t.TempDir()
	repo, err := NewRevocationRepository(tempDir)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	ctx := context.Background()
	rl, err := NewCachedRevocationList(ctx, repo)
	if err != nil {
		t.Fatalf("failed to create CachedRevocationList: %v", err)
	}

	tokenID := "concurrent-token-123"
	expiry := time.Now().Add(1 * time.Hour)

	concurrency := 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var prepares []device.RevokeFunc
	var prepErrs []error
	var mu sync.Mutex

	// 并发 Prepare
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			commitFn, err := rl.PrepareRevoke(ctx, tokenID, expiry)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				prepares = append(prepares, commitFn)
			} else {
				prepErrs = append(prepErrs, err)
			}
		}()
	}
	wg.Wait()

	// 并发 Prepare 应该全都成功，因为此时还没有人 Commit 正式吊销
	if len(prepares) != concurrency {
		t.Errorf("expected all %d prepares to succeed, got %d", concurrency, len(prepares))
	}
	if len(prepErrs) != 0 {
		t.Errorf("expected 0 prepare errors, got %d", len(prepErrs))
	}

	// 并发 Commit
	wg.Add(concurrency)
	successCount := 0
	revokedCount := 0
	otherErrCount := 0

	for _, commitFn := range prepares {
		fn := commitFn
		go func() {
			defer wg.Done()
			err := fn()
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, device.ErrTokenAlreadyRevoked) {
				revokedCount++
			} else {
				otherErrCount++
			}
		}()
	}
	wg.Wait()

	// 最终应该只有一个 Commit 成功，其余应该因为乐观锁返回 ErrTokenAlreadyRevoked
	if successCount != 1 {
		t.Errorf("expected exactly 1 successful commit, got %d", successCount)
	}
	if revokedCount != concurrency-1 {
		t.Errorf("expected %d already-revoked errors on commit, got %d", concurrency-1, revokedCount)
	}
	if otherErrCount != 0 {
		t.Errorf("expected 0 other commit errors, got %d", otherErrCount)
	}
}
