// Package tagutil 提供标签的唯一归一口径。
//
// 标签系统有「实例标签 / 标签注册表 / 账号标签」三份存储,历史上各自归一(或都不归一),
// 导致同一逻辑标签出现 OpenCode / opencode / " opencode " 等多种写法,删除时只清一种、其余的复活。
// 本包是所有写入/删除路径共用的唯一归一实现:trim + 小写 + 去重,比较一律大小写不敏感。
// 存储层(browser / accountpool / database)统一依赖本包,保证全库同一标签只有一种写法(小写)。
package tagutil

import "strings"

// Normalize 归一单个标签:trim + 转小写。返回空串表示非法标签(全空白)。
func Normalize(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// NormalizeAll 归一标签列表:逐个 trim + 转小写,去空、按归一值去重,保留首次出现顺序。
// nil 或全空入参返回空切片(非 nil),便于直接 JSON 序列化为 []。
func NormalizeAll(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		v := Normalize(t)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ContainsFold 判断 tags 是否包含 tag(大小写、首尾空白不敏感)。
func ContainsFold(tags []string, tag string) bool {
	want := Normalize(tag)
	if want == "" {
		return false
	}
	for _, t := range tags {
		if Normalize(t) == want {
			return true
		}
	}
	return false
}
