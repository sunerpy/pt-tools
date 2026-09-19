// MIT License
// Copyright (c) 2025 pt-tools

package models

import "strings"

/*
 * TorrentInfo.FreeLevel 这一列里存过三代拼法，筛选必须认全：
 *
 *   - site/v2 的规范值（当前写入路径）：FREE / 2XFREE / PERCENT_50 / PERCENT_30 /
 *     PERCENT_70 / 2XUP / 2X50 / NONE；
 *   - PHP 时代 DiscountType 的字面量（models.PHPTorrentInfo.GetFreeLevel）：
 *     free / 2xfree / 50% / 30% / 2x / 2x50% / none / custom；
 *   - 建表默认值 normal，以及 M-Team 原始串 _2X_FREE / _2X_UP / _2X_PERCENT_50。
 *
 * 只按规范值做等值匹配，就会让库里存着 "30%" 的行在选「30%」时静默消失 ——
 * 页脚数字与表里的行一起少，用户看不出是筛错了还是真没有。
 */

// FreeLevelAliases 把一个规范优惠档映射到库里可能出现过的全部拼法（大写形式）。
var FreeLevelAliases = map[string][]string{
	"FREE":       {"FREE"},
	"2XFREE":     {"2XFREE", "_2X_FREE", "2X FREE"},
	"PERCENT_50": {"PERCENT_50", "50%"},
	"PERCENT_30": {"PERCENT_30", "30%"},
	"PERCENT_70": {"PERCENT_70", "70%"},
	"2XUP":       {"2XUP", "_2X_UP", "2X"},
	"2X50":       {"2X50", "_2X_PERCENT_50", "2X50%"},
	// 空串、NORMAL 都是「没有优惠」：前者是没写过，后者是建表默认值。
	"NONE": {"NONE", "NORMAL", ""},
}

// CanonicalFreeLevel 把任意一种拼法收敛到规范档名。
// 认不出来的原样返回（大写去空白），调用方据此仍能做等值匹配而不是静默放行。
func CanonicalFreeLevel(raw string) string {
	v := strings.ToUpper(strings.TrimSpace(raw))
	for canonical, aliases := range FreeLevelAliases {
		for _, a := range aliases {
			if v == a {
				return canonical
			}
		}
	}
	return v
}

// FreeLevelQueryValues 返回筛某一档时应当匹配的全部存库拼法（大写）。
// 未知档位返回它自己，查询照样成立，只是命中为空。
func FreeLevelQueryValues(raw string) []string {
	canonical := CanonicalFreeLevel(raw)
	if aliases, ok := FreeLevelAliases[canonical]; ok {
		out := make([]string, len(aliases))
		copy(out, aliases)
		return out
	}
	return []string{canonical}
}
