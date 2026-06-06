package inmem

import (
	"context"
	"os"
	"testing"
	"time"

	"main/internal/domain/device"
	"main/internal/infrastructure/localfs"
	"main/internal/scalar"
)

func TestDeviceRepository_CRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "inmem_device_repo_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rawRepo, err := localfs.NewDeviceRepository(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. 初始化缓存装饰器
	cachedRepo, err := NewDeviceRepository(ctx, rawRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 2. 内存初始应为空
	devicesIter := cachedRepo.Find(ctx)
	count := 0
	for _, err := range devicesIter {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 cached devices, got %d", count)
	}

	// 3. 保存一个设备
	id1 := scalar.NewID()
	credID1 := []byte("cached_cred_1")
	pubKey1 := []byte("cached_pub_1")
	device1 := device.FromRepository(
		id1,
		credID1,
		pubKey1,
		5,
		time.Now().Add(-1*time.Hour).Truncate(time.Second),
		time.Now().Truncate(time.Second),
		"127.0.0.1",
		"Test Agent 1",
		"",
		time.Time{},
	)

	err = cachedRepo.Save(ctx, device1)
	if err != nil {
		t.Fatal(err)
	}

	// 4. 从缓存获取该设备 (应该立刻能在内存查到，不需要读取磁盘)
	gotDevice, err := cachedRepo.Get(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if gotDevice.ID() != device1.ID() || string(gotDevice.CredentialID()) != string(device1.CredentialID()) {
		t.Errorf("cached device mismatch: got %+v, want %+v", gotDevice, device1)
	}

	// 5. 再次初始化一个全新的 DeviceRepository 校验是否落盘成功
	newCachedRepo, err := NewDeviceRepository(ctx, rawRepo)
	if err != nil {
		t.Fatal(err)
	}
	gotDevice2, err := newCachedRepo.Get(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if gotDevice2.ID() != id1 {
		t.Errorf("expected device %v, got %v", id1, gotDevice2.ID())
	}

	// 6. 测试删除操作
	err = cachedRepo.Delete(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}

	// 确认缓存与磁盘数据均已被删除
	_, err = cachedRepo.Get(ctx, id1)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist for deleted cached device, got %v", err)
	}

	// 从磁盘新建加载也应该获取不到
	anotherRepo, err := NewDeviceRepository(ctx, rawRepo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = anotherRepo.Get(ctx, id1)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist from disk after delete, got %v", err)
	}
}