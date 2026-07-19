package accountpool

import (
	"sync"
	"time"
)

// LeaseStopFunc 停止绑定实例的回调（由 App 提供，避免 accountpool 依赖 browser/launchcode）。
type LeaseStopFunc func(profileId string)

// LeaseReclaimScheduler 租约过期回收定时器（参照 browser.ProxySpeedScheduler）。
//
// 周期性扫描过期的 held 租约：标记为 expired、将账号恢复为 active，
// 并对 auto_started=1 的租约停止其绑定实例。使用 stopCh 实现优雅关闭。
type LeaseReclaimScheduler struct {
	service  *AccountPoolService
	stopFn   LeaseStopFunc
	interval time.Duration
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewLeaseReclaimScheduler 创建回收定时器。interval<=0 时默认 30s。
func NewLeaseReclaimScheduler(service *AccountPoolService, stopFn LeaseStopFunc, interval time.Duration) *LeaseReclaimScheduler {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &LeaseReclaimScheduler{
		service:  service,
		stopFn:   stopFn,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动定时任务（非阻塞）
func (s *LeaseReclaimScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	go s.loop()
}

// Stop 停止定时任务
func (s *LeaseReclaimScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
}

// RunOnce 立即执行一轮回收（可手动触发 / 测试使用）
func (s *LeaseReclaimScheduler) RunOnce() {
	go s.reclaim()
}

func (s *LeaseReclaimScheduler) loop() {
	// 启动后延迟 10s 跑第一轮，避免与启动流程竞争
	select {
	case <-time.After(10 * time.Second):
	case <-s.stopCh:
		return
	}
	s.reclaim()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.reclaim()
		case <-s.stopCh:
			return
		}
	}
}

// reclaim 执行一轮回收：调用 service.ReclaimExpired()，停止 auto_started 的实例。
func (s *LeaseReclaimScheduler) reclaim() {
	if s.service == nil {
		return
	}
	reclaimed, err := s.service.ReclaimExpired()
	if err != nil || len(reclaimed) == 0 {
		return
	}
	if s.stopFn == nil {
		return
	}
	for _, lease := range reclaimed {
		if lease.AutoStarted == 1 && lease.ProfileID != "" {
			s.stopFn(lease.ProfileID)
		}
	}
}