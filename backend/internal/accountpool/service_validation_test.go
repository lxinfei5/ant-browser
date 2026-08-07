package accountpool

import (
	"strings"
	"testing"
)

// newTestService 构造一个带 v21 形态 schema + email/phone 唯一索引的服务。
func newTestService(t *testing.T) *AccountPoolService {
	t.Helper()
	dao := newTestDAO(t)
	return NewAccountPoolService(dao)
}

// TestCreate_EmailValidation 覆盖邮箱：合法/非法/空/大小写归一。
func TestCreate_EmailValidation(t *testing.T) {
	cases := []struct {
		name      string
		email     string
		wantErr   string // 空串表示期望成功
		wantEmail string // 成功时期望落库的归一值
	}{
		{"合法", "user@example.com", "", "user@example.com"},
		{"大小写归一", "User@Example.COM", "", "user@example.com"},
		{"首尾空白", "  a@b.co  ", "", "a@b.co"},
		{"空(可选)", "", "", ""},
		{"缺@与域名", "notanemail", "邮箱格式不正确", ""},
		{"缺顶级域", "a@b", "邮箱格式不正确", ""},
		{"缺本地名", "@x.com", "邮箱格式不正确", ""},
		{"含空格", "a b@c.com", "邮箱格式不正确", ""},
		{"双@", "a@@b.com", "邮箱格式不正确", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := newTestService(t)
			acc, err := svc.Create(AccountInput{AccountName: "n", Email: c.email})
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("email=%q 期望错误 %q, got err=%v", c.email, c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("email=%q 期望成功, got err=%v", c.email, err)
			}
			if acc.Email != c.wantEmail {
				t.Fatalf("email=%q 落库值 = %q, want %q", c.email, acc.Email, c.wantEmail)
			}
		})
	}
}

// TestCreate_PhoneValidation 覆盖手机号：合法/非法/空/E164/分隔符归一。
func TestCreate_PhoneValidation(t *testing.T) {
	cases := []struct {
		name      string
		phone     string
		wantErr   string
		wantPhone string
	}{
		{"纯数字", "13800138000", "", "13800138000"},
		{"E164带+", "+8613800138000", "", "+8613800138000"},
		{"去空格连字符", "+86 138-0013-8000", "", "+8613800138000"},
		{"去括号", "+1 (555) 123-4567", "", "+15551234567"},
		{"空(可选)", "", "", ""},
		{"过短", "123", "手机号格式不正确", ""},
		{"含字母", "abc12345", "手机号格式不正确", ""},
		{"多个+", "++12345", "手机号格式不正确", ""},
		{"+不在开头", "13+8000", "手机号格式不正确", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := newTestService(t)
			acc, err := svc.Create(AccountInput{AccountName: "n", Phone: c.phone})
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("phone=%q 期望错误 %q, got err=%v", c.phone, c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("phone=%q 期望成功, got err=%v", c.phone, err)
			}
			if acc.Phone != c.wantPhone {
				t.Fatalf("phone=%q 落库值 = %q, want %q", c.phone, acc.Phone, c.wantPhone)
			}
		})
	}
}

// TestCreate_EmailUniqueness 两个账号同邮箱：第二个失败并返回友好中文错误；大小写变体也算冲突。
func TestCreate_EmailUniqueness(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Create(AccountInput{AccountName: "A", Email: "dup@example.com"}); err != nil {
		t.Fatalf("第一个账号创建失败: %v", err)
	}
	// 完全相同的邮箱
	if _, err := svc.Create(AccountInput{AccountName: "B", Email: "dup@example.com"}); err == nil ||
		!strings.Contains(err.Error(), "邮箱已被另一个账号使用") {
		t.Fatalf("重复邮箱应返回友好错误, got: %v", err)
	}
	// 大小写变体也应判冲突
	if _, err := svc.Create(AccountInput{AccountName: "C", Email: "DUP@Example.COM"}); err == nil ||
		!strings.Contains(err.Error(), "邮箱已被另一个账号使用") {
		t.Fatalf("大小写变体邮箱应判冲突, got: %v", err)
	}
}

// TestCreate_PhoneUniqueness 两个账号同手机号：第二个失败；不同写法规一后同号也算冲突。
func TestCreate_PhoneUniqueness(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Create(AccountInput{AccountName: "A", Phone: "+8613800138000"}); err != nil {
		t.Fatalf("第一个账号创建失败: %v", err)
	}
	// 不同写法（空格/连字符）归一后同号
	if _, err := svc.Create(AccountInput{AccountName: "B", Phone: "+86 138-0013-8000"}); err == nil ||
		!strings.Contains(err.Error(), "手机号已被另一个账号使用") {
		t.Fatalf("归一后同号应返回友好错误, got: %v", err)
	}
}

// TestUpdate_EmailUniqueness 更新时：不能改成别人的邮箱，但保留自身邮箱可通过。
func TestUpdate_EmailUniqueness(t *testing.T) {
	svc := newTestService(t)
	a, err := svc.Create(AccountInput{AccountName: "A", Email: "a@example.com"})
	if err != nil {
		t.Fatalf("创建 A 失败: %v", err)
	}
	b, err := svc.Create(AccountInput{AccountName: "B", Email: "b@example.com"})
	if err != nil {
		t.Fatalf("创建 B 失败: %v", err)
	}

	// B 改成 A 的邮箱 -> 冲突
	if _, err := svc.Update(b.AccountID, AccountInput{AccountName: "B", Email: "a@example.com"}); err == nil ||
		!strings.Contains(err.Error(), "邮箱已被另一个账号使用") {
		t.Fatalf("改成他人邮箱应判冲突, got: %v", err)
	}
	// B 保留自身邮箱 -> 通过（excludeID 排除自身）
	if _, err := svc.Update(b.AccountID, AccountInput{AccountName: "B", Email: "b@example.com"}); err != nil {
		t.Fatalf("保留自身邮箱应通过, got: %v", err)
	}
	// A 清空邮箱后，B 可改用 a@example.com
	if _, err := svc.Update(a.AccountID, AccountInput{AccountName: "A", Email: ""}); err != nil {
		t.Fatalf("A 清空邮箱失败: %v", err)
	}
	if _, err := svc.Update(b.AccountID, AccountInput{AccountName: "B", Email: "a@example.com"}); err != nil {
		t.Fatalf("A 清空后 B 应可使用该邮箱, got: %v", err)
	}
}

// TestCreate_AccountNameRequired accountName 仍是唯一必填：只给邮箱不给名称 -> 报错。
func TestCreate_AccountNameRequired(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Create(AccountInput{Email: "a@example.com"}); err == nil ||
		!strings.Contains(err.Error(), "accountName is required") {
		t.Fatalf("缺 accountName 应报错, got: %v", err)
	}
}

// TestCreate_NameOnlyAccount 历史仅命名账号仍可创建/编辑（email/phone/accountRef 均可选）。
func TestCreate_NameOnlyAccount(t *testing.T) {
	svc := newTestService(t)
	acc, err := svc.Create(AccountInput{AccountName: "仅命名"})
	if err != nil {
		t.Fatalf("仅命名账号应可创建, got: %v", err)
	}
	if acc.Email != "" || acc.Phone != "" || acc.AccountRef != "" {
		t.Fatalf("仅命名账号的身份锚点应为空, got: %+v", acc)
	}
	// 仍可编辑（更新时不强制任何锚点）
	if _, err := svc.Update(acc.AccountID, AccountInput{AccountName: "仅命名2"}); err != nil {
		t.Fatalf("仅命名账号应可更新, got: %v", err)
	}
}

// TestCreate_TagsNormalized 服务即标签：tags 写库前归一（trim+小写+去重）。
func TestCreate_TagsNormalized(t *testing.T) {
	svc := newTestService(t)
	acc, err := svc.Create(AccountInput{AccountName: "n", Tags: []string{" XHS ", "xhs", "VIP"}})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if len(acc.Tags) != 2 || acc.Tags[0] != "xhs" || acc.Tags[1] != "vip" {
		t.Fatalf("tags 未归一: %v", acc.Tags)
	}
}

// TestMapUniqueContactError_Backstop 直接打 DB 唯一索引兜底路径：先用 dao.Upsert 绕过
// 服务的 List 预检写入一行，再经 service.Create 触发 idx_accounts_email/phone 冲突，
// 断言 mapUniqueContactError 把原始英文 SQLite 错误翻译为友好中文（不泄露索引名）。
// 该路径在正常单线程流程里被预检先行拦截，仅并发 TOCTOU 时才会走到，此前完全无测试。
func TestMapUniqueContactError_Backstop(t *testing.T) {
	t.Run("email", func(t *testing.T) {
		dao := newTestDAO(t)
		// 直接落库，绕过 Create 的预检（等价于并发 TOCTOU 下另一条已写入的记录）
		if err := dao.Upsert(&Account{AccountID: "seed", AccountName: "seed", Email: "race@example.com"}); err != nil {
			t.Fatalf("预置失败: %v", err)
		}
		// 大小写变体同 email 直写 -> 命中 LOWER(email) 唯一索引 -> 原始英文错误
		rawErr := dao.Upsert(&Account{AccountID: "seed2", AccountName: "seed2", Email: "RACE@example.com"})
		if rawErr == nil {
			t.Fatalf("同 email(大小写变体) 直写应触发唯一索引, got nil")
		}
		mapped := mapUniqueContactError(rawErr)
		if mapped == nil || !strings.Contains(mapped.Error(), "邮箱已被另一个账号使用") {
			t.Fatalf("email 冲突应翻译为友好错误, got: %v (raw: %v)", mapped, rawErr)
		}
		if strings.Contains(strings.ToLower(mapped.Error()), "idx_accounts") || strings.Contains(mapped.Error(), "index") {
			t.Fatalf("友好错误不应泄露索引名/SQL, got: %v", mapped)
		}
	})
	t.Run("phone", func(t *testing.T) {
		dao := newTestDAO(t)
		if err := dao.Upsert(&Account{AccountID: "seed", AccountName: "seed", Phone: "+8613800138000"}); err != nil {
			t.Fatalf("预置失败: %v", err)
		}
		rawErr := dao.Upsert(&Account{AccountID: "seed2", AccountName: "seed2", Phone: "+8613800138000"})
		if rawErr == nil {
			t.Fatalf("同 phone 直写应触发唯一索引, got nil")
		}
		mapped := mapUniqueContactError(rawErr)
		if mapped == nil || !strings.Contains(mapped.Error(), "手机号已被另一个账号使用") {
			t.Fatalf("phone 冲突应翻译为友好错误, got: %v (raw: %v)", mapped, rawErr)
		}
	})
}

// TestUpdate_PreservesCooldownUntil 锁定 Update 的字段保留契约：cooldown_until 是后端维护的
// 账号健康状态（仅 CooldownAccountsByProxy 写），前端编辑表单不携带该字段，Update 必须从既有行
// 继承它（如同 last_used_at），否则一次无关编辑会静默清空冷却截止时间、保留 status='cooldown'
// 却丢失截止时间，并绕过 CooldownAccountsByProxy 的“不缩短更长冷却”守卫。
func TestUpdate_PreservesCooldownUntil(t *testing.T) {
	dao := newTestDAO(t)
	svc := NewAccountPoolService(dao)
	svc.SetDB(dao.db) // UpdateAccountStatus 需要底层 runner

	acc, err := svc.Create(AccountInput{AccountName: "A"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 驱动账号进入冷却（截止时间在未来）
	future := "2031-01-01T00:00:00Z"
	if err := dao.UpdateAccountStatus(dao.db, acc.AccountID, AccountStatusCooldown, future, "", future); err != nil {
		t.Fatalf("置冷却失败: %v", err)
	}

	// 用最小输入编辑（不携带 cooldownUntil；status 由前端状态选择器回显为 cooldown，与本真实路径一致）
	updated, err := svc.Update(acc.AccountID, AccountInput{AccountName: "A-renamed", Status: AccountStatusCooldown, Notes: "edit"})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.CooldownUntil != future {
		t.Fatalf("Update 应保留 cooldown_until, got %q, want %q", updated.CooldownUntil, future)
	}
	if updated.Status != AccountStatusCooldown {
		t.Fatalf("Update 应保留 status='cooldown', got %q", updated.Status)
	}

	// 二次确认落库值（非仅返回值）
	got, err := dao.GetByID(acc.AccountID)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.CooldownUntil != future {
		t.Fatalf("落库 cooldown_until 被清空: got %q, want %q", got.CooldownUntil, future)
	}
}

// TestRefreshCooldownExpiry 锁定冷却惰性到期自愈：过期的 cooldown 账号在 List/Get 读取时
// 透明复位为 active 并清空 cooldown_until（冷却不再是单向阀）；未到期/disabled 不受影响。
func TestRefreshCooldownExpiry(t *testing.T) {
	dao := newTestDAO(t)
	svc := NewAccountPoolService(dao)
	svc.SetDB(dao.db)

	acc, err := svc.Create(AccountInput{AccountName: "A"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 过期冷却（截止时间在过去）
	past := "2020-01-01T00:00:00Z"
	if err := dao.UpdateAccountStatus(dao.db, acc.AccountID, AccountStatusCooldown, past, "", past); err != nil {
		t.Fatalf("置过期冷却失败: %v", err)
	}

	// List 读取应触发惰性恢复
	list, err := svc.List(AccountFilter{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List 应返回 1 个账号, got %d", len(list))
	}
	if list[0].Status != AccountStatusActive || list[0].CooldownUntil != "" {
		t.Fatalf("过期冷却应惰性复位为 active, got status=%q cooldownUntil=%q", list[0].Status, list[0].CooldownUntil)
	}
	// 落库值也已复位（自愈被持久化）
	got, err := dao.GetByID(acc.AccountID)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.Status != AccountStatusActive || got.CooldownUntil != "" {
		t.Fatalf("自愈应落库, got status=%q cooldownUntil=%q", got.Status, got.CooldownUntil)
	}

	// 未到期冷却不受影响
	acc2, err := svc.Create(AccountInput{AccountName: "B"})
	if err != nil {
		t.Fatalf("创建 B 失败: %v", err)
	}
	future := "2031-01-01T00:00:00Z"
	if err := dao.UpdateAccountStatus(dao.db, acc2.AccountID, AccountStatusCooldown, future, "", future); err != nil {
		t.Fatalf("置未到期冷却失败: %v", err)
	}
	got2, err := svc.Get(acc2.AccountID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got2.Status != AccountStatusCooldown || got2.CooldownUntil != future {
		t.Fatalf("未到期冷却应保持, got status=%q cooldownUntil=%q", got2.Status, got2.CooldownUntil)
	}
}
