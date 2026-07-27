package backend

import (
	"ant-chrome/backend/internal/apppath"
	"ant-chrome/backend/internal/accountpool"
	"ant-chrome/backend/internal/automation"
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"ant-chrome/backend/internal/dockicon"
	"ant-chrome/backend/internal/launchcode"
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/proxy"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// startup 应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := apppath.EnsureWritableLayout(a.appRoot); err != nil {
		runtime.LogFatal(ctx, fmt.Sprintf("初始化 Linux 用户数据目录失败: %v", err))
		return
	}

	cfg := a.startupLoadConfig()
	a.config = cfg
	a.applyRuntimeConfig(cfg.Runtime)

	log := a.startupInitLogger(ctx, cfg)
	a.startupLogEnvironment(log, cfg)

	if err := os.MkdirAll(a.resolveAppPath("data"), 0o755); err != nil {
		log.Error("创建 data 目录失败", logger.F("error", err))
	}

	a.ensureDefaultCores()
	a.startupInitInterceptor(log, cfg)

	db, err := a.startupInitDatabase(cfg)
	if err != nil {
		log.Error("初始化数据库失败", logger.F("error", err))
		runtime.LogFatal(ctx, fmt.Sprintf("初始化数据库失败: %v", err))
		return
	}
	a.db = db
	if err := db.Migrate(); err != nil {
		log.Error("数据库迁移失败", logger.F("error", err))
	}

	a.startupInitManagers(cfg, db)
	a.startupInitLaunchCode(log)
	a.startupInitLaunchServer(log)
	a.startupInitDockIcon(log)
	a.startupInitBackup(log)
	a.startupInitAutomation()
	a.startupInitBridgeHooks()
	a.startupInitSpeedScheduler()
	a.startupInitLeaseReclaim(log)

	log.Info("应用启动成功")
}

func (a *App) startupLoadConfig() *config.Config {
	configPath := a.resolveAppPath("config.yaml")
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return config.DefaultConfig()
	}
	// 默认开启 API 认证：首次运行且未配置 key 时自动生成并持久化，避免每次启动轮换 key。
	if _, ensureErr := config.EnsureLaunchServerAuthKey(cfg, configPath); ensureErr != nil {
		logger.New("App").Error("自动生成 LaunchServer API key 失败", logger.F("error", ensureErr))
	}
	return cfg
}

func (a *App) startupInitLogger(ctx context.Context, cfg *config.Config) *logger.Logger {
	logConfig := logger.LoggerConfig{
		Level:           cfg.Logging.Level,
		FileEnabled:     cfg.Logging.FileEnabled,
		FilePath:        a.resolveAppPath(cfg.Logging.FilePath),
		Format:          cfg.Logging.Format,
		BufferSize:      cfg.Logging.BufferSize,
		AsyncQueueSize:  cfg.Logging.AsyncQueueSize,
		FlushIntervalMs: cfg.Logging.FlushIntervalMs,
		Rotation: logger.RotationConfig{
			Enabled:      cfg.Logging.Rotation.Enabled,
			MaxSizeMB:    cfg.Logging.Rotation.MaxSizeMB,
			MaxAge:       cfg.Logging.Rotation.MaxAge,
			MaxBackups:   cfg.Logging.Rotation.MaxBackups,
			TimeInterval: cfg.Logging.Rotation.TimeInterval,
		},
	}
	logger.InitWithConfig(ctx, logConfig)
	return logger.New("App")
}

func (a *App) startupLogEnvironment(log *logger.Logger, cfg *config.Config) {
	log.Info("应用启动中...",
		logger.F("version", a.appVersion()),
		logger.F("max_memory_mb", cfg.Runtime.MaxMemoryMB),
		logger.F("gc_percent", cfg.Runtime.GCPercent),
	)
	if apppath.IsDetached(a.appRoot) {
		log.Info("检测到安装目录需要只读运行，已切换到用户数据目录",
			logger.F("install_root", apppath.InstallRoot(a.appRoot)),
			logger.F("state_root", apppath.StateRoot(a.appRoot)),
		)
	}
}

func (a *App) startupInitInterceptor(log *logger.Logger, cfg *config.Config) {
	if !cfg.Logging.Interceptor.Enabled {
		return
	}
	interceptorConfig := logger.InterceptorConfig{
		Enabled:         cfg.Logging.Interceptor.Enabled,
		LogParameters:   cfg.Logging.Interceptor.LogParameters,
		LogResults:      cfg.Logging.Interceptor.LogResults,
		SensitiveFields: cfg.Logging.Interceptor.SensitiveFields,
	}
	a.interceptor = logger.NewMethodInterceptor(log, interceptorConfig)
}

func (a *App) startupInitDatabase(cfg *config.Config) (*database.DB, error) {
	return database.NewDB(a.resolveAppPath(cfg.Database.SQLite.Path))
}

func (a *App) startupInitManagers(cfg *config.Config, db *database.DB) {
	a.browserMgr = browser.NewManager(cfg, a.appRoot)
	a.xrayMgr = proxy.NewXrayManager(cfg, a.appRoot)
	a.clashMgr = proxy.NewClashManager(cfg, a.appRoot)
	a.singboxMgr = proxy.NewSingBoxManager(cfg, a.appRoot)

	conn := db.GetConn()
	a.browserMgr.ProfileDAO = browser.NewSQLiteProfileDAO(conn)
	a.browserMgr.ProxyDAO = browser.NewSQLiteProxyDAO(conn)
	a.browserMgr.CoreDAO = browser.NewSQLiteCoreDAO(conn)
	a.browserMgr.BookmarkDAO = browser.NewSQLiteBookmarkDAO(conn)
	a.browserMgr.GroupDAO = browser.NewSQLiteGroupDAO(conn)
	a.browserMgr.ExtensionDAO = browser.NewSQLiteExtensionDAO(conn)
	accountDAO := accountpool.NewSQLiteAccountDAO(conn)
	a.accountPool = accountpool.NewAccountPoolService(accountDAO)
	a.accountPool.SetLeaseDAO(accountpool.NewSQLiteLeaseDAO(conn))
	a.accountPool.SetDB(conn)
	a.accountPool.SetRuntimeProbe(a)
	a.accountPool.SetProfileFactory(a)
	a.accountPool.SetProxyResolver(a)

	a.migrateToSQLite()

	a.browserMgr.InitData()
	if err := a.browserMgr.CleanupExpiredTrash(); err != nil {
		logger.New("Browser").Error("启动清理回收站失败", logger.F("error", err))
	}
	a.autoDetectCores()
	a.loadProxies()
	a.reconcileProfileProxyBindings()
}

func (a *App) startupInitLaunchCode(log *logger.Logger) {
	launchCodeDAO := launchcode.NewSQLiteLaunchCodeDAO(a.db.GetConn())
	a.launchCodeSvc = launchcode.NewLaunchCodeService(launchCodeDAO)
	if err := a.launchCodeSvc.LoadAll(); err != nil {
		log.Error("LaunchCode 加载失败", logger.F("error", err))
	}
	a.browserMgr.CodeProvider = a.launchCodeSvc
}

// startupInitDockIcon 注入 Dock 图标解析器：为绑定了定制图标账号的 profile
// 生成/复用定制图标的 Chromium .app 克隆，并清理孤儿克隆缓存。
func (a *App) startupInitDockIcon(log *logger.Logger) {
	lookup := func(profileId string) (browser.DockIconAccount, bool) {
		if a.accountPool == nil {
			return browser.DockIconAccount{}, false
		}
		accounts, err := a.accountPool.List(accountpool.AccountFilter{})
		if err != nil {
			return browser.DockIconAccount{}, false
		}
		for _, acct := range accounts {
			if acct == nil || acct.BoundProfileID != profileId {
				continue
			}
			displayName := acct.AccountName
			if displayName == "" {
				displayName = acct.AccountRef
			}
			return browser.DockIconAccount{
				Found:       true,
				IconKind:    acct.IconKind,
				IconColor:   acct.IconColor,
				IconText:    acct.IconText,
				DisplayName: displayName,
			}, true
		}
		return browser.DockIconAccount{}, false
	}

	resolver := dockicon.NewResolver(apppath.StateRoot(a.appRoot), lookup)
	a.dockIconResolver = resolver
	a.browserMgr.DockIconResolver = resolver
	a.browserMgr.DockIconLookup = lookup

	// 清理孤儿克隆（profile 在应用关闭期间被删除的情况）。
	validIds := make([]string, 0)
	for _, p := range a.browserMgr.List() {
		validIds = append(validIds, p.ProfileId)
	}
	resolver.Sweep(validIds)
	log.Debug("Dock 图标解析器已注入", logger.F("valid_profiles", len(validIds)))
}

func (a *App) startupInitLaunchServer(log *logger.Logger) {
	port := a.config.LaunchServer.Port
	a.launchServer = launchcode.NewLaunchServer(a.launchCodeSvc, a, a.browserMgr, port)
	a.launchServer.SetAccountPoolService(a.accountPool)
	a.launchServer.SetAPIAuthConfig(launchcode.APIAuthConfig{
		Enabled: a.config.LaunchServer.Auth.IsEnabled(),
		APIKey:  a.config.LaunchServer.Auth.APIKey,
		Header:  a.config.LaunchServer.Auth.Header,
	})
	if err := a.launchServer.Start(); err != nil {
		log.Error("LaunchServer 启动失败", logger.F("error", err))
		return
	}
	log.Info("LaunchServer 监听地址",
		logger.F("url", fmt.Sprintf("http://127.0.0.1:%d", a.launchServer.Port())),
		logger.F("preferred_port", port),
	)
}

func (a *App) startupInitAutomation() {
	a.automationMgr = automation.NewManager(a.appRoot, a.config, func(event string, payload any) {
		if a.ctx == nil {
			return
		}
		runtime.EventsEmit(a.ctx, event, payload)
	}, automation.Options{})
}

func (a *App) startupInitBridgeHooks() {
	a.xrayMgr.OnBridgeDied = func(key string, err error) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "proxy:bridge:died", map[string]interface{}{
				"engine": "xray",
				"key":    key[:8],
				"error":  err.Error(),
			})
		}
	}
	a.singboxMgr.OnBridgeDied = func(key string, err error) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "proxy:bridge:died", map[string]interface{}{
				"engine": "singbox",
				"key":    key[:8],
				"error":  err.Error(),
			})
		}
	}
}

func (a *App) startupInitSpeedScheduler() {
	a.speedScheduler = browser.NewProxySpeedScheduler(
		a.browserMgr.ProxyDAO,
		func(proxyId string) (bool, int64, string) {
			connectorType := config.NormalizeBrowserConnectorType(a.config.Browser.DefaultConnectorType)
			r := a.testProxySpeedWithConnector(proxyId, a.getLatestProxies(), connectorType)
			return r.Ok, r.LatencyMs, r.Error
		},
		5*time.Minute,
		5,
	)
	a.speedScheduler.Start()
}

// startupInitLeaseReclaim 启动租约过期回收定时器（在 launchServer + accountPool 就绪之后）
func (a *App) startupInitLeaseReclaim(log *logger.Logger) {
	stopFn := func(profileId string) {
		_, _ = a.StopInstance(profileId)
	}
	a.leaseReclaim = accountpool.NewLeaseReclaimScheduler(a.accountPool, stopFn, 30*time.Second)
	a.leaseReclaim.Start()
	log.Info("租约过期回收定时器已启动", logger.F("interval", "30s"))
}

// startupInitBackup 启动定时备份（在 launchServer 就绪之后）。
// interval_minutes<=0（默认）时不启动，保持行为不变。
func (a *App) startupInitBackup(log *logger.Logger) {
	minutes := 0
	if a.config != nil {
		minutes = a.config.Backup.IntervalMinutes
	}
	if minutes <= 0 {
		log.Info("定时备份未启用（backup.interval_minutes<=0）")
		return
	}
	interval := time.Duration(minutes) * time.Minute
	a.backupScheduler = NewBackupScheduler(a.buildBackupExportFn(), interval)
	a.backupScheduler.Start()
	log.Info("定时备份已启动", logger.F("interval", fmt.Sprintf("%dm", minutes)))
}
