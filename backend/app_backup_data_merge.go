package backend

import (
	"ant-chrome/backend/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) backupMergeProxiesFile(payloadRoot string, resetFirst bool, stats *backupMergeStats) error {
	srcPath := filepath.Join(payloadRoot, "system", "proxies.yaml")
	dstPath := a.resolveAppPath("proxies.yaml")

	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			if resetFirst {
				_ = os.Remove(dstPath)
			}
			return nil
		}
		return err
	}

	if resetFirst {
		return backupCopyFile(srcPath, dstPath)
	}

	incoming, err := config.LoadProxies(srcPath)
	if err != nil {
		return err
	}
	current, err := config.LoadProxies(dstPath)
	if err != nil {
		return err
	}

	merged := append([]config.BrowserProxy{}, current...)
	existingID := make(map[string]struct{}, len(current))
	existingCfg := make(map[string]struct{}, len(current))
	for _, p := range current {
		existingID[strings.ToLower(strings.TrimSpace(p.ProxyId))] = struct{}{}
		existingCfg[strings.ToLower(strings.TrimSpace(p.ProxyConfig))] = struct{}{}
	}
	for _, p := range incoming {
		idKey := strings.ToLower(strings.TrimSpace(p.ProxyId))
		cfgKey := strings.ToLower(strings.TrimSpace(p.ProxyConfig))
		if _, ok := existingID[idKey]; ok {
			stats.Skipped++
			continue
		}
		if cfgKey != "" {
			if _, ok := existingCfg[cfgKey]; ok {
				stats.Skipped++
				continue
			}
		}
		merged = append(merged, p)
		existingID[idKey] = struct{}{}
		if cfgKey != "" {
			existingCfg[cfgKey] = struct{}{}
		}
		stats.Imported++
	}

	return config.SaveProxies(dstPath, merged)
}

func backupFindDatabaseFile(payloadRoot string) string {
	candidates := []string{
		filepath.Join(payloadRoot, "app", "database", "app.db"),
		filepath.Join(payloadRoot, "app", "data", "app.db"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func (a *App) backupMergeDatabaseFromSource(srcDBPath string, resetFirst bool, stats *backupMergeStats) error {
	if a.db == nil || a.db.GetConn() == nil {
		return fmt.Errorf("数据库未初始化")
	}
	tx, err := a.db.GetConn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ATTACH DATABASE ? AS src`, srcDBPath); err != nil {
		return fmt.Errorf("挂载备份数据库失败: %w", err)
	}
	defer tx.Exec(`DETACH DATABASE src`)

	mergeTables := []struct {
		name       string
		insertAll  string
		insertSafe string
	}{
		{
			name: "browser_groups",
			insertAll: `INSERT INTO browser_groups (group_id, group_name, parent_id, sort_order, created_at, updated_at)
SELECT group_id, group_name, parent_id, sort_order, created_at, updated_at FROM src.browser_groups`,
			insertSafe: `INSERT INTO browser_groups (group_id, group_name, parent_id, sort_order, created_at, updated_at)
SELECT s.group_id, s.group_name, s.parent_id, s.sort_order, s.created_at, s.updated_at
FROM src.browser_groups s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_groups t
  WHERE t.group_id = s.group_id OR (t.parent_id = s.parent_id AND lower(t.group_name) = lower(s.group_name))
)`,
		},
		{
			name: "browser_cores",
			insertAll: `INSERT INTO browser_cores (core_id, core_name, core_path, is_default, sort_order, created_at)
SELECT core_id, core_name, core_path, is_default, sort_order, created_at FROM src.browser_cores`,
			insertSafe: `INSERT INTO browser_cores (core_id, core_name, core_path, is_default, sort_order, created_at)
SELECT s.core_id, s.core_name, s.core_path, s.is_default, s.sort_order, s.created_at
FROM src.browser_cores s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_cores t
  WHERE t.core_id = s.core_id OR lower(t.core_path) = lower(s.core_path)
)`,
		},
		{
			name: "browser_proxies",
			insertAll: `INSERT INTO browser_proxies (proxy_id, proxy_name, proxy_config, dns_servers, group_name, source_id, source_url, source_name_prefix, source_auto_refresh, source_refresh_interval_m, source_last_refresh_at, last_latency_ms, last_test_ok, last_tested_at, last_ip_health_json, sort_order, created_at)
SELECT proxy_id, proxy_name, proxy_config, dns_servers, COALESCE(group_name,''), COALESCE(source_id,''), COALESCE(source_url,''), COALESCE(source_name_prefix,''), COALESCE(source_auto_refresh,0), COALESCE(source_refresh_interval_m,0), COALESCE(source_last_refresh_at,''), COALESCE(last_latency_ms,-1), COALESCE(last_test_ok,0), COALESCE(last_tested_at,''), COALESCE(last_ip_health_json,''), sort_order, created_at
FROM src.browser_proxies`,
			insertSafe: `INSERT INTO browser_proxies (proxy_id, proxy_name, proxy_config, dns_servers, group_name, source_id, source_url, source_name_prefix, source_auto_refresh, source_refresh_interval_m, source_last_refresh_at, last_latency_ms, last_test_ok, last_tested_at, last_ip_health_json, sort_order, created_at)
SELECT s.proxy_id, s.proxy_name, s.proxy_config, s.dns_servers, COALESCE(s.group_name,''), COALESCE(s.source_id,''), COALESCE(s.source_url,''), COALESCE(s.source_name_prefix,''), COALESCE(s.source_auto_refresh,0), COALESCE(s.source_refresh_interval_m,0), COALESCE(s.source_last_refresh_at,''), COALESCE(s.last_latency_ms,-1), COALESCE(s.last_test_ok,0), COALESCE(s.last_tested_at,''), COALESCE(s.last_ip_health_json,''), s.sort_order, s.created_at
FROM src.browser_proxies s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_proxies t
  WHERE t.proxy_id = s.proxy_id OR lower(t.proxy_config) = lower(s.proxy_config)
)`,
		},
		{
			name: "browser_profiles",
			insertAll: `INSERT INTO browser_profiles (profile_id, profile_name, user_data_dir, core_id, fingerprint_args, proxy_id, proxy_config, launch_args, tags, keywords, group_id, created_at, updated_at)
SELECT profile_id, profile_name, user_data_dir, core_id, fingerprint_args, proxy_id, proxy_config, launch_args, tags, keywords, COALESCE(group_id,''), created_at, updated_at
FROM src.browser_profiles`,
			insertSafe: `INSERT INTO browser_profiles (profile_id, profile_name, user_data_dir, core_id, fingerprint_args, proxy_id, proxy_config, launch_args, tags, keywords, group_id, created_at, updated_at)
SELECT s.profile_id, s.profile_name, s.user_data_dir, s.core_id, s.fingerprint_args, s.proxy_id, s.proxy_config, s.launch_args, s.tags, s.keywords, COALESCE(s.group_id,''), s.created_at, s.updated_at
FROM src.browser_profiles s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_profiles t
  WHERE t.profile_id = s.profile_id OR lower(t.user_data_dir) = lower(s.user_data_dir)
)`,
		},
		{
			name: "browser_bookmarks",
			insertAll: `INSERT INTO browser_bookmarks (name, url, sort_order)
SELECT name, url, sort_order FROM src.browser_bookmarks`,
			insertSafe: `INSERT INTO browser_bookmarks (name, url, sort_order)
SELECT s.name, s.url, s.sort_order
FROM src.browser_bookmarks s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_bookmarks t WHERE lower(t.url) = lower(s.url)
)`,
		},
		{
			name: "browser_extensions",
			insertAll: `INSERT INTO browser_extensions (extension_id, name, version, description, manifest_json, source_url, install_dir, enabled, installed_at, updated_at)
SELECT extension_id, name, version, description, manifest_json, source_url, install_dir, enabled, installed_at, updated_at FROM src.browser_extensions`,
			insertSafe: `INSERT INTO browser_extensions (extension_id, name, version, description, manifest_json, source_url, install_dir, enabled, installed_at, updated_at)
SELECT s.extension_id, s.name, s.version, s.description, s.manifest_json, s.source_url, s.install_dir, s.enabled, s.installed_at, s.updated_at
FROM src.browser_extensions s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_extensions t WHERE t.extension_id = s.extension_id
)`,
		},
		{
			name: "browser_profile_extension_settings",
			insertAll: `INSERT INTO browser_profile_extension_settings (profile_id, configured, updated_at)
SELECT profile_id, configured, updated_at FROM src.browser_profile_extension_settings`,
			insertSafe: `INSERT INTO browser_profile_extension_settings (profile_id, configured, updated_at)
SELECT s.profile_id, s.configured, s.updated_at
FROM src.browser_profile_extension_settings s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_profile_extension_settings t WHERE t.profile_id = s.profile_id
)`,
		},
		{
			name: "browser_profile_extensions",
			insertAll: `INSERT INTO browser_profile_extensions (profile_id, extension_id, enabled, created_at, updated_at)
SELECT profile_id, extension_id, enabled, created_at, updated_at FROM src.browser_profile_extensions`,
			insertSafe: `INSERT INTO browser_profile_extensions (profile_id, extension_id, enabled, created_at, updated_at)
SELECT s.profile_id, s.extension_id, s.enabled, s.created_at, s.updated_at
FROM src.browser_profile_extensions s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_profile_extensions t WHERE t.profile_id = s.profile_id AND t.extension_id = s.extension_id
)`,
		},
		{
			name: "launch_codes",
			insertAll: `INSERT INTO launch_codes (profile_id, code, created_at, updated_at)
SELECT profile_id, code, created_at, updated_at FROM src.launch_codes`,
			insertSafe: `INSERT INTO launch_codes (profile_id, code, created_at, updated_at)
SELECT s.profile_id, s.code, s.created_at, s.updated_at
FROM src.launch_codes s
WHERE NOT EXISTS (
  SELECT 1 FROM launch_codes t
  WHERE t.profile_id = s.profile_id OR t.code = s.code
)`,
		},
		// accounts(v21 列形,email/phone 一等列,无 platform):reset 恢复时整表重灌;
		// 非 reset(合并)时按 account_id 去重跳过。email/phone 列在下方按源库是否存在再决定是否带上,
		// 以兼容 v21 之前的旧备份(有 platform、无 email/phone)。
		// 兜底:目标库对 LOWER(email)/LOWER(phone) 建了部分唯一索引(v20/v21),任何一行冲突都会让
		// 整条 INSERT 报错并回滚整个数据库导入(所有表同一事务)。故两套插入都用 OR IGNORE 逐行跳过冲突,
		// 保证一个冲突账号不拖垮其余全部数据的恢复。
		{
			name: "accounts",
			insertAll: `INSERT OR IGNORE INTO accounts (account_id, account_name, account_ref, bound_profile_id, proxy_id, status, cooldown_until, notes, tags, group_id, credential_json, metadata_json, last_used_at, created_at, updated_at, deleted_at)
SELECT account_id, account_name, COALESCE(account_ref,''), COALESCE(bound_profile_id,''), COALESCE(proxy_id,''), COALESCE(status,'active'), COALESCE(cooldown_until,''), COALESCE(notes,''), COALESCE(tags,'[]'), COALESCE(group_id,''), COALESCE(credential_json,'{}'), COALESCE(metadata_json,'{}'), COALESCE(last_used_at,''), created_at, updated_at, COALESCE(deleted_at,'') FROM src.accounts`,
			insertSafe: `INSERT OR IGNORE INTO accounts (account_id, account_name, account_ref, bound_profile_id, proxy_id, status, cooldown_until, notes, tags, group_id, credential_json, metadata_json, last_used_at, created_at, updated_at, deleted_at)
SELECT s.account_id, s.account_name, COALESCE(s.account_ref,''), COALESCE(s.bound_profile_id,''), COALESCE(s.proxy_id,''), COALESCE(s.status,'active'), COALESCE(s.cooldown_until,''), COALESCE(s.notes,''), COALESCE(s.tags,'[]'), COALESCE(s.group_id,''), COALESCE(s.credential_json,'{}'), COALESCE(s.metadata_json,'{}'), COALESCE(s.last_used_at,''), s.created_at, s.updated_at, COALESCE(s.deleted_at,'')
FROM src.accounts s
WHERE NOT EXISTS (
  SELECT 1 FROM accounts t WHERE t.account_id = s.account_id
)`,
		},
		// browser_tags:标签注册表;reset 恢复时重灌,合并时按 tag_name(NOCASE)去重跳过。
		// 兜底:tag_name 主键是 NOCASE 的,旧备份可能含 'Foo'/'foo' 大小写重复行(v19 之前大小写敏感),
		// 用 OR IGNORE 逐行跳过,避免一行冲突回滚整个导入。
		{
			name: "browser_tags",
			insertAll: `INSERT OR IGNORE INTO browser_tags (tag_name, created_at)
SELECT tag_name, created_at FROM src.browser_tags`,
			insertSafe: `INSERT OR IGNORE INTO browser_tags (tag_name, created_at)
SELECT s.tag_name, s.created_at
FROM src.browser_tags s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_tags t WHERE t.tag_name = s.tag_name
)`,
		},
	}

	for _, item := range mergeTables {
		exists, err := backupSrcTableExists(tx, item.name)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}

		total, err := backupCountRows(tx, "src."+item.name)
		if err != nil {
			return err
		}
		if total == 0 {
			continue
		}

		sqlText := item.insertAll
		if !resetFirst {
			sqlText = item.insertSafe
		}
		if item.name == "browser_bookmarks" {
			hasOpenOnStart, err := backupSrcColumnExists(tx, item.name, "open_on_start")
			if err != nil {
				return err
			}
			if hasOpenOnStart {
				if resetFirst {
					sqlText = `INSERT INTO browser_bookmarks (name, url, open_on_start, sort_order)
SELECT name, url, COALESCE(open_on_start,0), sort_order FROM src.browser_bookmarks`
				} else {
					sqlText = `INSERT INTO browser_bookmarks (name, url, open_on_start, sort_order)
SELECT s.name, s.url, COALESCE(s.open_on_start,0), s.sort_order
FROM src.browser_bookmarks s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_bookmarks t WHERE lower(t.url) = lower(s.url)
)`
				}
			}
		}
		if item.name == "accounts" {
			// 仅当源备份的 accounts 已含 email/phone 列(v21 及以后)才一并恢复这两个一等身份列;
			// 否则按默认列集插入(email/phone 落 v20 默认 ''),兼容 v21 之前的旧备份。
			hasEmail, err := backupSrcColumnExists(tx, item.name, "email")
			if err != nil {
				return err
			}
			hasPhone, err := backupSrcColumnExists(tx, item.name, "phone")
			if err != nil {
				return err
			}
			// 平台折叠:v21 把 platform 列物理删除前,会在本地库就地 BackfillPlatformTags(xhs/x -> tags)。
			// 但恢复旧备份走的是 ATTACH + INSERT...SELECT,目标库已无 platform 列、回填又只在启动 Migrate 跑,
			// 若不在这里折叠,旧备份中的 platform 归属会被静默丢弃。故当源含 platform 列时,
			// 用 json_insert 把 xhs/x 追加进 tags(缺失才追加;非法 tags JSON 视为空重建),与就地去重逻辑对齐。
			hasPlatform, err := backupSrcColumnExists(tx, item.name, "platform")
			if err != nil {
				return err
			}
			// tagsFold(列名) 生成把 platform 折进 tags 的 SQL 表达式;无 platform 列时退回原 COALESCE。
			tagsFold := func(col string) string {
				base := "COALESCE(" + col + ",'[]')"
				if !hasPlatform {
					return base
				}
				p := "lower(trim(COALESCE(platform,'')))"
				return "CASE WHEN " + p + " IN ('xhs','x')" +
					" AND NOT EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(" + col + ") THEN " + col + " ELSE '[]' END) je WHERE lower(trim(je.value)) = " + p + ")" +
					" THEN json_insert(CASE WHEN json_valid(" + col + ") THEN " + col + " ELSE '[]' END, '$[#]', " + p + ")" +
					" ELSE " + base + " END"
			}
			if hasEmail && hasPhone {
				if resetFirst {
					sqlText = `INSERT OR IGNORE INTO accounts (account_id, account_name, account_ref, bound_profile_id, proxy_id, status, cooldown_until, notes, tags, group_id, credential_json, metadata_json, last_used_at, created_at, updated_at, deleted_at, email, phone)
SELECT account_id, account_name, COALESCE(account_ref,''), COALESCE(bound_profile_id,''), COALESCE(proxy_id,''), COALESCE(status,'active'), COALESCE(cooldown_until,''), COALESCE(notes,''), ` + tagsFold("tags") + `, COALESCE(group_id,''), COALESCE(credential_json,'{}'), COALESCE(metadata_json,'{}'), COALESCE(last_used_at,''), created_at, updated_at, COALESCE(deleted_at,''), COALESCE(email,''), COALESCE(phone,'')
FROM src.accounts`
				} else {
					sqlText = `INSERT OR IGNORE INTO accounts (account_id, account_name, account_ref, bound_profile_id, proxy_id, status, cooldown_until, notes, tags, group_id, credential_json, metadata_json, last_used_at, created_at, updated_at, deleted_at, email, phone)
SELECT s.account_id, s.account_name, COALESCE(s.account_ref,''), COALESCE(s.bound_profile_id,''), COALESCE(s.proxy_id,''), COALESCE(s.status,'active'), COALESCE(s.cooldown_until,''), COALESCE(s.notes,''), ` + tagsFold("s.tags") + `, COALESCE(s.group_id,''), COALESCE(s.credential_json,'{}'), COALESCE(s.metadata_json,'{}'), COALESCE(s.last_used_at,''), s.created_at, s.updated_at, COALESCE(s.deleted_at,''), COALESCE(s.email,''), COALESCE(s.phone,'')
FROM src.accounts s
WHERE NOT EXISTS (
  SELECT 1 FROM accounts t WHERE t.account_id = s.account_id
)`
				}
			} else if hasPlatform {
				// 旧备份(无 email/phone,有 platform):除默认列集外,还需把 platform 折进 tags。
				if resetFirst {
					sqlText = `INSERT OR IGNORE INTO accounts (account_id, account_name, account_ref, bound_profile_id, proxy_id, status, cooldown_until, notes, tags, group_id, credential_json, metadata_json, last_used_at, created_at, updated_at, deleted_at)
SELECT account_id, account_name, COALESCE(account_ref,''), COALESCE(bound_profile_id,''), COALESCE(proxy_id,''), COALESCE(status,'active'), COALESCE(cooldown_until,''), COALESCE(notes,''), ` + tagsFold("tags") + `, COALESCE(group_id,''), COALESCE(credential_json,'{}'), COALESCE(metadata_json,'{}'), COALESCE(last_used_at,''), created_at, updated_at, COALESCE(deleted_at,'')
FROM src.accounts`
				} else {
					sqlText = `INSERT OR IGNORE INTO accounts (account_id, account_name, account_ref, bound_profile_id, proxy_id, status, cooldown_until, notes, tags, group_id, credential_json, metadata_json, last_used_at, created_at, updated_at, deleted_at)
SELECT s.account_id, s.account_name, COALESCE(s.account_ref,''), COALESCE(s.bound_profile_id,''), COALESCE(s.proxy_id,''), COALESCE(s.status,'active'), COALESCE(s.cooldown_until,''), COALESCE(s.notes,''), ` + tagsFold("s.tags") + `, COALESCE(s.group_id,''), COALESCE(s.credential_json,'{}'), COALESCE(s.metadata_json,'{}'), COALESCE(s.last_used_at,''), s.created_at, s.updated_at, COALESCE(s.deleted_at,'')
FROM src.accounts s
WHERE NOT EXISTS (
  SELECT 1 FROM accounts t WHERE t.account_id = s.account_id
)`
				}
			}
		}
		if item.name == "browser_profiles" {
			hasRestoreLastSession, err := backupSrcColumnExists(tx, item.name, "restore_last_session")
			if err != nil {
				return err
			}
			if hasRestoreLastSession {
				if resetFirst {
					sqlText = `INSERT INTO browser_profiles (profile_id, profile_name, user_data_dir, core_id, fingerprint_args, proxy_id, proxy_config, launch_args, tags, keywords, group_id, created_at, updated_at, restore_last_session)
SELECT profile_id, profile_name, user_data_dir, core_id, fingerprint_args, proxy_id, proxy_config, launch_args, tags, keywords, COALESCE(group_id,''), created_at, updated_at, COALESCE(restore_last_session,'')
FROM src.browser_profiles`
				} else {
					sqlText = `INSERT INTO browser_profiles (profile_id, profile_name, user_data_dir, core_id, fingerprint_args, proxy_id, proxy_config, launch_args, tags, keywords, group_id, created_at, updated_at, restore_last_session)
SELECT s.profile_id, s.profile_name, s.user_data_dir, s.core_id, s.fingerprint_args, s.proxy_id, s.proxy_config, s.launch_args, s.tags, s.keywords, COALESCE(s.group_id,''), s.created_at, s.updated_at, COALESCE(s.restore_last_session,'')
FROM src.browser_profiles s
WHERE NOT EXISTS (
  SELECT 1 FROM browser_profiles t
  WHERE t.profile_id = s.profile_id OR lower(t.user_data_dir) = lower(s.user_data_dir)
)`
				}
			}
		}
		res, err := tx.Exec(sqlText)
		if err != nil {
			return fmt.Errorf("导入数据表失败(%s): %w", item.name, err)
		}
		affected, _ := res.RowsAffected()
		inserted := int(affected)
		if inserted < 0 {
			inserted = total
		}
		stats.Imported += inserted
		// 冲突跳过计数:OR IGNORE 命中的行不发改变更,差额即被跳过的行数。
		// accounts/browser_tags 在 reset 模式也可能因唯一索引(email/phone、NOCASE tag_name)冲突而跳过,
		// 故对这两张表 reset 模式也统计 skipped;其余表 reset 已清表,无冲突源,保持原口径。
		conflictAware := !resetFirst || item.name == "accounts" || item.name == "browser_tags"
		if conflictAware && total > inserted {
			stats.Skipped += total - inserted
		}
	}

	return tx.Commit()
}
