package localfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"sync"
	"time"

	"main/internal/domain/device"
	"main/internal/scalar"
	"main/internal/util"
)

// #region Repository 文件系统持久化的设备仓库

// DeviceRepository 负责将 Device 实体持久化到磁盘
type DeviceRepository struct {
	dataDir string
	mu      sync.RWMutex
}

// NewDeviceRepository 创建并初始化设备仓库
func NewDeviceRepository(dataDir string) (*DeviceRepository, error) {
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		return nil, err
	}
	return &DeviceRepository{
		dataDir: dataDir,
	}, nil
}

func (r *DeviceRepository) filename() string {
	return filepath.Join(r.dataDir, "devices.jsonl")
}

type dto struct {
	ID                    string    `json:"id"`
	CredentialID          string    `json:"credentialId"`
	PublicKey             string    `json:"publicKey"`
	SignCount             uint32    `json:"signCount"`
	CreatedAt             time.Time `json:"createdAt"`
	LastLoginAt           time.Time `json:"lastLoginAt"`
	LastLoginIP           string    `json:"lastLoginIp"`
	UserAgent             string    `json:"userAgent"`
	RefreshTokenID        string    `json:"refreshTokenId,omitempty"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt,omitempty"`
}

func (r *DeviceRepository) load() iter.Seq2[*device.Device, error] {
	return func(yield func(*device.Device, error) bool) {
		file, err := os.Open(r.filename())
		if err != nil {
			if os.IsNotExist(err) {
				return
			}
			yield(nil, err)
			return
		}
		defer file.Close()

		dec := json.NewDecoder(file)
		for dec.More() {
			var dto dto
			if err := dec.Decode(&dto); err != nil {
				yield(nil, err)
				return
			}

			id, err := scalar.ParseID(dto.ID)
			if err != nil {
				yield(nil, err)
				return
			}
			credID, err := base64.StdEncoding.DecodeString(dto.CredentialID)
			if err != nil {
				yield(nil, err)
				return
			}
			pubKey, err := base64.StdEncoding.DecodeString(dto.PublicKey)
			if err != nil {
				yield(nil, err)
				return
			}
			d := device.FromRepository(
				id,
				credID,
				pubKey,
				dto.SignCount,
				dto.CreatedAt,
				dto.LastLoginAt,
				dto.LastLoginIP,
				dto.UserAgent,
				dto.RefreshTokenID,
				dto.RefreshTokenExpiresAt,
			)
			if !yield(d, nil) {
				return
			}
		}
	}
}

func (r *DeviceRepository) save(devices []*device.Device) error {
	return util.AtomicSave(r.filename(), func(f *os.File) error {
		enc := json.NewEncoder(f)
		for _, d := range devices {
			dto := dto{
				ID:                    d.ID().String(),
				CredentialID:          base64.StdEncoding.EncodeToString(d.CredentialID()),
				PublicKey:             base64.StdEncoding.EncodeToString(d.PublicKey()),
				SignCount:             d.SignCount(),
				CreatedAt:             d.CreatedAt(),
				LastLoginAt:           d.LastLoginAt(),
				LastLoginIP:           d.LastLoginIP(),
				UserAgent:             d.UserAgent(),
				RefreshTokenID:        d.RefreshTokenID(),
				RefreshTokenExpiresAt: d.RefreshTokenExpiresAt(),
			}
			if err := enc.Encode(dto); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *DeviceRepository) Save(ctx context.Context, dev *device.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var devices []*device.Device
	var found bool
	for d, err := range r.load() {
		if err != nil {
			return err
		}
		if d.ID() == dev.ID() {
			devices = append(devices, dev)
			found = true
		} else {
			devices = append(devices, d)
		}
	}
	if !found {
		devices = append(devices, dev)
	}

	return r.save(devices)
}

func (r *DeviceRepository) Get(ctx context.Context, id scalar.ID) (*device.Device, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for d, err := range r.load() {
		if err != nil {
			return nil, err
		}
		if d.ID() == id {
			return d, nil
		}
	}
	return nil, os.ErrNotExist
}

func (r *DeviceRepository) Delete(ctx context.Context, id scalar.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var newDevices []*device.Device
	for d, err := range r.load() {
		if err != nil {
			return err
		}
		if d.ID() != id {
			newDevices = append(newDevices, d)
		}
	}
	return r.save(newDevices)
}

func (r *DeviceRepository) Find(ctx context.Context) iter.Seq2[*device.Device, error] {
	return func(yield func(*device.Device, error) bool) {
		var devices []*device.Device
		var loadErr error

		func() {
			r.mu.RLock()
			defer r.mu.RUnlock()
			for d, err := range r.load() {
				if err != nil {
					loadErr = err
					return
				}
				devices = append(devices, d)
			}
		}()

		if loadErr != nil {
			yield(nil, loadErr)
			return
		}

		for _, d := range devices {
			if !yield(d, nil) {
				return
			}
		}
	}
}

// #endregion

// 编译时接口检查
var _ device.Repository = (*DeviceRepository)(nil)
