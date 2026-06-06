package device

import (
	"main/internal/domain/device"
	"main/internal/shared"
	"main/internal/util"
)

type DTOFactory struct{}

func NewDTOFactory() *DTOFactory {
	return &DTOFactory{}
}

func (f *DTOFactory) New(d *device.Device) *shared.DeviceDTO {
	if d == nil {
		return nil
	}
	return &shared.DeviceDTO{
		ID:          d.ID(),
		Name:        util.ParseUserAgent(d.UserAgent()),
		CreatedAt:   d.CreatedAt(),
		LastLoginAt: d.LastLoginAt(),
		LastLoginIP: d.LastLoginIP(),
		UserAgent:   d.UserAgent(),
	}
}
