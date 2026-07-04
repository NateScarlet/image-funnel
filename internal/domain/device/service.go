package device

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"main/internal/apperror"
	"main/internal/domain/pairing"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"go.uber.org/zap"
)

type Service struct {
	repo             Repository
	pairingSvc       *pairing.Service
	wa               *webauthn.WebAuthn
	setupToken       string                           // 在 NewService 中生成，不存在并发问题
	sessionDataMap   map[string]*webauthn.SessionData // key is a random string representing the session challenge
	sessionDataMu    sync.RWMutex
	logger           *zap.Logger
	deviceSavedPub   pubsub.Topic[*Device]
	deviceDeletedPub pubsub.Topic[scalar.ID]
	revocationList   RevocationList
	factory          *Factory
}

func NewService(
	repo Repository,
	pairingSvc *pairing.Service,
	logger *zap.Logger,
	rpid string,
	rpOrigins []string,
	deviceSavedPub pubsub.Topic[*Device],
	deviceDeletedPub pubsub.Topic[scalar.ID],
	revocationList RevocationList,
	factory *Factory,
) (*Service, error) {
	// 实际生产中 RPDisplayName 和 RPID 应该从配置读取
	wconfig := &webauthn.Config{
		RPDisplayName: "Image Funnel",
		RPID:          rpid,
		RPOrigins:     rpOrigins,
	}

	wa, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create webauthn instance: %w", err)
	}

	setupToken, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate setup token: %w", err)
	}

	s := &Service{
		repo:             repo,
		pairingSvc:       pairingSvc,
		wa:               wa,
		setupToken:       setupToken,
		sessionDataMap:   make(map[string]*webauthn.SessionData),
		logger:           logger,
		deviceSavedPub:   deviceSavedPub,
		deviceDeletedPub: deviceDeletedPub,
		revocationList:   revocationList,
		factory:          factory,
	}

	return s, nil
}

// SetupToken 返回服务初始化时生成的一次性 SetupToken
func (s *Service) SetupToken() string {
	return s.setupToken
}

// GeneratePairingCode 生成一个 6 位的随机验证码
func generatePairingCode() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate pairing code: %w", err)
	}
	return fmt.Sprintf("%06d", int(b[0])<<16|int(b[1])<<8|int(b[2]))[:6], nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random string: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

func (s *Service) storeSessionData(data *webauthn.SessionData) (string, error) {
	s.sessionDataMu.Lock()
	defer s.sessionDataMu.Unlock()
	key, err := generateRandomString(16)
	if err != nil {
		return "", err
	}
	s.sessionDataMap[key] = data
	// 启动一个定时器清理过期 SessionData
	go func(k string) {
		time.Sleep(5 * time.Minute)
		s.removeSessionData(k)
	}(key)
	return key, nil
}

func (s *Service) getSessionData(key string) (*webauthn.SessionData, bool) {
	s.sessionDataMu.RLock()
	defer s.sessionDataMu.RUnlock()
	data, ok := s.sessionDataMap[key]
	return data, ok
}

func (s *Service) removeSessionData(key string) {
	s.sessionDataMu.Lock()
	defer s.sessionDataMu.Unlock()
	delete(s.sessionDataMap, key)
}

func (s *Service) getUserWithCredentials(ctx context.Context) (*webauthnUser, error) {
	var creds []webauthn.Credential
	for d, err := range s.repo.Find(ctx) {
		if err != nil {
			return nil, err
		}
		creds = append(creds, webauthn.Credential{
			ID:            d.CredentialID(),
			PublicKey:     d.PublicKey(),
			Authenticator: webauthn.Authenticator{SignCount: d.SignCount()},
		})
	}
	return &webauthnUser{
		id:          []byte("image-funnel-user"),
		name:        "image-funnel",
		displayName: "Image Funnel User",
		credentials: creds,
	}, nil
}

// BeginRegistration 开始 WebAuthn 注册流程
func (s *Service) BeginRegistration(ctx context.Context) (*protocol.CredentialCreation, string, error) {
	user, err := s.getUserWithCredentials(ctx)
	if err != nil {
		return nil, "", err
	}

	var exclusions []protocol.CredentialDescriptor
	for _, cred := range user.WebAuthnCredentials() {
		exclusions = append(exclusions, protocol.CredentialDescriptor{
			Type:         protocol.PublicKeyCredentialType,
			CredentialID: cred.ID,
		})
	}

	// Exclude existing credentials
	creation, sessionData, err := s.wa.BeginRegistration(user, webauthn.WithExclusions(exclusions))
	if err != nil {
		return nil, "", err
	}
	sessionKey, err := s.storeSessionData(sessionData)
	if err != nil {
		return nil, "", err
	}
	return creation, sessionKey, nil
}

// FinishRegistration 结束 WebAuthn 注册流程
// 接受 setupToken 和 isTrustedIP，若当前无设备且提供有效的 setupToken，或请求来自可信 IP，则直接注册为可信设备。
// 否则返回 PairingRequest
func (s *Service) FinishRegistration(ctx context.Context, sessionKey string, responseJSON string, providedSetupToken string, isTrustedIP bool, ip string, userAgent string) (*Device, *pairing.Request, error) {
	sessionData, ok := s.getSessionData(sessionKey)
	if !ok {
		return nil, nil, apperror.New("SESSION_EXPIRED", "Registration session expired or invalid", "注册会话已过期或无效")
	}
	s.removeSessionData(sessionKey)

	user, err := s.getUserWithCredentials(ctx)
	if err != nil {
		return nil, nil, err
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader([]byte(responseJSON)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse webauthn response: %w", err)
	}

	cred, err := s.wa.CreateCredential(user, *sessionData, parsedResponse)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create credential: %w", err)
	}

	newDevice, err := s.factory.New(
		scalar.NewID(),
		cred.ID,
		cred.PublicKey,
		cred.Authenticator.SignCount,
		time.Now(),
		ip,
		userAgent,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create device: %w", err)
	}

	// 检查当前是否已有注册设备
	hasDevices := false
	for _, err := range s.repo.Find(ctx) {
		if err != nil {
			return nil, nil, err
		}
		hasDevices = true
		break
	}

	// 如果这是首个设备且提供了正确的 setupToken，或者当前请求来自可信 IP，则免配对直接保存为可信设备
	if (!hasDevices && s.setupToken != "" && providedSetupToken == s.setupToken) || isTrustedIP {
		err = s.repo.Save(ctx, newDevice)
		if err != nil {
			return nil, nil, err
		}
		if s.deviceSavedPub != nil {
			s.deviceSavedPub.Publish(ctx, newDevice)
		}
		// 若使用的是 setupToken，在首次成功后将其清除
		if !hasDevices && s.setupToken != "" && providedSetupToken == s.setupToken {
			s.setupToken = ""
		}
		return newDevice, nil, nil
	}

	// Create pairing request
	code, err := generatePairingCode()
	if err != nil {
		return nil, nil, err
	}
	pr, err := s.pairingSvc.Create(
		ctx,
		code,
		newDevice.ID(),
		newDevice.CredentialID(),
		newDevice.PublicKey(),
		newDevice.SignCount(),
		ip,
		userAgent,
	)
	if err != nil {
		return nil, nil, err
	}

	return nil, pr, nil
}

// BeginLogin 开始登录流程
func (s *Service) BeginLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	user, err := s.getUserWithCredentials(ctx)
	if err != nil {
		return nil, "", err
	}

	assertion, sessionData, err := s.wa.BeginLogin(user)
	if err != nil {
		return nil, "", err
	}

	sessionKey, err := s.storeSessionData(sessionData)
	if err != nil {
		return nil, "", err
	}
	return assertion, sessionKey, nil
}

// FinishLogin 结束登录流程
func (s *Service) FinishLogin(ctx context.Context, sessionKey string, responseJSON string, ip string, userAgent string) (*Device, error) {
	sessionData, ok := s.getSessionData(sessionKey)
	if !ok {
		return nil, apperror.New("SESSION_EXPIRED", "Login session expired or invalid", "登录会话已过期或无效")
	}
	s.removeSessionData(sessionKey)

	user, err := s.getUserWithCredentials(ctx)
	if err != nil {
		return nil, err
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader([]byte(responseJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse webauthn response: %w", err)
	}

	cred, err := s.wa.ValidateLogin(user, *sessionData, parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to validate login: %w", err)
	}

	// Find the device
	var matchedDevice *Device
	for d, err := range s.repo.Find(ctx) {
		if err != nil {
			return nil, err
		}
		if bytes.Equal(d.CredentialID(), cred.ID) {
			matchedDevice = d
			break
		}
	}

	if matchedDevice == nil {
		return nil, apperror.New("DEVICE_NOT_FOUND", "Device not found", "未找到该设备")
	}

	// Update device info
	matchedDevice.UpdateLogin(ip, userAgent, time.Now())
	matchedDevice.UpdateSignCount(cred.Authenticator.SignCount)
	err = s.repo.Save(ctx, matchedDevice)
	if err != nil {
		return nil, err
	}
	if s.deviceSavedPub != nil {
		s.deviceSavedPub.Publish(ctx, matchedDevice)
	}

	return matchedDevice, nil
}

// ApproveRequest 批准配对请求
func (s *Service) ApproveRequest(ctx context.Context, code string) error {
	pr := s.pairingSvc.Get(ctx, code)
	if pr == nil {
		return apperror.New("INVALID_PAIRING_CODE", "Invalid pairing code", "无效的配对码")
	}

	newDevice, err := s.factory.New(
		pr.DeviceID(),
		pr.CredentialID(),
		pr.PublicKey(),
		pr.SignCount(),
		pr.CreatedAt(),
		pr.IP(),
		pr.UserAgent(),
	)
	if err != nil {
		return fmt.Errorf("failed to create device from pairing request: %w", err)
	}

	// Double check if device already exists
	for d, err := range s.repo.Find(ctx) {
		if err != nil {
			return err
		}
		if bytes.Equal(d.CredentialID(), newDevice.CredentialID()) {
			if err := s.pairingSvc.Delete(ctx, code, shared.PairingRequestStatusApproved); err != nil {
				return fmt.Errorf("failed to delete pairing request after device already exists: %w", err)
			}
			return nil // Already registered
		}
	}

	err = s.repo.Save(ctx, newDevice)
	if err != nil {
		return err
	}
	if s.deviceSavedPub != nil {
		s.deviceSavedPub.Publish(ctx, newDevice)
	}

	return s.pairingSvc.Delete(ctx, code, shared.PairingRequestStatusApproved)
}

// RejectRequest 拒绝配对请求
func (s *Service) RejectRequest(ctx context.Context, code string) error {
	return s.pairingSvc.Delete(ctx, code, shared.PairingRequestStatusRejected)
}

// Delete 删除已注册设备，同时自动吊销其活跃的刷新令牌
func (s *Service) Delete(ctx context.Context, id scalar.ID) error {
	// 删除设备前，吊销其刷新令牌
	if device, err := s.repo.Get(ctx, id); err == nil && device.RefreshTokenID() != "" {
		if s.revocationList != nil {
			if addErr := s.revocationList.Add(ctx, device.RefreshTokenID(), device.RefreshTokenExpiresAt()); addErr != nil {
				s.logger.Warn("failed to revoke refresh token on device delete", zap.Error(addErr))
			}
		}
	}

	err := s.repo.Delete(ctx, id)
	if err != nil {
		return err
	}
	if s.deviceDeletedPub != nil {
		s.deviceDeletedPub.Publish(ctx, id)
	}
	return nil
}

// Exists 检查指定 ID 的设备是否存在
func (s *Service) Exists(ctx context.Context, id scalar.ID) (bool, error) {
	_, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// UpdateRefreshToken 更新设备关联的刷新令牌信息，并更新设备信息和最后登录状态
func (s *Service) UpdateRefreshToken(ctx context.Context, deviceID scalar.ID, jti string, expiresAt time.Time, ip string, userAgent string) error {
	device, err := s.repo.Get(ctx, deviceID)
	if err != nil {
		return err
	}
	device.UpdateRefreshToken(jti, expiresAt)
	device.UpdateLogin(ip, userAgent, time.Now())
	err = s.repo.Save(ctx, device)
	if err != nil {
		return err
	}
	if s.deviceSavedPub != nil {
		s.deviceSavedPub.Publish(ctx, device)
	}
	return nil
}

func (s *Service) GetPairingRequest(ctx context.Context, code string) *pairing.Request {
	return s.pairingSvc.Get(ctx, code)
}

// List 列出所有设备
func (s *Service) List(ctx context.Context) ([]*Device, error) {
	var devices []*Device
	for d, err := range s.repo.Find(ctx) {
		if err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// Count 获取设备总数
func (s *Service) Count(ctx context.Context) (int, error) {
	count := 0
	for _, err := range s.repo.Find(ctx) {
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}
