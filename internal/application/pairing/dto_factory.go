package pairing

import (
	"main/internal/domain/pairing"
	"main/internal/shared"
)

type DTOFactory struct{}

func NewDTOFactory() *DTOFactory {
	return &DTOFactory{}
}

func (f *DTOFactory) New(pr *pairing.Request, status shared.PairingRequestStatus) *shared.PairingRequestDTO {
	if pr == nil {
		return nil
	}
	return &shared.PairingRequestDTO{
		Code:      pr.Code(),
		CreatedAt: pr.CreatedAt(),
		Status:    status,
	}
}
