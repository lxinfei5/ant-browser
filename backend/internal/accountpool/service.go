package accountpool

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AccountPoolService 账号池业务服务
type AccountPoolService struct {
	dao AccountDAO
}

// NewAccountPoolService 创建 AccountPoolService
func NewAccountPoolService(dao AccountDAO) *AccountPoolService {
	return &AccountPoolService{dao: dao}
}

// Create 创建账号；若 input.BoundProfileID 非空则绑定到指定实例
func (s *AccountPoolService) Create(input AccountInput) (*Account, error) {
	if strings.TrimSpace(input.AccountName) == "" {
		return nil, fmt.Errorf("accountName is required")
	}
	account := buildAccountFromInput(uuid.NewString(), input)
	account.CreatedAt = time.Now().Format(time.RFC3339)
	account.UpdatedAt = account.CreatedAt
	if err := s.dao.Upsert(account); err != nil {
		return nil, err
	}
	return s.dao.GetByID(account.AccountID)
}

// Get 查询单个账号
func (s *AccountPoolService) Get(accountID string) (*Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("accountId is required")
	}
	return s.dao.GetByID(accountID)
}

// List 查询账号列表，支持 platform / status / group_id 过滤
func (s *AccountPoolService) List(filter AccountFilter) ([]*Account, error) {
	return s.dao.List(filter)
}

// Update 更新账号；BoundProfileID 为空表示解除绑定
func (s *AccountPoolService) Update(accountID string, input AccountInput) (*Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("accountId is required")
	}
	existing, err := s.dao.GetByID(accountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.AccountName) == "" {
		return nil, fmt.Errorf("accountName is required")
	}

	account := buildAccountFromInput(accountID, input)
	account.CreatedAt = existing.CreatedAt
	account.UpdatedAt = time.Now().Format(time.RFC3339)
	account.LastUsedAt = existing.LastUsedAt
	account.DeletedAt = existing.DeletedAt
	if err := s.dao.Upsert(account); err != nil {
		return nil, err
	}
	return s.dao.GetByID(accountID)
}

// Delete 软删除账号
func (s *AccountPoolService) Delete(accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return fmt.Errorf("accountId is required")
	}
	return s.dao.SoftDelete(accountID, time.Now().Format(time.RFC3339))
}

func buildAccountFromInput(accountID string, input AccountInput) *Account {
	return &Account{
		AccountID:      accountID,
		AccountName:    strings.TrimSpace(input.AccountName),
		Platform:       strings.TrimSpace(input.Platform),
		AccountRef:     strings.TrimSpace(input.AccountRef),
		BoundProfileID: strings.TrimSpace(input.BoundProfileID),
		ProxyID:        strings.TrimSpace(input.ProxyID),
		Status:         normalizeStatus(input.Status),
		CooldownUntil:  strings.TrimSpace(input.CooldownUntil),
		Notes:          input.Notes,
		Tags:           normalizeTags(input.Tags),
		GroupID:        strings.TrimSpace(input.GroupID),
		Credential:     nonNilMap(input.Credential),
		Metadata:       nonNilMap(input.Metadata),
	}
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "disabled":
		return "disabled"
	case "":
		return "active"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}