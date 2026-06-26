package pairing

import (
	"context"
	"iter"
	"time"

	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
)

// RequestResolvedEvent 配对请求被解决（批准或拒绝）时发布的事件
type RequestResolvedEvent struct {
	Request *Request
	Status  shared.PairingRequestStatus
}

// Service 负责控制设备配对请求（Request）的完整业务生命周期。
type Service struct {
	repo         Repository
	prCreatedPub pubsub.Topic[*Request]
	prResolvedPub pubsub.Topic[*RequestResolvedEvent]
	prCreatedSub pubsub.Topic[*Request]
	prResolvedSub pubsub.Topic[*RequestResolvedEvent]
}

// NewService 初始化配对领域的 Service 实例。
func NewService(
	repo Repository,
	prCreatedPub pubsub.Topic[*Request],
	prResolvedPub pubsub.Topic[*RequestResolvedEvent],
	prCreatedSub pubsub.Topic[*Request],
	prResolvedSub pubsub.Topic[*RequestResolvedEvent],
) *Service {
	return &Service{
		repo:         repo,
		prCreatedPub: prCreatedPub,
		prResolvedPub: prResolvedPub,
		prCreatedSub: prCreatedSub,
		prResolvedSub: prResolvedSub,
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

	if s.prCreatedPub != nil {
		s.prCreatedPub.Publish(ctx, req)
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

	if s.prResolvedPub != nil {
		s.prResolvedPub.Publish(ctx, &RequestResolvedEvent{
			Request: req,
			Status:  status,
		})
	}

	return nil
}

// SubscribeRequestCreated 订阅新配对请求产生的事件通道。
func (s *Service) SubscribeRequestCreated(ctx context.Context) iter.Seq2[*Request, error] {
	if s.prCreatedSub != nil {
		return s.prCreatedSub.Subscribe(ctx)
	}
	return func(yield func(*Request, error) bool) {}
}

// SubscribeRequestResolved 订阅配对请求被解决（批准或拒绝）的事件通道。
func (s *Service) SubscribeRequestResolved(ctx context.Context) iter.Seq2[*RequestResolvedEvent, error] {
	if s.prResolvedSub != nil {
		return s.prResolvedSub.Subscribe(ctx)
	}
	return func(yield func(*RequestResolvedEvent, error) bool) {}
}

// Find 返回所有当前的配对请求。
func (s *Service) Find(ctx context.Context) iter.Seq2[*Request, error] {
	return s.repo.Find(ctx)
}
