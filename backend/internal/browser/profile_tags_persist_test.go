package browser

import (
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"path/filepath"
	"testing"
)

// newManagerWithSQLite builds a Manager backed by a real SQLite DAO on the given DB file,
// mirroring how the production app wires Manager.ProfileDAO / TagRegistry.
func newManagerWithSQLite(t *testing.T, dbFile string) *Manager {
	t.Helper()
	db, err := database.NewDB(dbFile)
	if err != nil {
		t.Fatalf("NewDB returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}
	m := NewManager(&config.Config{}, t.TempDir())
	m.ProfileDAO = NewSQLiteProfileDAO(db.GetConn())
	m.TagRegistry = NewSQLiteTagRegistryDAO(db.GetConn())
	return m
}

// TestUpdatePersistsTagsAcrossReload reproduces the user's exact path:
// tag an instance in the editor (Manager.Update) -> save -> reopen the tag
// management page from a fresh process (new Manager reading the same DB) ->
// List() must still carry the tag. This rules out the hypothesis that the
// editor save path drops / fails to persist tags.
func TestUpdatePersistsTagsAcrossReload(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "profiles.db")

	// 1) editor session: create then tag the instance with "opencode"
	editor := newManagerWithSQLite(t, dbFile)
	created, err := editor.Create(ProfileInput{ProfileName: "opencode-instance"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := editor.Update(created.ProfileId, ProfileInput{
		ProfileName: "opencode-instance",
		Tags:        []string{"opencode"},
	}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	// 2) reopen: brand-new Manager (m.Profiles empty) loading from the same DB,
	// exactly how BrowserProfileList -> Manager.List -> InitData/loadProfiles reads back.
	reader := newManagerWithSQLite(t, dbFile)
	listed := reader.List()

	var got *Profile
	for i := range listed {
		if listed[i].ProfileId == created.ProfileId {
			got = &listed[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("profile %s not found after reload", created.ProfileId)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "opencode" {
		t.Fatalf("tags after reload = %#v, want [\"opencode\"]", got.Tags)
	}
}

// TestDAO_UpsertList_RoundTripsTags proves the SQL Upsert + scanProfile round-trip
// at the DAO layer (the existing DAO test only persisted empty tags).
func TestDAO_UpsertList_RoundTripsTags(t *testing.T) {
	editor := newManagerWithSQLite(t, filepath.Join(t.TempDir(), "profiles.db"))
	dao := editor.ProfileDAO
	p := &Profile{
		ProfileId:       "p-tags",
		ProfileName:     "tags",
		Tags:            []string{"opencode", "work"},
		FingerprintArgs: []string{},
		LaunchArgs:      []string{},
		Keywords:        []string{},
		CreatedAt:       "2026-08-02T00:00:00Z",
		UpdatedAt:       "2026-08-02T00:00:00Z",
	}
	if err := dao.Upsert(p); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	// upsert again with different tags to exercise ON CONFLICT(tags) update
	p.Tags = []string{"opencode"}
	if err := dao.Upsert(p); err != nil {
		t.Fatalf("second Upsert returned error: %v", err)
	}
	stored, err := dao.GetById(p.ProfileId)
	if err != nil {
		t.Fatalf("GetById returned error: %v", err)
	}
	if len(stored.Tags) != 1 || stored.Tags[0] != "opencode" {
		t.Fatalf("stored tags = %#v, want [\"opencode\"]", stored.Tags)
	}
}
