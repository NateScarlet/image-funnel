package inmem

import (
	"bytes"
	"context"
	"testing"
	"time"

	"main/internal/domain/pairing"
	"main/internal/scalar"
)

// TestPairingRequestRepository_CRUD 测试配对仓储的基本 CRUD 操作
func TestPairingRequestRepository_CRUD(t *testing.T) {
	repo := NewPairingRequestRepository()
	ctx := context.Background()

	code := "123456"
	deviceID := scalar.NewID()
	credID := []byte("cred-123")
	pubKey := []byte("pubkey-456")
	signCount := uint32(5)
	createdAt := time.Now().Truncate(time.Second)

	ip := "127.0.0.1"
	userAgent := "Test User Agent"

	// 1. 初始化读取不存在的验证码应当报错
	_, err := repo.Get(ctx, code)
	if err == nil {
		t.Error("expected error for non-existent pairing code, got nil")
	}

	// 2. 正常保存一个配对请求
	req := pairing.FromRepository(code, deviceID, credID, pubKey, signCount, createdAt, ip, userAgent)
	err = repo.Save(ctx, req)
	if err != nil {
		t.Fatalf("failed to save pairing request: %v", err)
	}

	// 3. 读取保存的配对请求并验证字段
	got, err := repo.Get(ctx, code)
	if err != nil {
		t.Fatalf("failed to get pairing request: %v", err)
	}

	if got.Code() != code {
		t.Errorf("expected code %s, got %s", code, got.Code())
	}
	if got.DeviceID() != deviceID {
		t.Errorf("expected device ID %s, got %s", deviceID, got.DeviceID())
	}
	if !bytes.Equal(got.CredentialID(), credID) {
		t.Errorf("expected credential ID %v, got %v", credID, got.CredentialID())
	}
	if !bytes.Equal(got.PublicKey(), pubKey) {
		t.Errorf("expected public key %v, got %v", pubKey, got.PublicKey())
	}
	if got.SignCount() != signCount {
		t.Errorf("expected sign count %d, got %d", signCount, got.SignCount())
	}
	if !got.CreatedAt().Equal(createdAt) {
		t.Errorf("expected created at %v, got %v", createdAt, got.CreatedAt())
	}
	if got.IP() != ip {
		t.Errorf("expected ip %s, got %s", ip, got.IP())
	}
	if got.UserAgent() != userAgent {
		t.Errorf("expected user agent %s, got %s", userAgent, got.UserAgent())
	}

	// 4. 删除此配对请求
	err = repo.Delete(ctx, code)
	if err != nil {
		t.Fatalf("failed to delete pairing request: %v", err)
	}

	// 5. 再次查询应当报错
	_, err = repo.Get(ctx, code)
	if err == nil {
		t.Error("expected error for deleted pairing code, got nil")
	}
}

// TestPairingRequestRepository_Find 测试 Find 方法的并发安全与正确性
func TestPairingRequestRepository_Find(t *testing.T) {
	repo := NewPairingRequestRepository()
	ctx := context.Background()

	req1 := pairing.FromRepository("111111", scalar.NewID(), []byte("c1"), []byte("p1"), 1, time.Now(), "127.0.0.1", "ua1")
	req2 := pairing.FromRepository("222222", scalar.NewID(), []byte("c2"), []byte("p2"), 2, time.Now(), "127.0.0.1", "ua2")

	_ = repo.Save(ctx, req1)
	_ = repo.Save(ctx, req2)

	foundCodes := make(map[string]bool)
	for req, err := range repo.Find(ctx) {
		if err != nil {
			t.Errorf("unexpected error during find: %v", err)
		}
		foundCodes[req.Code()] = true
	}

	if len(foundCodes) != 2 {
		t.Errorf("expected 2 records, got %d", len(foundCodes))
	}
	if !foundCodes["111111"] || !foundCodes["222222"] {
		t.Errorf("expected codes 111111 and 222222 to be found, got: %v", foundCodes)
	}
}
