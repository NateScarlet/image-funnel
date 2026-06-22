package device

import (
	"context"
	"main/internal/shared"
)

// Devices 获取已注册的设备列表
func (h *Handler) Devices(ctx context.Context) ([]*shared.DeviceDTO, error) {
	devices, err := h.service.List(ctx)
	if err != nil {
		return nil, err
	}
	var dtos []*shared.DeviceDTO
	for _, d := range devices {
		dtos = append(dtos, h.dtoFactory.New(d))
	}
	return dtos, nil
}