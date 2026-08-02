package browser

import (
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"path/filepath"
	"reflect"
	"testing"
)

func newTestTagRegistry(t *testing.T) TagRegistryDAO {
	t.Helper()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "tags.db"))
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	return NewSQLiteTagRegistryDAO(db.GetConn())
}

func TestSQLiteTagRegistryDAO_EnsureListDelete(t *testing.T) {
	dao := newTestTagRegistry(t)

	if err := dao.Ensure("opencode"); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if err := dao.Ensure("grok"); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	// 幂等：重复注册不报错也不产生重复行
	if err := dao.Ensure("opencode"); err != nil {
		t.Fatalf("Ensure(dup) returned error: %v", err)
	}

	tags, err := dao.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if want := []string{"grok", "opencode"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("List = %v, want %v", tags, want)
	}

	if err := dao.Delete("opencode"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	tags, err = dao.List()
	if err != nil {
		t.Fatalf("List after delete returned error: %v", err)
	}
	if want := []string{"grok"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("List after delete = %v, want %v", tags, want)
	}
}

func TestManager_ListTags_MergesProfileAndRegistry(t *testing.T) {
	dao := newTestTagRegistry(t)
	m := &Manager{
		Config:   &config.Config{},
		Profiles: map[string]*Profile{
			"p1": {Tags: []string{"profile-tag", "shared"}},
		},
		TagRegistry: dao,
	}

	if err := dao.Ensure("registry-only"); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if err := dao.Ensure("shared"); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}

	// ListTags 只返回注册表（不含实例标签）
	registryOnly, err := dao.List()
	if err != nil {
		t.Fatalf("dao.List returned error: %v", err)
	}
	if want := []string{"registry-only", "shared"}; !reflect.DeepEqual(registryOnly, want) {
		t.Fatalf("ListTags = %v, want %v", registryOnly, want)
	}

	// GetAllTags 合并实例标签 + 注册表，去重排序
	all := m.GetAllTags()
	want := []string{"profile-tag", "registry-only", "shared"}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("GetAllTags = %v, want %v", all, want)
	}
}

func TestManager_GetAllTags_RegistryUnavailable(t *testing.T) {
	m := &Manager{
		Config: &config.Config{},
		Profiles: map[string]*Profile{
			"p1": {Tags: []string{"a", "b"}},
		},
		// TagRegistry 未注入：仅返回实例标签
	}
	all := m.GetAllTags()
	if want := []string{"a", "b"}; !reflect.DeepEqual(all, want) {
		t.Fatalf("GetAllTags = %v, want %v", all, want)
	}
	if got := m.ListTags(); len(got) != 0 {
		t.Fatalf("ListTags = %v, want empty", got)
	}
}
