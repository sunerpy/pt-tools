package v2

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// 很多新版 NexusPHP 主题（例如 audiences.me 2026-11 改版）只在 /mybonus.php
// 展示「做种数 / 做种体积」，userdetails.php 上没有这两项，导致抓取结果为 0。
// 这里在 userdetails 解析完之后，按需访问一次 mybonus.php 补齐。
var (
	myBonusTotalCountRe    = regexp.MustCompile(`总做种数[^0-9]{0,200}([0-9][0-9,]*)`)
	myBonusOfficialCountRe = regexp.MustCompile(`官组做种数[^0-9]{0,200}([0-9][0-9,]*)`)
	myBonusNormalCountRe   = regexp.MustCompile(`普通做种数[^0-9]{0,200}([0-9][0-9,]*)`)
	myBonusCountRe         = regexp.MustCompile(`做种数[^0-9]{0,200}([0-9][0-9,]*)`)

	myBonusTotalSizeRe    = regexp.MustCompile(`总体积[^0-9]{0,200}([0-9][0-9,]*(?:\.[0-9]+)?)`)
	myBonusOfficialSizeRe = regexp.MustCompile(`官组体积[^0-9]{0,200}([0-9][0-9,]*(?:\.[0-9]+)?)`)
	myBonusNormalSizeRe   = regexp.MustCompile(`普通体积[^0-9]{0,200}([0-9][0-9,]*(?:\.[0-9]+)?)`)
	myBonusSizeRe         = regexp.MustCompile(`(?:做种体积|做种大小|体积)[^0-9]{0,200}([0-9][0-9,]*(?:\.[0-9]+)?)`)
	myBonusUnitRe         = regexp.MustCompile(`(?i)\b(TB|TIB|GB|GIB|MB|MIB|KB|KIB|B)\b`)
)

func myBonusInt(re *regexp.Regexp, body string) (int, bool) {
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return 0, false
	}
	v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0, false
	}
	return v, true
}

// myBonusSize 从「数字 + 单位」两个节点取值。
// 新版主题把单位包在 <small class="...-unit">TB</small> 里，数字与单位之间夹着标签，
// 所以不能用 `数字\s*单位` 的正则，而是在数字之后的一段窗口内找第一个单位。
func myBonusSize(re *regexp.Regexp, body string) (int64, bool) {
	loc := re.FindStringSubmatchIndex(body)
	if len(loc) < 4 {
		return 0, false
	}
	numStr := strings.ReplaceAll(body[loc[2]:loc[3]], ",", "")
	value, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, false
	}
	tail := body[loc[3]:]
	if len(tail) > 240 {
		tail = tail[:240]
	}
	unit := ""
	if u := myBonusUnitRe.FindString(tail); u != "" {
		unit = strings.ToUpper(u)
	}
	mult := float64(1)
	switch {
	case strings.HasPrefix(unit, "K"):
		mult = 1024
	case strings.HasPrefix(unit, "M"):
		mult = 1024 * 1024
	case strings.HasPrefix(unit, "G"):
		mult = 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "T"):
		mult = 1024 * 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "P"):
		mult = 1024 * 1024 * 1024 * 1024 * 1024
	}
	return int64(value * mult), true
}

// fillFromMyBonus 从 /mybonus.php 补齐做种数与做种体积。
func (d *NexusPHPDriver) fillFromMyBonus(ctx context.Context, info *UserInfo) {
	req := NexusPHPRequest{Path: "/mybonus.php", Method: "GET"}
	res, err := d.Execute(ctx, req)
	if err != nil || len(res.RawBody) == 0 {
		return
	}
	body := string(res.RawBody)
	if len(body) < 512 {
		return
	}

	// 做种数：总做种数 > 官组+普通 > 任意做种数
	if info.Seeding == 0 {
		if v, ok := myBonusInt(myBonusTotalCountRe, body); ok {
			info.Seeding = v
		} else {
			off, ok1 := myBonusInt(myBonusOfficialCountRe, body)
			nor, ok2 := myBonusInt(myBonusNormalCountRe, body)
			if ok1 && ok2 {
				info.Seeding = off + nor
			} else if v, ok := myBonusInt(myBonusCountRe, body); ok {
				info.Seeding = v
			}
		}
	}

	// 做种体积：总体积 > 官组+普通 > 任意体积；能拿到更大的总值时以总值为准
	if total, ok := myBonusSize(myBonusTotalSizeRe, body); ok && total > info.SeederSize {
		info.SeederSize = total
	} else if info.SeederSize == 0 {
		if v, ok := myBonusSize(myBonusOfficialSizeRe, body); ok {
			info.SeederSize = v
			if n, ok2 := myBonusSize(myBonusNormalSizeRe, body); ok2 {
				info.SeederSize = v + n
			}
		} else if v, ok := myBonusSize(myBonusSizeRe, body); ok {
			info.SeederSize = v
		}
	}
}
