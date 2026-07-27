package browser

import (
	"ant-chrome/backend/internal/apppath"
	"ant-chrome/backend/internal/config"
	"os/exec"
	"sync"
)

// Profile 浏览器配置文件
type Profile struct {
	ProfileId          string   `json:"profileId"`
	ProfileName        string   `json:"profileName"`
	UserDataDir        string   `json:"userDataDir"`
	CoreId             string   `json:"coreId"`
	FingerprintArgs    []string `json:"fingerprintArgs"`
	ProxyId            string   `json:"proxyId"`
	ProxyConfig        string   `json:"proxyConfig"`
	ProxyBindSourceID  string   `json:"proxyBindSourceId"`
	ProxyBindSourceURL string   `json:"proxyBindSourceUrl"`
	ProxyBindName      string   `json:"proxyBindName"`
	ProxyBindUpdatedAt string   `json:"proxyBindUpdatedAt"`
	LaunchArgs         []string `json:"launchArgs"`
	Tags               []string `json:"tags"`
	Keywords           []string `json:"keywords"`
	GroupId            string   `json:"groupId"` // 所属分组ID
	LaunchCode         string   `json:"launchCode"`
	Running            bool     `json:"running"`
	DebugPort          int      `json:"debugPort"`
	DebugReady         bool     `json:"debugReady"`
	Pid                int      `json:"pid"`
	RuntimeWarning     string   `json:"runtimeWarning"`
	LastError          string   `json:"lastError"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
	DeletedAt          string   `json:"deletedAt"`
	LastStartAt        string   `json:"lastStartAt"`
	LastStopAt         string   `json:"lastStopAt"`
}

// ProfileInput 创建/更新配置文件的输入
type ProfileInput struct {
	ProfileName     string   `json:"profileName"`
	UserDataDir     string   `json:"userDataDir"`
	CoreId          string   `json:"coreId"`
	FingerprintArgs []string `json:"fingerprintArgs"`
	ProxyId         string   `json:"proxyId"`
	ProxyConfig     string   `json:"proxyConfig"`
	LaunchArgs      []string `json:"launchArgs"`
	Tags            []string `json:"tags"`
	Keywords        []string `json:"keywords"`
	GroupId         string   `json:"groupId"` // 所属分组ID
}

// ProfileCopyOptions 复制实例时的附加选项。
type ProfileCopyOptions struct {
	Mode              string   `json:"mode"`
	AutomationTargets []string `json:"automationTargets"`
}

// Tab 浏览器标签页
type Tab struct {
	TabId  string `json:"tabId"`
	Title  string `json:"title"`
	Url    string `json:"url"`
	Active bool   `json:"active"`
}

// Settings 浏览器全局设置
type Settings struct {
	UserDataRoot           string   `json:"userDataRoot"`
	DefaultFingerprintArgs []string `json:"defaultFingerprintArgs"`
	DefaultLaunchArgs      []string `json:"defaultLaunchArgs"`
	DefaultStartURLs       []string `json:"defaultStartUrls"`
	LightStartEnabled      bool     `json:"lightStartEnabled"`
	RestoreLastSession     bool     `json:"restoreLastSession"`
	StartReadyTimeoutMs    int      `json:"startReadyTimeoutMs"`
	StartStableWindowMs    int      `json:"startStableWindowMs"`
	DefaultConnectorType   string   `json:"defaultConnectorType"`
}

// CoreInput 内核配置输入
type CoreInput struct {
	CoreId    string `json:"coreId"`
	CoreName  string `json:"coreName"`
	CorePath  string `json:"corePath"`
	IsDefault bool   `json:"isDefault"`
}

// CoreValidateResult 内核路径验证结果
type CoreValidateResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// CoreExtendedInfo 内核扩展信息
type CoreExtendedInfo struct {
	CoreId        string `json:"coreId"`
	ChromeVersion string `json:"chromeVersion"`
	InstanceCount int    `json:"instanceCount"`
}

// Group 实例分组
type Group struct {
	GroupId   string `json:"groupId"`
	GroupName string `json:"groupName"`
	ParentId  string `json:"parentId"` // 空字符串表示根级分组
	SortOrder int    `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// GroupInput 创建/更新分组的输入
type GroupInput struct {
	GroupName string `json:"groupName"`
	ParentId  string `json:"parentId"`
	SortOrder int    `json:"sortOrder"`
}

// GroupWithCount 带实例计数的分组
type GroupWithCount struct {
	Group
	InstanceCount int `json:"instanceCount"`
}

// 类型别名
type Proxy = config.BrowserProxy
type Core = config.BrowserCore
type Environment = config.BrowserEnvironment
type ProfileConfig = config.BrowserProfileConfig

// CodeProvider 提供 LaunchCode 的接口（由 launchcode.LaunchCodeService 实现）
type CodeProvider interface {
	EnsureCode(profileId string) (string, error)
	Remove(profileId string) error
}

// DockIconAccount 携带账号的 Dock 图标切片。
// 定义在 browser 包内（而非 dockicon/accountpool），使 browser 无需 import 二者，避免循环依赖。
type DockIconAccount struct {
	Found       bool
	IconKind    string // "" | color | text | image（空=不定制）
	IconColor   string
	IconText    string
	DisplayName string
}

// DockIconLookup 由 App 层注入：根据 profileId 查其绑定账号的图标设置与显示名。
type DockIconLookup func(profileId string) (DockIconAccount, bool)

// DockIconResolver 由 dockicon 包实现：把某 profile 的启动 exe 换成定制图标的克隆 bundle 内 exe。
type DockIconResolver interface {
	// Materialize 返回定制图标克隆 bundle 内的可执行文件路径；sourceExe 为原始内核 exe。
	// 任何失败都应回退返回 sourceExe（由实现保证），调用方不因图标失败而阻断启动。
	Materialize(profileId, sourceExe, pngPath, displayName string) (string, error)
	// Invalidate 仅失效缓存（下次启动重建），不删除主图。
	Invalidate(profileId string)
	// Remove 删除该 profile 的克隆与主图。
	Remove(profileId string)
	// Sweep 清理不在 validProfileIds 内的孤儿克隆/主图。
	Sweep(validProfileIds []string)
	// RebuildAll 失效全部缓存，下次启动惰性重建。
	RebuildAll()
	// SaveMasterPNG 持久化某 profile 的主图 PNG，返回其路径。
	SaveMasterPNG(profileId string, png []byte) (string, error)
	// MasterPNGPath 返回某 profile 已持久化主图 PNG 的路径；无则返回空串。
	MasterPNGPath(profileId string) string
}

// Manager 浏览器管理器
type Manager struct {
	Config           *config.Config
	AppRoot          string // 应用根目录，所有相对路径基于此解析（生产=exe目录，dev=项目根目录）
	Profiles         map[string]*Profile
	Mutex            sync.Mutex
	BrowserProcesses map[string]*exec.Cmd
	XrayBridges      map[string]*XrayBridge
	CodeProvider     CodeProvider

	// Dock 图标（可选注入；为空则不定制，直接用原内核 exe）
	DockIconResolver DockIconResolver
	DockIconLookup   DockIconLookup

	// DAO 层（注入后使用 SQLite 存储，未注入时降级到 config.yaml）
	ProfileDAO   ProfileDAO
	ProxyDAO     ProxyDAO
	CoreDAO      CoreDAO
	BookmarkDAO  BookmarkDAO
	GroupDAO     GroupDAO
	ExtensionDAO ExtensionDAO
}

// XrayBridge Xray 桥接进程
type XrayBridge struct {
	NodeKey   string
	Port      int
	Cmd       *exec.Cmd
	Pid       int
	Running   bool
	LastError string
}

// NewManager 创建浏览器管理器
func NewManager(cfg *config.Config, appRoot string) *Manager {
	return &Manager{
		Config:           cfg,
		AppRoot:          appRoot,
		Profiles:         make(map[string]*Profile),
		BrowserProcesses: make(map[string]*exec.Cmd),
		XrayBridges:      make(map[string]*XrayBridge),
	}
}

// ResolveRelativePath 将相对路径解析为绝对路径（基于 AppRoot）。
// 如果传入的已经是绝对路径则直接返回。
func (m *Manager) ResolveRelativePath(p string) string {
	return apppath.Resolve(m.AppRoot, p)
}
