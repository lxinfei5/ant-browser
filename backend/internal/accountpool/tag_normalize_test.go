package accountpool

import (
	"reflect"
	"testing"
)

// TestNormalizeTags 账号标签归一:trim + 小写 + 去重(统一走 tagutil)。
func TestNormalizeTags(t *testing.T) {
	got := normalizeTags([]string{" OpenCode ", "opencode", "WORK", "", "work"})
	want := []string{"opencode", "work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTags = %v, want %v", got, want)
	}
}

// TestSQLiteAccountDAO_UpsertNormalizesTags 写库前 normalizeTags 归一,读回为小写去重。
func TestSQLiteAccountDAO_UpsertNormalizesTags(t *testing.T) {
	dao := newTestDAO(t)
	acc := &Account{
		AccountID:   "a1",
		AccountName: "x",
		Tags:        []string{" OpenCode ", "opencode", "VIP"},
		Credential:  map[string]any{},
		Metadata:    map[string]any{},
	}
	if err := dao.Upsert(acc); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := dao.GetByID("a1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if want := []string{"opencode", "vip"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("tags = %v, want %v", got.Tags, want)
	}
}

// TestAccountPoolService_RemoveTagFromAll 从全部账号移除标签(大小写不敏感):
// 命中(含大小写变体)的被剥掉,未命中的不受影响,返回受影响数并持久化。
func TestAccountPoolService_RemoveTagFromAll(t *testing.T) {
	dao := newTestDAO(t)
	svc := NewAccountPoolService(dao)

	seed := []struct {
		id   string
		tags []string
	}{
		{"a1", []string{"opencode", "work"}}, // 命中
		{"a2", []string{"OpenCode"}},          // 大小写变体,也应命中(Upsert 归一为 opencode)
		{"a3", []string{"vip"}},               // 不命中
	}
	for _, s := range seed {
		if err := dao.Upsert(&Account{AccountID: s.id, AccountName: s.id, Tags: s.tags, Credential: map[string]any{}, Metadata: map[string]any{}}); err != nil {
			t.Fatalf("seed Upsert %s: %v", s.id, err)
		}
	}

	affected, err := svc.RemoveTagFromAll(" OpenCode ")
	if err != nil {
		t.Fatalf("RemoveTagFromAll: %v", err)
	}
	if affected != 2 {
		t.Fatalf("affected = %d, want 2", affected)
	}

	// a1 只剩 work;a2 空;a3 不变
	assertAccountTags(t, dao, "a1", []string{"work"})
	assertAccountTags(t, dao, "a2", []string{})
	assertAccountTags(t, dao, "a3", []string{"vip"})
}

func assertAccountTags(t *testing.T, dao *SQLiteAccountDAO, id string, want []string) {
	t.Helper()
	got, err := dao.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID %s: %v", id, err)
	}
	if len(want) == 0 {
		if len(got.Tags) != 0 {
			t.Fatalf("account %s tags = %v, want empty", id, got.Tags)
		}
		return
	}
	if !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("account %s tags = %v, want %v", id, got.Tags, want)
	}
}
