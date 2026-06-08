package device

import (
	"context"
	"iter"
	"os"
	"testing"
	"time"

	"main/internal/scalar"

	"go.uber.org/zap"
)

// mockRepository 模拟设备仓库，仅实现 Save 和 Get 以便测试 UpdateRefreshToken
type mockRepository struct {
	dev *Device
}

func (m *mockRepository) Save(ctx context.Context, d *Device) error {
	m.dev = d
	return nil
}

func (m *mockRepository) Get(ctx context.Context, id scalar.ID) (*Device, error) {
	if m.dev != nil && m.dev.ID() == id {
		return m.dev, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockRepository) Delete(ctx context.Context, id scalar.ID) error {
	return nil
}

func (m *mockRepository) Find(ctx context.Context) iter.Seq2[*Device, error] {
	return func(yield func(*Device, error) bool) {}
}

// mockEventBus 模拟事件总线，用于验证发布事件
type mockEventBus struct {
	savedDevices []*Device
}

func (m *mockEventBus) PublishDeviceSaved(ctx context.Context, d *Device) {
	m.savedDevices = append(m.savedDevices, d)
}

func (m *mockEventBus) PublishDeviceDeleted(ctx context.Context, id scalar.ID) {}

func TestService_UpdateRefreshToken(t *testing.T) {
	ctx := context.Background()

	// 1. 初始化依赖项
	repo := &mockRepository{}
	ebus := &mockEventBus{}
	factory := NewFactory()

	// 2. 创建一个测试设备并存入仓库
	deviceID := scalar.NewID()
	initialTime := time.Now().Add(-1 * time.Hour)
	dev, err := factory.New(
		deviceID,
		[]byte("test_cred_id"),
		[]byte("test_pub_key"),
		1,
		initialTime,
		"192.168.1.100",
		"Mozilla/5.0 Chrome/120.0",
	)
	if err != nil {
		t.Fatalf("failed to create initial device: %v", err)
	}
	_ = repo.Save(ctx, dev)

	// 3. 构建 Service 实例
	service, err := NewService(repo, nil, zap.NewNop(), "localhost", []string{"http://localhost"}, ebus, nil, factory)
	if err != nil {
		t.Fatalf("failed to create device service: %v", err)
	}

	// 4. 调用 UpdateRefreshToken
	newJTI := "new_refresh_token_jti"
	newExpire := time.Now().Add(24 * time.Hour)
	newIP := "10.0.0.1"
	newUA := "Mozilla/5.0 Firefox/120.0"

	err = service.UpdateRefreshToken(ctx, deviceID, newJTI, newExpire, newIP, newUA)
	if err != nil {
		t.Fatalf("UpdateRefreshToken failed: %v", err)
	}

	// 5. 验证设备属性是否已被更新并成功保存
	updatedDev := repo.dev
	if updatedDev == nil {
		t.Fatal("device was not saved to repository")
	}

	if updatedDev.RefreshTokenID() != newJTI {
		t.Errorf("expected refresh token JTI %q, got %q", newJTI, updatedDev.RefreshTokenID())
	}
	if !updatedDev.RefreshTokenExpiresAt().Equal(newExpire) {
		t.Errorf("expected refresh token expire time %v, got %v", newExpire, updatedDev.RefreshTokenExpiresAt())
	}
	if updatedDev.LastLoginIP() != newIP {
		t.Errorf("expected last login IP %q, got %q", newIP, updatedDev.LastLoginIP())
	}
	if updatedDev.UserAgent() != newUA {
		t.Errorf("expected user agent %q, got %q", newUA, updatedDev.UserAgent())
	}
	// 最后登录时间应该被更新为当前时间（比 initialTime 更晚）
	if !updatedDev.LastLoginAt().After(initialTime) {
		t.Errorf("expected last login time to be updated, got %v", updatedDev.LastLoginAt())
	}

	// 6. 验证是否向事件总线发布了设备保存的事件
	if len(ebus.savedDevices) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(ebus.savedDevices))
	}
	if ebus.savedDevices[0].ID() != deviceID {
		t.Errorf("expected published device ID %v, got %v", deviceID, ebus.savedDevices[0].ID())
	}
}
