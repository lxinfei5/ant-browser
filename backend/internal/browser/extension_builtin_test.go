package browser

import (
	"ant-chrome/backend/internal/browser/builtins/forcefont"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newExtensionTestManager(t *testing.T) (*Manager, *SQLiteExtensionDAO) {
	t.Helper()
	root := t.TempDir()
	db, err := database.NewDB(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	dao := NewSQLiteExtensionDAO(db.GetConn())
	mgr := NewManager(&config.Config{}, root)
	mgr.ExtensionDAO = dao
	return mgr, dao
}

func TestForceFontExtensionIDMatchesManifestKey(t *testing.T) {
	der, err := base64.StdEncoding.DecodeString(forcefont.ManifestKey)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}
	sum := sha256.Sum256(der)
	var id strings.Builder
	for _, byt := range sum[:16] {
		for _, nibble := range []byte{byt >> 4, byt & 0x0f} {
			if nibble < 10 {
				id.WriteByte(byte('a' + nibble))
			} else {
				id.WriteByte(byte('k' + nibble - 10))
			}
		}
	}
	if id.String() != forcefont.ExtensionID {
		t.Fatalf("extension id = %s, want %s", id.String(), forcefont.ExtensionID)
	}
}

func TestEnsureBuiltinExtensionsInstallsAndRepairs(t *testing.T) {
	mgr, dao := newExtensionTestManager(t)

	if err := mgr.EnsureBuiltinExtensions(); err != nil {
		t.Fatalf("EnsureBuiltinExtensions: %v", err)
	}
	stored, err := dao.Get(forcefont.ExtensionID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !stored.Enabled || !stored.Builtin {
		t.Fatalf("expected enabled builtin, got enabled=%v builtin=%v", stored.Enabled, stored.Builtin)
	}
	if stored.SourceURL != forcefont.SourceURL {
		t.Fatalf("source = %q", stored.SourceURL)
	}
	contentPath := filepath.Join(stored.InstallDir, "content.js")
	if _, err := os.Stat(contentPath); err != nil {
		t.Fatalf("content.js missing: %v", err)
	}

	if err := dao.SetEnabled(forcefont.ExtensionID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if err := os.WriteFile(contentPath, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stored.InstallDir, "evil.js"), []byte("alert(1)"), 0o644); err != nil {
		t.Fatalf("extra file: %v", err)
	}

	if err := mgr.EnsureBuiltinExtensions(); err != nil {
		t.Fatalf("Ensure after tamper: %v", err)
	}
	restored, err := dao.Get(forcefont.ExtensionID)
	if err != nil {
		t.Fatalf("Get after repair: %v", err)
	}
	if restored.Enabled {
		t.Fatal("expected disabled flag to be preserved")
	}
	data, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("read repaired content: %v", err)
	}
	if string(data) == "tampered" || !strings.Contains(string(data), "YAHEI_SIZE_ADJUST") {
		t.Fatalf("content.js was not restored from embed")
	}
	if _, err := os.Stat(filepath.Join(stored.InstallDir, "evil.js")); !os.IsNotExist(err) {
		t.Fatal("expected extra file to be removed during repair")
	}
}

func TestEnsureBuiltinSeedsConfiguredProfilesOnce(t *testing.T) {
	mgr, dao := newExtensionTestManager(t)
	if _, err := dao.SetProfileSettings("p1", []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, true); err != nil {
		t.Fatalf("SetProfileSettings: %v", err)
	}
	if err := mgr.EnsureBuiltinExtensions(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	settings, err := dao.GetProfileSettings("p1")
	if err != nil {
		t.Fatalf("GetProfileSettings: %v", err)
	}
	found := false
	for _, id := range settings.ExtensionIDs {
		if id == forcefont.ExtensionID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected builtin id in configured profile, got %v", settings.ExtensionIDs)
	}

	if _, err := dao.SetProfileSettings("p1", []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, true); err != nil {
		t.Fatalf("uncheck builtin: %v", err)
	}
	if err := mgr.EnsureBuiltinExtensions(); err != nil {
		t.Fatalf("Ensure second: %v", err)
	}
	settings, err = dao.GetProfileSettings("p1")
	if err != nil {
		t.Fatalf("GetProfileSettings second: %v", err)
	}
	for _, id := range settings.ExtensionIDs {
		if id == forcefont.ExtensionID {
			t.Fatal("second ensure re-added an explicitly unchecked builtin")
		}
	}
}

func TestEnabledExtensionDirsSkipsDisabledBuiltin(t *testing.T) {
	mgr, dao := newExtensionTestManager(t)
	if err := mgr.EnsureBuiltinExtensions(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	dirs := mgr.EnabledExtensionDirs()
	if len(dirs) != 1 {
		t.Fatalf("enabled dirs = %d, want 1", len(dirs))
	}
	if err := dao.SetEnabled(forcefont.ExtensionID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if dirs := mgr.EnabledExtensionDirs(); len(dirs) != 0 {
		t.Fatalf("disabled builtin still loaded: %v", dirs)
	}
}

func TestInstallAndDeleteRejectBuiltin(t *testing.T) {
	mgr, _ := newExtensionTestManager(t)
	if err := mgr.EnsureBuiltinExtensions(); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, err := mgr.InstallExtensionPackageBytes(forcefont.ExtensionID, "https://chromewebstore.google.com", []byte("PK\x03\x04")); err == nil {
		t.Fatal("expected store overwrite to fail")
	}
	dir := t.TempDir()
	manifest, err := forcefont.Files.ReadFile("files/manifest.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := mgr.InstallExtensionDirectory(dir); err == nil {
		t.Fatal("expected directory import of builtin to fail")
	}
}

func TestVerifyBuiltinFilesRejectsMismatch(t *testing.T) {
	files, err := readBuiltinForceFontFiles()
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":"0"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := verifyBuiltinFiles(dir, files); err == nil {
		t.Fatal("expected mismatch to fail closed")
	}
	if digest := builtinFilesDigest(files); digest == "" {
		t.Fatal("expected non-empty digest")
	}
}
