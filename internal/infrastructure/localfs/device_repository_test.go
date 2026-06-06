package localfs

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"main/internal/domain/device"
	"main/internal/scalar"
)

// 测试正常的 CRUD 流程以及流式 Find 遍历
func TestDeviceRepository_CRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "device_repo_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repo, err := NewDeviceRepository(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. 初始读取应该为空
	devicesIter := repo.Find(ctx)
	count := 0
	for _, err := range devicesIter {
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 devices, got %d", count)
	}

	// 2. 创建并保存一个设备
	id1 := scalar.NewID()
	credID1 := []byte("credential_id_1")
	pubKey1 := []byte("public_key_1")
	device1 := device.FromRepository(
		id1,
		credID1,
		pubKey1,
		10,
		time.Now().Add(-1*time.Hour).Truncate(time.Second),
		time.Now().Truncate(time.Second),
		"127.0.0.1",
		"Test User Agent 1",
		"",
		time.Time{},
	)

	err = repo.Save(ctx, device1)
	if err != nil {
		t.Fatal(err)
	}

	// 3. 获取该设备
	gotDevice, err := repo.Get(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}
	if gotDevice.ID() != device1.ID() ||
		string(gotDevice.CredentialID()) != string(device1.CredentialID()) ||
		string(gotDevice.PublicKey()) != string(device1.PublicKey()) ||
		gotDevice.SignCount() != device1.SignCount() ||
		!gotDevice.CreatedAt().Equal(device1.CreatedAt()) ||
		!gotDevice.LastLoginAt().Equal(device1.LastLoginAt()) ||
		gotDevice.LastLoginIP() != device1.LastLoginIP() ||
		gotDevice.UserAgent() != device1.UserAgent() {
		t.Errorf("saved device mismatch: got %+v, want %+v", gotDevice, device1)
	}

	// 4. 新增第二个设备
	id2 := scalar.NewID()
	credID2 := []byte("credential_id_2")
	pubKey2 := []byte("public_key_2")
	device2 := device.FromRepository(
		id2,
		credID2,
		pubKey2,
		20,
		time.Now().Truncate(time.Second),
		time.Now().Truncate(time.Second),
		"192.168.1.1",
		"Test User Agent 2",
		"",
		time.Time{},
	)

	err = repo.Save(ctx, device2)
	if err != nil {
		t.Fatal(err)
	}

	// 5. 遍历确认两台设备
	var list []*device.Device
	for d, err := range repo.Find(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, d)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(list))
	}

	// 6. 删除一台设备
	err = repo.Delete(ctx, id1)
	if err != nil {
		t.Fatal(err)
	}

	// 再次 Get 被删除的设备应当报错 NotExist
	_, err = repo.Get(ctx, id1)
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist for deleted device, got %v", err)
	}

	// 只剩下一台
	gotDevice2, err := repo.Get(ctx, id2)
	if err != nil {
		t.Fatal(err)
	}
	if gotDevice2.ID() != id2 {
		t.Errorf("expected device %v, got %v", id2, gotDevice2.ID())
	}
}

// 测试读取损坏或异常的 jsonl 数据时不忽略错误
func TestRepository_InvalidData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "device_repo_invalid_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repo, err := NewDeviceRepository(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 写入非法的 JSON 数据到 devices.jsonl
	invalidData := "invalid-json-content\n"
	err = os.WriteFile(repo.filename(), []byte(invalidData), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// 读取应当报错
	var parseFailed bool
	for _, err := range repo.Find(ctx) {
		if err != nil {
			parseFailed = true
			break
		}
	}
	if !parseFailed {
		t.Error("expected error on parsing invalid JSON, got nil")
	}

	// 写入合法 JSON 和 ID，但是 Base64 错误
	validIDStr := scalar.NewID().String()
	badBase64Data := fmt.Sprintf(`{"id": "%s", "credentialId": "invalid base64!!!", "publicKey": "cGti"}`+"\n", validIDStr)
	err = os.WriteFile(repo.filename(), []byte(badBase64Data), 0644)
	if err != nil {
		t.Fatal(err)
	}

	parseFailed = false
	for _, err := range repo.Find(ctx) {
		if err != nil {
			parseFailed = true
			break
		}
	}
	if !parseFailed {
		t.Error("expected error on base64 decode failure, got nil")
	}
}
