package accountpool

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ant-chrome/backend/internal/tagutil"

	"github.com/google/uuid"
)

// ProxyResolver 由上层（App）实现，供账号池在代理失败冷却时解析绑定实例当前使用的代理 ID。
// 保持 accountpool 不依赖 browser/launchcode。
type ProxyResolver interface {
	// ProxyIDForProfile 解析绑定实例当前使用的代理 ID；无则返回空串。
	ProxyIDForProfile(profileID string) string
}

// AccountPoolService 账号池业务服务
type AccountPoolService struct {
	dao           AccountDAO
	db            *sql.DB
	proxyResolver ProxyResolver
}

// NewAccountPoolService 创建 AccountPoolService
func NewAccountPoolService(dao AccountDAO) *AccountPoolService {
	return &AccountPoolService{dao: dao}
}

// SetDB 注入底层 *sql.DB，供 UpdateAccountStatus 作为默认 runner 使用
func (s *AccountPoolService) SetDB(db *sql.DB) {
	s.db = db
}

// SetProxyResolver 注入代理解析器（代理失败冷却时按实例解析代理）
func (s *AccountPoolService) SetProxyResolver(r ProxyResolver) {
	s.proxyResolver = r
}

// 联系方式校验（无新依赖，仅用 stdlib regexp）。
var (
	// 邮箱：保守口径，非空即需 user@host.tld 形状
	emailRegexp = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	// 手机号：归一后允许前导 +，5~15 位数字
	phoneRegexp = regexp.MustCompile(`^\+?[0-9]{5,15}$`)
)

// Create 创建账号；若 input.BoundProfileID 非空则绑定到指定实例
func (s *AccountPoolService) Create(input AccountInput) (*Account, error) {
	if strings.TrimSpace(input.AccountName) == "" {
		return nil, fmt.Errorf("accountName is required")
	}
	if err := s.validateContactForWrite(input, ""); err != nil {
		return nil, err
	}
	account := buildAccountFromInput(uuid.NewString(), input)
	account.CreatedAt = time.Now().Format(time.RFC3339)
	account.UpdatedAt = account.CreatedAt
	if err := s.dao.Upsert(account); err != nil {
		return nil, mapUniqueContactError(err)
	}
	return s.dao.GetByID(account.AccountID)
}

// Get 查询单个账号
func (s *AccountPoolService) Get(accountID string) (*Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("accountId is required")
	}
	acc, err := s.dao.GetByID(accountID)
	if err != nil {
		return nil, err
	}
	s.refreshCooldownExpiry(acc)
	return acc, nil
}

// List 查询账号列表，支持 status / group_id 过滤
func (s *AccountPoolService) List(filter AccountFilter) ([]*Account, error) {
	accounts, err := s.dao.List(filter)
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		s.refreshCooldownExpiry(acc)
	}
	return accounts, nil
}

// refreshCooldownExpiry 惰性到期自愈：账号处于 cooldown 且截止时间已过时，
// 透明地把状态复位为 active 并清空 cooldown_until（冷却不再是单向阀）。
//
// 租约子系统被移除后，曾经的「冷却到期自动恢复」路径随之删除，导致代理失败冷却
// 一旦写入便永不解除。这里在读取侧（List/Get，UI 列表与启动校验都经此）做惰性
// 恢复，并尽力落库一次（失败不影响本次读取结果，下次读取仍会重试收敛）。
// 仅在确实发生状态跃迁时写库，收敛后零写入。disabled / 空截止时间的账号不受影响。
func (s *AccountPoolService) refreshCooldownExpiry(acc *Account) {
	if acc == nil || acc.Status != AccountStatusCooldown {
		return
	}
	until := strings.TrimSpace(acc.CooldownUntil)
	if until == "" {
		return
	}
	t, err := time.Parse(time.RFC3339, until)
	if err != nil || time.Now().Before(t) {
		return // 未到期或截止时间不可解析：保持冷却
	}
	acc.Status = AccountStatusActive
	acc.CooldownUntil = ""
	now := time.Now().Format(time.RFC3339)
	acc.UpdatedAt = now
	// 尽力落库；s.db 未注入时退回 dao 默认 runner。
	_ = s.dao.UpdateAccountStatus(s.db, acc.AccountID, AccountStatusActive, "", acc.LastUsedAt, now)
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
	if err := s.validateContactForWrite(input, accountID); err != nil {
		return nil, err
	}

	account := buildAccountFromInput(accountID, input)
	account.CreatedAt = existing.CreatedAt
	account.UpdatedAt = time.Now().Format(time.RFC3339)
	account.LastUsedAt = existing.LastUsedAt
	account.DeletedAt = existing.DeletedAt
	// cooldown_until 是后端维护的账号健康状态（仅 CooldownAccountsByProxy 写），
	// 前端编辑表单不携带该字段；这里像 last_used_at 一样从既有行继承，
	// 避免一次无关编辑把冷却截止时间静默清空（保留 status='cooldown' 却丢失截止时间，
	// 并绕过 CooldownAccountsByProxy 的“不缩短更长冷却”守卫）。
	account.CooldownUntil = existing.CooldownUntil
	if err := s.dao.Upsert(account); err != nil {
		return nil, mapUniqueContactError(err)
	}
	return s.dao.GetByID(accountID)
}

// RemoveTagFromAll 从全部账号的 tags 中移除指定标签(大小写、空白不敏感)。
// 用于「删除标签三清」中的账号一清。返回受影响账号数;单个账号 Upsert 失败不中断,
// 累计后随受影响数一并返回(尽力而为,幂等,可安全重试收敛)。软删账号不在 List 内,不受影响。
func (s *AccountPoolService) RemoveTagFromAll(tag string) (int, error) {
	if tagutil.Normalize(tag) == "" {
		return 0, nil
	}
	accounts, err := s.dao.List(AccountFilter{})
	if err != nil {
		return 0, err
	}
	affected := 0
	var firstErr error
	failCount := 0
	for _, acc := range accounts {
		if !tagutil.ContainsFold(acc.Tags, tag) {
			continue
		}
		filtered := acc.Tags[:0]
		for _, t := range acc.Tags {
			if tagutil.Normalize(t) != tagutil.Normalize(tag) {
				filtered = append(filtered, t)
			}
		}
		acc.Tags = filtered
		if err := s.dao.Upsert(acc); err != nil {
			failCount++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		affected++
	}
	if firstErr != nil {
		return affected, fmt.Errorf("移除账号标签部分失败(%d 个失败): %w", failCount, firstErr)
	}
	return affected, nil
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
		AccountRef:     strings.TrimSpace(input.AccountRef),
		Email:          normalizeEmail(input.Email),
		Phone:          normalizePhone(input.Phone),
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
	case "cooldown":
		return "cooldown"
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

// normalizeEmail 邮箱归一：trim + 转小写。
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// normalizePhone 手机号归一：trim + 去除空格/连字符/括号，保留前导 +。
func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(phone))
	for _, r := range phone {
		switch r {
		case ' ', '-', '(', ')':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// validateContactForWrite 归一并校验 email/phone 格式，随后做占用预检。
// excludeID 用于 Update 时排除自身（避免与自身冲突误判）。返回友好中文错误。
func (s *AccountPoolService) validateContactForWrite(input AccountInput, excludeID string) error {
	email := normalizeEmail(input.Email)
	if email != "" && !emailRegexp.MatchString(email) {
		return fmt.Errorf("邮箱格式不正确")
	}
	phone := normalizePhone(input.Phone)
	if phone != "" && !phoneRegexp.MatchString(phone) {
		return fmt.Errorf("手机号格式不正确")
	}
	return s.checkContactUnique(email, phone, excludeID)
}

// checkContactUnique 在写库前检查 email/phone 是否已被其他（未删除）账号占用，给出友好中文错误。
// DB 的 LOWER() 部分唯一索引是兜底；此处预检以返回可读错误。email/phone 均为可选，空串跳过。
func (s *AccountPoolService) checkContactUnique(email, phone, excludeID string) error {
	if email == "" && phone == "" {
		return nil
	}
	accounts, err := s.dao.List(AccountFilter{})
	if err != nil {
		return err
	}
	for _, acc := range accounts {
		if acc.AccountID == excludeID {
			continue
		}
		if email != "" && acc.Email != "" && strings.EqualFold(acc.Email, email) {
			return fmt.Errorf("邮箱已被另一个账号使用")
		}
		if phone != "" && acc.Phone != "" && acc.Phone == phone {
			return fmt.Errorf("手机号已被另一个账号使用")
		}
	}
	return nil
}

// mapUniqueContactError 把 DB 部分唯一索引冲突（兜底）翻译为友好中文错误；非唯一冲突则原样返回。
func mapUniqueContactError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unique constraint") && !strings.Contains(msg, "constraint failed") {
		return err
	}
	if strings.Contains(msg, "email") {
		return fmt.Errorf("邮箱已被另一个账号使用")
	}
	if strings.Contains(msg, "phone") {
		return fmt.Errorf("手机号已被另一个账号使用")
	}
	return err
}

// ──────────────────────────────────────────────────────────────────────────
// 代理失败冷却（账号健康）
// ──────────────────────────────────────────────────────────────────────────

// CooldownAccountsByProxy 将所有绑定到指定代理的账号置为 cooldown，cooldown_until=now+cooldownSec。
// 绑定关系二选一：account.proxy_id == proxyID，或 account.bound_profile_id 经 ProxyResolver 解析的代理 == proxyID。
// cooldownSec<=0 时默认 3600。返回受影响的账号 ID 列表。
func (s *AccountPoolService) CooldownAccountsByProxy(proxyID string, cooldownSec int) ([]string, error) {
	proxyID = strings.TrimSpace(proxyID)
	if proxyID == "" {
		return nil, fmt.Errorf("proxyId is required")
	}
	if cooldownSec <= 0 {
		cooldownSec = 3600
	}
	accounts, err := s.dao.List(AccountFilter{})
	if err != nil {
		return nil, fmt.Errorf("查询账号列表失败: %w", err)
	}
	now := time.Now().UTC()
	cooldownUntil := now.Add(time.Duration(cooldownSec) * time.Second).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)

	affected := make([]string, 0)
	for _, acc := range accounts {
		if !s.accountBoundToProxy(acc, proxyID) {
			continue
		}
		// 已经在更长的冷却里则不缩短。
		if acc.Status == AccountStatusCooldown && acc.CooldownUntil != "" && acc.CooldownUntil > cooldownUntil {
			continue
		}
		if err := s.dao.UpdateAccountStatus(s.db, acc.AccountID, AccountStatusCooldown, cooldownUntil, acc.LastUsedAt, nowStr); err != nil {
			continue
		}
		affected = append(affected, acc.AccountID)
	}
	return affected, nil
}

// accountBoundToProxy 判断账号是否绑定到指定代理（直接 proxy_id 或经实例解析）。
func (s *AccountPoolService) accountBoundToProxy(acc *Account, proxyID string) bool {
	if acc == nil {
		return false
	}
	if strings.TrimSpace(acc.ProxyID) == proxyID {
		return true
	}
	if s.proxyResolver != nil && strings.TrimSpace(acc.BoundProfileID) != "" {
		return strings.TrimSpace(s.proxyResolver.ProxyIDForProfile(acc.BoundProfileID)) == proxyID
	}
	return false
}
