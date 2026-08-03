package browser

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestNormalizeTags 浏览器包归一门面,委托 tagutil。
func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" OpenCode ", "opencode", "WORK", "", "work"})
	want := []string{"opencode", "work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeTags = %v, want %v", got, want)
	}
}

// TestCreateNormalizesAndRegistersTags 实例创建时标签被归一为小写,并同步进注册表。
func TestCreateNormalizesAndRegistersTags(t *testing.T) {
	m := newManagerWithSQLite(t, filepath.Join(t.TempDir(), "profiles.db"))
	created, err := m.Create(ProfileInput{ProfileName: "inst", Tags: []string{" OpenCode ", "WORK"}})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if want := []string{"opencode", "work"}; !reflect.DeepEqual(created.Tags, want) {
		t.Fatalf("created tags = %v, want %v", created.Tags, want)
	}
	// 注册表应包含归一后的标签
	registryTags, err := m.TagRegistry.List()
	if err != nil {
		t.Fatalf("TagRegistry.List returned error: %v", err)
	}
	for _, want := range []string{"opencode", "work"} {
		if !containsString(registryTags, want) {
			t.Fatalf("registry tags = %v, missing %q", registryTags, want)
		}
	}
}

// TestUpdateNormalizesAndRegistersTags 实例更新(编辑页保存)标签被归一并同步注册表。
func TestUpdateNormalizesAndRegistersTags(t *testing.T) {
	m := newManagerWithSQLite(t, filepath.Join(t.TempDir(), "profiles.db"))
	created, err := m.Create(ProfileInput{ProfileName: "inst"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	updated, err := m.Update(created.ProfileId, ProfileInput{ProfileName: "inst", Tags: []string{"VIP", " vip "}})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if want := []string{"vip"}; !reflect.DeepEqual(updated.Tags, want) {
		t.Fatalf("updated tags = %v, want %v", updated.Tags, want)
	}
	registryTags, err := m.TagRegistry.List()
	if err != nil {
		t.Fatalf("TagRegistry.List returned error: %v", err)
	}
	if !containsString(registryTags, "vip") {
		t.Fatalf("registry tags = %v, missing vip", registryTags)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
