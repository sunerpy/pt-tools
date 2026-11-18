package v2

import "strings"

// 读取优惠时遇到的问题类型。TorrentItem.DiscountErr 只写「字段名: 问题类型」，不写原始值，也不放原始响应。
const (
	discountErrTimeUnparsable   = "时间格式无法解析"
	discountErrFormatUnparsable = "格式无法解析"
)

// DiscountTimeParseNote 是「字段 field 的时间非空但无法解析」的诊断说明，例如 "discountEndTime: 时间格式无法解析"。
func DiscountTimeParseNote(field string) string {
	return field + ": " + discountErrTimeUnparsable
}

// discountFormatParseNote 是「字段 field 的格式无法解析」的诊断说明（例如促销适用范围不是列表）
func discountFormatParseNote(field string) string {
	return field + ": " + discountErrFormatUnparsable
}

// NoteDiscountTimeUnparsable 记下优惠时间字段 field 非空但无法解析：说明追加到 DiscountErr（多条用「; 」连接），
// 免费级别的有效性改为 unknown——结束时间不可信，不能当作免费放行的凭据，更不能当成长期免费。
// 调用前先写好 DiscountLevel；没有优惠时结束时间无关，不记。
func NoteDiscountTimeUnparsable(item *TorrentItem, field string) {
	if item == nil || item.DiscountLevel == DiscountNone || item.DiscountLevel == "" {
		return
	}
	markDiscountUnreliable(item, DiscountTimeParseNote(field))
}

// markDiscountUnreliable 记下读取优惠时遇到的问题 note：说明追加到 DiscountErr，免费级别的有效性改为 unknown
func markDiscountUnreliable(item *TorrentItem, note string) {
	if item == nil || strings.TrimSpace(note) == "" {
		return
	}
	item.DiscountErr = joinDiscountNotes(item.DiscountErr, note)
	if IsFreeTorrent(item.DiscountLevel) {
		item.DiscountValidity = FreeUnknown
	}
}

// joinDiscountNotes 用「; 」连接非空的诊断说明
func joinDiscountNotes(notes ...string) string {
	kept := make([]string, 0, len(notes))
	for _, n := range notes {
		if n = strings.TrimSpace(n); n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}
