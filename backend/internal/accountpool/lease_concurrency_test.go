package accountpool

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// applyLeasesSchema 重建 account_leases 表及其偏唯一索引，模拟 v14 迁移。
func applyLeasesSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS account_leases (
			lease_id        TEXT PRIMARY KEY,
			account_id      TEXT NOT NULL,
			profile_id      TEXT NOT NULL,
			worker_id       TEXT NOT NULL DEFAULT '',
			purpose         TEXT NOT NULL DEFAULT 'scrape',
			status          TEXT NOT NULL DEFAULT 'held',
			cdp_endpoint    TEXT NOT NULL DEFAULT '',
			leased_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at      TEXT NOT NULL DEFAULT '',
			heartbeat_at    TEXT NOT NULL DEFAULT '',
			released_at     TEXT NOT NULL DEFAULT '',
			release_result  TEXT NOT NULL DEFAULT '',
			auto_started    INTEGER NOT NULL DEFAULT 0,
			metadata_json   TEXT NOT NULL DEFAULT '{}',
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_leases_one_held ON account_leases(account_id) WHERE status='held'`,
		`CREATE INDEX IF NOT EXISTS idx_leases_status ON account_leases(status)`,
		`CREATE INDEX IF NOT EXISTS idx_leases_expires_at ON account_leases(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_leases_worker_id ON account_leases(worker_id)`,
		`CREATE INDEX IF NOT EXISTS idx_leases_account_id ON account_leases(account_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("建租约表失败: %v", err)
		}
	}
}

// newTestLeaseDAO 构造一个已注入租约 DAO + DB 的服务，用于并发与状态测试。
func newTestLeaseDAO(t *testing.T) (*AccountPoolService, *SQLiteAccountDAO, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	// 与生产一致：单连接，避免内存库在多连接下各自独立（多 goroutine 并发时会触发）
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	applyAccountsSchema(t, db)
	applyLeasesSchema(t, db)

	accountDAO := NewSQLiteAccountDAO(db)
	svc := NewAccountPoolService(accountDAO)
	svc.SetLeaseDAO(NewSQLiteLeaseDAO(db))
	svc.SetDB(db)
	return svc, accountDAO, db
}

func TestLeaseConcurrency_SingleWinner(t *testing.T) {
	svc, _, _ := newTestLeaseDAO(t)

	// 单个候选账号，绑定实例但未运行
	if err := svc.dao.Upsert(&Account{
		AccountID:      "acc-1",
		AccountName:     "A",
		Platform:        "xhs",
		BoundProfileID:  "prof-1",
		Status:          AccountStatusActive,
		Tags:            []string{},
		Credential:      map[string]any{},
		Metadata:        map[string]any{},
	}); err != nil {
		t.Fatalf("Upsert 账号失败: %v", err)
	}

	const n = 16
	var (
		wg         sync.WaitGroup
		successCnt int32
		conflictCnt int32
	)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(worker int) {
			defer wg.Done()
			_, _, err := svc.Lease(LeaseInput{
				Platform: "xhs",
				WorkerID: fmt.Sprintf("w-%d", worker),
				TTLSec:   900,
			})
			if err == nil {
				atomic.AddInt32(&successCnt, 1)
				return
			}
			if errors.Is(err, ErrNoAvailableAccount) {
				atomic.AddInt32(&conflictCnt, 1)
				return
			}
			t.Errorf("unexpected lease error: %v", err)
		}(i)
	}
	wg.Wait()

	if successCnt != 1 {
		t.Fatalf("期望恰好 1 个租约成功，实际 %d（冲突 %d）", successCnt, conflictCnt)
	}
	if conflictCnt != n-1 {
		t.Fatalf("期望 %d 个冲突，实际 %d", n-1, conflictCnt)
	}

	// 数据库中该账号恰好一条 held 租约
	held, err := svc.leaseDAO.ListHeld(svc.db)
	if err != nil {
		t.Fatalf("ListHeld 失败: %v", err)
	}
	if len(held) != 1 || held[0].AccountID != "acc-1" {
		t.Fatalf("held 租约数量/归属异常: %+v", held)
	}
}

func TestLease_ExcludesRunningAccount(t *testing.T) {
	svc, _, _ := newTestLeaseDAO(t)
	_ = svc.dao.Upsert(&Account{
		AccountID: "acc-run", AccountName: "R", Platform: "x",
		BoundProfileID: "prof-run", Status: AccountStatusActive,
		Tags: []string{}, Credential: map[string]any{}, Metadata: map[string]any{},
	})

	// 注入一个永远报告“运行中”的探针
	svc.SetRuntimeProbe(runningProbe{running: map[string]bool{"prof-run": true}})

	_, _, err := svc.Lease(LeaseInput{Platform: "x", TTLSec: 60})
	if !errors.Is(err, ErrNoAvailableAccount) {
		t.Fatalf("运行中账号应被排除，期望 ErrNoAvailableAccount，got: %v", err)
	}
}

func TestRelease_StatusTransitions(t *testing.T) {
	svc, _, _ := newTestLeaseDAO(t)

	cases := []struct {
		result      string
		cooldown    int
		wantStatus  string
		wantCooldownNonEmpty bool
	}{
		{ReleaseResultOK, 0, AccountStatusActive, false},
		{ReleaseResultRisk, 120, AccountStatusCooldown, true},
		{ReleaseResultBan, 0, AccountStatusBanned, false},
		{ReleaseResultNeedLogin, 0, AccountStatusNeedLogin, false},
	}

	for i, c := range cases {
		// 每个用例使用独立账号，避免前一次释放改变状态影响后续租约
		accID := fmt.Sprintf("acc-t-%d", i)
		profID := fmt.Sprintf("prof-t-%d", i)
		_ = svc.dao.Upsert(&Account{
			AccountID: accID, AccountName: "T", Platform: "x",
			BoundProfileID: profID, Status: AccountStatusActive,
			Tags: []string{}, Credential: map[string]any{}, Metadata: map[string]any{},
		})

		_, lease, err := svc.Lease(LeaseInput{Platform: "x", WorkerID: fmt.Sprintf("w-%d", i), TTLSec: 900})
		if err != nil {
			t.Fatalf("case %d Lease 失败: %v", i, err)
		}
		_, account, err := svc.Release(lease.LeaseID, c.result, c.cooldown)
		if err != nil {
			t.Fatalf("case %d Release 失败: %v", i, err)
		}
		if account == nil {
			t.Fatalf("case %d 返回 nil account", i)
		}
		if account.Status != c.wantStatus {
			t.Fatalf("case %d 状态期望 %s，实际 %s", i, c.wantStatus, account.Status)
		}
		empty := strings.TrimSpace(account.CooldownUntil) == ""
		if c.wantCooldownNonEmpty && empty {
			t.Fatalf("case %d 期望 cooldown_until 非空", i)
		}
		if !c.wantCooldownNonEmpty && !empty {
			t.Fatalf("case %d 期望 cooldown_until 为空，实际 %s", i, account.CooldownUntil)
		}
		// ok 释放后账号恢复 active，可再次被租用
		if c.result == ReleaseResultOK {
			if _, _, err := svc.Lease(LeaseInput{Platform: "x", WorkerID: fmt.Sprintf("w-re-%d", i), TTLSec: 60}); err != nil {
				t.Fatalf("case %d ok 释放后应可再次租用，got: %v", i, err)
			}
		}
	}
}

func TestHeartbeat_ExtendsExpiry(t *testing.T) {
	svc, _, _ := newTestLeaseDAO(t)
	_ = svc.dao.Upsert(&Account{
		AccountID: "acc-h", AccountName: "H", Platform: "x",
		BoundProfileID: "prof-h", Status: AccountStatusActive,
		Tags: []string{}, Credential: map[string]any{}, Metadata: map[string]any{},
	})

	_, lease, err := svc.Lease(LeaseInput{Platform: "x", TTLSec: 60})
	if err != nil {
		t.Fatalf("Lease 失败: %v", err)
	}
	original := lease.ExpiresAt

	time.Sleep(10 * time.Millisecond)
	updated, err := svc.Heartbeat(lease.LeaseID, 1800)
	if err != nil {
		t.Fatalf("Heartbeat 失败: %v", err)
	}
	if updated.ExpiresAt <= original {
		t.Fatalf("Heartbeat 应延长过期时间: 原始 %s 更新 %s", original, updated.ExpiresAt)
	}

	// 非已释放的租约不可再续
	if _, _, err := svc.Release(lease.LeaseID, ReleaseResultOK, 0); err != nil {
		t.Fatalf("Release 失败: %v", err)
	}
	if _, err := svc.Heartbeat(lease.LeaseID, 60); !errors.Is(err, ErrLeaseNotHeld) {
		t.Fatalf("已释放租约续约应返回 ErrLeaseNotHeld，got: %v", err)
	}
}

func TestReclaimExpired_ReleasesAccount(t *testing.T) {
	svc, _, _ := newTestLeaseDAO(t)
	_ = svc.dao.Upsert(&Account{
		AccountID: "acc-e", AccountName: "E", Platform: "x",
		BoundProfileID: "prof-e", Status: AccountStatusActive,
		Tags: []string{}, Credential: map[string]any{}, Metadata: map[string]any{},
	})

	// 直接预置一条 expires_at 已过期的 held 租约，避免与 RFC3339 秒级截断竞态
	past := time.Now().UTC().Add(-10 * time.Second).Format(time.RFC3339)
	lease := &Lease{
		LeaseID: "lease-e", AccountID: "acc-e", ProfileID: "prof-e",
		Status: LeaseStatusHeld, ExpiresAt: past,
		LeasedAt: time.Now().Add(-20 * time.Second).Format(time.RFC3339),
		AutoStarted: 1, Metadata: map[string]any{},
	}
	if err := svc.leaseDAO.UpsertLease(svc.db, lease); err != nil {
		t.Fatalf("预置租约失败: %v", err)
	}

	reclaimed, err := svc.ReclaimExpired()
	if err != nil {
		t.Fatalf("ReclaimExpired 失败: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].LeaseID != lease.LeaseID {
		t.Fatalf("应回收 1 条过期租约，got: %+v", reclaimed)
	}

	// 账号应恢复 active
	account, err := svc.dao.GetByID("acc-e")
	if err != nil {
		t.Fatalf("GetByID 失败: %v", err)
	}
	if account.Status != AccountStatusActive {
		t.Fatalf("过期回收后账号应恢复 active，实际 %s", account.Status)
	}

	// 该租约状态为 expired，不再 held
	got, _ := svc.leaseDAO.GetLeaseByID(svc.db, lease.LeaseID)
	if got.Status != LeaseStatusExpired {
		t.Fatalf("过期租约状态应为 expired，实际 %s", got.Status)
	}
}

// runningProbe 测试用 RuntimeProbe
type runningProbe struct {
	running map[string]bool
}

func (p runningProbe) IsRunning(profileId string) bool {
	return p.running[profileId]
}

func TestLease_RetriesToNextCandidateOnConflict(t *testing.T) {
	svc, _, _ := newTestLeaseDAO(t)
	_ = svc.dao.Upsert(&Account{
		AccountID: "acc-a", AccountName: "A", Platform: "x",
		BoundProfileID: "prof-a", Status: AccountStatusActive,
		Tags: []string{}, Credential: map[string]any{}, Metadata: map[string]any{},
	})
	_ = svc.dao.Upsert(&Account{
		AccountID: "acc-b", AccountName: "B", Platform: "x",
		BoundProfileID: "prof-b", Status: AccountStatusActive,
		Tags: []string{}, Credential: map[string]any{}, Metadata: map[string]any{},
	})

	// 预先在 acc-a 上建一条 held 租约，使其被唯一索引锁定
	pre := &Lease{
		LeaseID: "pre-1", AccountID: "acc-a", ProfileID: "prof-a",
		Status: LeaseStatusHeld, ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339),
		LeasedAt: time.Now().Format(time.RFC3339), Metadata: map[string]any{},
	}
	if err := svc.leaseDAO.UpsertLease(svc.db, pre); err != nil {
		t.Fatalf("预置 held 租约失败: %v", err)
	}

	// acc-a 已被占用（active 且无 running），acc-b 可用；Lease 应在同一事务内重试到 acc-b
	account, lease, err := svc.Lease(LeaseInput{Platform: "x", TTLSec: 60})
	if err != nil {
		t.Fatalf("应回退到 acc-b 成功，got: %v", err)
	}
	if account.AccountID != "acc-b" {
		t.Fatalf("期望租用 acc-b，实际 %s", account.AccountID)
	}
	if lease.AccountID != "acc-b" {
		t.Fatalf("租约账号应为 acc-b，实际 %s", lease.AccountID)
	}

	held, _ := svc.leaseDAO.ListHeld(svc.db)
	if len(held) != 2 {
		t.Fatalf("应有两 held 租约（pre + new），实际 %d", len(held))
	}
}