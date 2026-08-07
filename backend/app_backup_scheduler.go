package backend

import (
	"ant-chrome/backend/internal/backup"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// BackupScheduler 定时备份调度器（参照 browser.ProxySpeedScheduler）。
//
// 周期性将全量配置与数据导出为 ZIP 写入固定目录（非交互，不弹出保存对话框）。
// interval<=0 时不应创建/启动（由调用方根据配置决定）。使用 stopCh 实现优雅关闭。
type BackupScheduler struct {
	export   backupExportFn
	interval time.Duration
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// backupExportFn 执行一次非交互导出，写入固定路径。返回 zip 路径与错误。
type backupExportFn func() (string, error)

// NewBackupScheduler 创建定时备份调度器。interval<=0 时不启动（调用方应跳过 Start）。
func NewBackupScheduler(export backupExportFn, interval time.Duration) *BackupScheduler {
	return &BackupScheduler{
		export:   export,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动定时任务（非阻塞）。
func (s *BackupScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running || s.interval <= 0 {
		return
	}
	s.running = true
	go s.loop()
}

// Stop 停止定时任务。
func (s *BackupScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
}

// RunOnce 立即执行一次备份（手动触发 / 测试）。
func (s *BackupScheduler) RunOnce() {
	go s.runBackup()
}

func (s *BackupScheduler) loop() {
	// 启动后延迟 10s 跑第一轮，避免与启动流程竞争。
	select {
	case <-time.After(10 * time.Second):
	case <-s.stopCh:
		return
	}
	s.runBackup()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runBackup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *BackupScheduler) runBackup() {
	if s.export == nil {
		return
	}
	_, _ = s.export()
}

// buildBackupExportFn 构造一个非交互导出闭包：将全量配置与数据写入固定时间戳路径。
// 复用 backup.BuildScope + backup.BuildManifest + backupWritePackageZip（均不弹对话框）。
func (a *App) buildBackupExportFn() backupExportFn {
	return func() (string, error) {
		if a.config == nil {
			return "", fmt.Errorf("config is not available")
		}
		dir := filepath.Join(a.appRoot, "data", "backups")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("创建备份目录失败: %w", err)
		}
		zipPath := filepath.Join(dir, fmt.Sprintf("profilepool-backup-%s.zip", time.Now().Format("20060102-150405")))

		scope, err := backup.BuildScope(backup.BuildOptions{AppRoot: a.appRoot, Config: a.config})
		if err != nil {
			return "", err
		}
		manifest := backup.BuildManifest(scope, a.appName(), a.appVersion(), time.Now())
		if _, _, _, err := backupWritePackageZip(zipPath, scope, manifest, nil); err != nil {
			return "", err
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "backup:scheduled:result", map[string]interface{}{
				"ok":      true,
				"zipPath": zipPath,
			})
		}
		return zipPath, nil
	}
}