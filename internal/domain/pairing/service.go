package pairing

import (
	"context"
	"iter"
	"time"

	"main/internal/scalar"
	"main/internal/shared"
)

// EventBus 定义了配对领域事件的发布与订阅标准接口，
// 这样可以避免将领域服务直接绑定到某个具体的消息中介。
type EventBus interface {
	PublishPairingRequestCreated(ctx context.Context, dto *shared.PairingRequestDTO)
	PublishPairingRequestUpdated(ctx context.Context, dto *shared.PairingRequestDTO)
	SubscribePairingRequestCreated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error]
	SubscribePairingRequestUpdated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error]
}

// Service 负责控制设备配对请求（Request）的完整业务生命周期。
type Service struct {
	repo Repository
	ebus EventBus
}

// NewService 初始化配对领域的 Service 实例。
func NewService(repo Repository, ebus EventBus) *Service {
	return &Service{
		repo: repo,
		ebus: ebus,
	}
}

// Create 生成并保存一个新的配对请求，成功后向总线发布配对创建的通知。
func (s *Service) Create(
	ctx context.Context,
	code string,
	deviceID scalar.ID,
	credentialID []byte,
	publicKey []byte,
	signCount uint32,
	ip string,
	userAgent string,
) (*Request, error) {
	req := FromRepository(code, deviceID, credentialID, publicKey, signCount, time.Now(), ip, userAgent)
	err := s.repo.Save(ctx, req)
	if err != nil {
		return nil, err
	}

	if s.ebus != nil {
		s.ebus.PublishPairingRequestCreated(ctx, &shared.PairingRequestDTO{
			Code:      code,
			CreatedAt: req.CreatedAt(),
			Status:    shared.PairingRequestStatusPending,
		})
	}

	return req, nil
}

// Get 查询对应的配对请求，找不到时直接返回 nil 维持 API 层的简洁性。
func (s *Service) Get(ctx context.Context, code string) *Request {
	req, err := s.repo.Get(ctx, code)
	if err != nil {
		return nil
	}
	return req
}

// Delete 移除指定的配对请求，并在移除成功后向总线发布配对请求更新的通知。
func (s *Service) Delete(ctx context.Context, code string, status shared.PairingRequestStatus) error {
	req, err := s.repo.Get(ctx, code)
	if err != nil {
		return err
	}

	err = s.repo.Delete(ctx, code)
	if err != nil {
		return err
	}

	if s.ebus != nil {
		s.ebus.PublishPairingRequestUpdated(ctx, &shared.PairingRequestDTO{
			Code:      code,
			CreatedAt: req.CreatedAt(),
			Status:    status,
		})
	}

	return nil
}

// SubscribeRequestCreated 订阅新配对请求产生的事件通道。
func (s *Service) SubscribeRequestCreated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	if s.ebus != nil {
		return s.ebus.SubscribePairingRequestCreated(ctx)
	}
	return func(yield func(*shared.PairingRequestDTO, error) bool) {}
}

// SubscribeRequestUpdated 订阅配对请求状态更新或删除的事件通道。
func (s *Service) SubscribeRequestUpdated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	if s.ebus != nil {
		return s.ebus.SubscribePairingRequestUpdated(ctx)
	}
	return func(yield func(*shared.PairingRequestDTO, error) bool) {}
}

// Find 返回所有当前的配对请求。
func (s *Service) Find(ctx context.Context) iter.Seq2[*Request, error] {
	return s.repo.Find(ctx)
}
