package backend

import (
	"ant-chrome/backend/internal/logger"
	"fmt"
	"strings"
	"time"
)

// BrowserProfileBatchSetTags 批量为实例设置标签（追加模式：将 tags 加入已有标签；replace 模式：直接替换）
func (a *App) BrowserProfileBatchSetTags(profileIds []string, tags []string, replace bool) error {
	log := logger.New("Browser")
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()

	for _, profileID := range profileIds {
		profile, exists := a.browserMgr.Profiles[profileID]
		if !exists {
			continue
		}
		if replace {
			profile.Tags = tags
		} else {
			existing := make(map[string]struct{})
			for _, tag := range profile.Tags {
				existing[tag] = struct{}{}
			}
			for _, tag := range tags {
				if _, ok := existing[tag]; !ok {
					profile.Tags = append(profile.Tags, tag)
					existing[tag] = struct{}{}
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
	return nil
}

// BrowserProfileBatchRemoveTags 批量从实例移除指定标签
func (a *App) BrowserProfileBatchRemoveTags(profileIds []string, tags []string) error {
	log := logger.New("Browser")
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()

	removeSet := make(map[string]struct{})
	for _, tag := range tags {
		removeSet[tag] = struct{}{}
	}

	for _, profileID := range profileIds {
		profile, exists := a.browserMgr.Profiles[profileID]
		if !exists {
			continue
		}
		filtered := profile.Tags[:0]
		for _, tag := range profile.Tags {
			if _, ok := removeSet[tag]; !ok {
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

// BrowserCreateTag 在标签注册表中注册一个新标签（幂等）。
// 标签管理页「新建标签」调用，使新标签持久化、并进入实例编辑器建议与实例列表筛选。
func (a *App) BrowserCreateTag(tagName string) error {
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()
	tagName = strings.TrimSpace(tagName)
	if tagName == "" {
		return fmt.Errorf("标签名称不能为空")
	}
	if a.browserMgr.TagRegistry == nil {
		return fmt.Errorf("标签注册表不可用")
	}
	return a.browserMgr.TagRegistry.Ensure(tagName)
}

// BrowserDeleteTag 从标签注册表删除一个标签（不影响已挂在实例上的标签）。
func (a *App) BrowserDeleteTag(tagName string) error {
	a.browserMgr.Mutex.Lock()
	defer a.browserMgr.Mutex.Unlock()
	tagName = strings.TrimSpace(tagName)
	if tagName == "" {
		return fmt.Errorf("标签名称不能为空")
	}
	if a.browserMgr.TagRegistry == nil {
		return fmt.Errorf("标签注册表不可用")
	}
	return a.browserMgr.TagRegistry.Delete(tagName)
}

// BrowserRenameTag 重命名所有实例中的指定标签
func (a *App) BrowserRenameTag(oldName string, newName string) error {
	log := logger.New("Browser")
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
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
			uniqueTags := make([]string, 0)
			seen := make(map[string]struct{})
			for _, tag := range newTags {
				if _, ok := seen[tag]; !ok {
					uniqueTags = append(uniqueTags, tag)
					seen[tag] = struct{}{}
				}
			}

			profile.Tags = uniqueTags
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
