package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/logger"
	"ant-chrome/backend/internal/tagutil"
	"fmt"
	"strings"
	"time"
)

// BrowserProfileBatchSetTags 批量为实例设置标签（追加模式：将 tags 加入已有标签；replace 模式：直接替换）。
// 写入前统一归一(trim+小写+去重),并把新标签同步进注册表,保证实例标签与注册表不发散。
func (a *App) BrowserProfileBatchSetTags(profileIds []string, tags []string, replace bool) error {
	log := logger.New("Browser")
	a.browserMgr.InitData()
	tags = browser.NormalizeTags(tags)

	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()

	for _, profileID := range profileIds {
		profile, exists := a.browserMgr.Profiles[profileID]
		if !exists {
			continue
		}
		if replace {
			profile.Tags = append([]string{}, tags...)
		} else {
			existing := make(map[string]struct{})
			for _, tag := range profile.Tags {
				existing[tagutil.Normalize(tag)] = struct{}{}
			}
			for _, tag := range tags {
				key := tagutil.Normalize(tag)
				if _, ok := existing[key]; !ok {
					profile.Tags = append(profile.Tags, tag)
					existing[key] = struct{}{}
				}
			}
		}
		profile.UpdatedAt = time.Now().Format(time.RFC3339)
		if a.browserMgr.ProfileDAO != nil {
			if err := a.browserMgr.ProfileDAO.Upsert(profile); err != nil {
				log.Error("批量设置标签失败", logger.F("profile_id", profileID), logger.F("error", err))
				return err
			}
		}
	}
	// 同步注册表(同锁内,保证实例标签与注册表一致)
	for _, tag := range tags {
		if a.browserMgr.TagRegistry != nil {
			if err := a.browserMgr.TagRegistry.Ensure(tag); err != nil {
				log.Warn("同步标签到注册表失败", logger.F("tag", tag), logger.F("error", err))
			}
		}
	}
	return nil
}

// BrowserProfileBatchRemoveTags 批量从实例移除指定标签(大小写、空白不敏感,与读取侧口径一致)。
func (a *App) BrowserProfileBatchRemoveTags(profileIds []string, tags []string) error {
	log := logger.New("Browser")
	a.browserMgr.InitData()

	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()

	removeSet := make(map[string]struct{})
	for _, tag := range tags {
		removeSet[tagutil.Normalize(tag)] = struct{}{}
	}

	for _, profileID := range profileIds {
		profile, exists := a.browserMgr.Profiles[profileID]
		if !exists {
			continue
		}
		filtered := profile.Tags[:0]
		for _, tag := range profile.Tags {
			if _, ok := removeSet[tagutil.Normalize(tag)]; !ok {
				filtered = append(filtered, tag)
			}
		}
		profile.Tags = filtered
		profile.UpdatedAt = time.Now().Format(time.RFC3339)
		if a.browserMgr.ProfileDAO != nil {
			if err := a.browserMgr.ProfileDAO.Upsert(profile); err != nil {
				log.Error("批量移除标签失败", logger.F("profile_id", profileID), logger.F("error", err))
				return err
			}
		}
	}
	return nil
}

// BrowserCreateTag 在标签注册表中注册一个新标签（幂等，归一为小写）。
// 标签管理页「新建标签」调用，使新标签持久化、并进入实例编辑器建议与实例列表筛选。
func (a *App) BrowserCreateTag(tagName string) error {
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()
	tagName = browser.NormalizeTag(tagName)
	if tagName == "" {
		return fmt.Errorf("标签名称不能为空")
	}
	if a.browserMgr.TagRegistry == nil {
		return fmt.Errorf("标签注册表不可用")
	}
	return a.browserMgr.TagRegistry.Ensure(tagName)
}

// BrowserDeleteTag 删除一个标签:三清——同时从「标签注册表 + 所有实例 + 所有账号」移除(大小写不敏感)。
// 取代旧的「只删注册表、不触碰实例/账号」语义(那会在标签册聚合时让标签复活,表现为删不掉)。
// 尽力而为:任一步失败不中断后续步骤,结束返回聚合错误;操作幂等,可安全重试收敛。
func (a *App) BrowserDeleteTag(tagName string) error {
	log := logger.New("Browser")
	tagName = browser.NormalizeTag(tagName)
	if tagName == "" {
		return fmt.Errorf("标签名称不能为空")
	}
	a.browserMgr.InitData()

	var registryErr error
	profileFail := 0
	removedProfiles := 0

	// ① 注册表 + ② 实例(同一把锁内)。注册表放最前——它是「删了复活」症状的源头。
	a.browserMgr.Mutex.Lock()
	if a.browserMgr.TagRegistry != nil {
		if err := a.browserMgr.TagRegistry.Delete(tagName); err != nil {
			registryErr = err
			log.Error("删除注册表标签失败", logger.F("tag", tagName), logger.F("error", err))
		}
	}
	for _, profile := range a.browserMgr.Profiles {
		if !tagutil.ContainsFold(profile.Tags, tagName) {
			continue
		}
		filtered := profile.Tags[:0]
		for _, t := range profile.Tags {
			if tagutil.Normalize(t) != tagName {
				filtered = append(filtered, t)
			}
		}
		profile.Tags = filtered
		profile.UpdatedAt = time.Now().Format(time.RFC3339)
		if a.browserMgr.ProfileDAO != nil {
			if err := a.browserMgr.ProfileDAO.Upsert(profile); err != nil {
				profileFail++
				log.Error("从实例移除标签失败", logger.F("profile_id", profile.ProfileId), logger.F("error", err))
				continue
			}
		}
		removedProfiles++
	}
	a.browserMgr.Mutex.Unlock()

	// ②b 回收站实例同样剥离,防止恢复后标签复活(不进 Manager.Profiles,直接走 DAO)
	trashFail := 0
	if a.browserMgr.ProfileDAO != nil {
		if deleted, err := a.browserMgr.ProfileDAO.ListDeleted(); err == nil {
			for _, profile := range deleted {
				if !tagutil.ContainsFold(profile.Tags, tagName) {
					continue
				}
				filtered := profile.Tags[:0]
				for _, t := range profile.Tags {
					if tagutil.Normalize(t) != tagName {
						filtered = append(filtered, t)
					}
				}
				profile.Tags = filtered
				if err := a.browserMgr.ProfileDAO.Upsert(profile); err != nil {
					trashFail++
				}
			}
		}
	}

	// ③ 账号(在 browserMgr 锁外,缩短临界区;账号池可能为 nil,如部分测试/异常环境)
	var accountFail error
	removedAccounts := 0
	if a.accountPool != nil {
		affected, err := a.accountPool.RemoveTagFromAll(tagName)
		removedAccounts = affected
		if err != nil {
			accountFail = err
			log.Error("从账号移除标签失败", logger.F("tag", tagName), logger.F("error", err))
		}
	}

	log.Info("删除标签(三清)", logger.F("tag", tagName),
		logger.F("removed_profiles", removedProfiles), logger.F("removed_accounts", removedAccounts))

	// 聚合失败:有任何子步骤失败就返回错误,但数据已尽量清理,前端照常刷新即可看到收敛态。
	if registryErr != nil || profileFail > 0 || trashFail > 0 || accountFail != nil {
		return fmt.Errorf("删除标签部分失败(注册表:%v;实例失败:%d;回收站失败:%d;账号:%v)",
			registryErr, profileFail, trashFail, accountFail)
	}
	return nil
}

// BrowserRenameTag 重命名所有实例中的指定标签(归一为小写;范围限实例+注册表,不含账号——见方案 G)。
func (a *App) BrowserRenameTag(oldName string, newName string) error {
	log := logger.New("Browser")
	oldName = browser.NormalizeTag(oldName)
	newName = browser.NormalizeTag(newName)
	if oldName == "" || newName == "" {
		return fmt.Errorf("标签名称不能为空")
	}

	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()

	changedCount := 0
	for profileID, profile := range a.browserMgr.Profiles {
		tagChanged := false
		var newTags []string
		for _, tag := range profile.Tags {
			if strings.EqualFold(tag, oldName) {
				newTags = append(newTags, newName)
				tagChanged = true
			} else {
				newTags = append(newTags, tag)
			}
		}

		if tagChanged {
			profile.Tags = browser.NormalizeTags(newTags)
			profile.UpdatedAt = time.Now().Format(time.RFC3339)
			if a.browserMgr.ProfileDAO != nil {
				if err := a.browserMgr.ProfileDAO.Upsert(profile); err != nil {
					log.Error("重命名标签保存失败", logger.F("profile_id", profileID), logger.F("error", err))
					return err
				}
			}
			changedCount++
		}
	}

	// 同步标签注册表：即使没有实例挂着该标签，注册表条目也要重命名
	if a.browserMgr.TagRegistry != nil {
		if err := a.browserMgr.TagRegistry.Ensure(newName); err != nil {
			log.Error("重命名标签注册失败", logger.F("new", newName), logger.F("error", err))
			return err
		}
		if err := a.browserMgr.TagRegistry.Delete(oldName); err != nil {
			log.Error("重命名旧标签删除失败", logger.F("old", oldName), logger.F("error", err))
			return err
		}
	}

	if changedCount > 0 && a.browserMgr.ProfileDAO == nil {
		if err := a.browserMgr.SaveProfiles(); err != nil {
			return err
		}
	}

	if changedCount > 0 {
		log.Info("重命名标签成功", logger.F("old", oldName), logger.F("new", newName), logger.F("changed_profiles", changedCount))
	}
	return nil
}
