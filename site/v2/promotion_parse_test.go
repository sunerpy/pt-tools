package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNoteDiscountTimeUnparsable(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	free := &TorrentItem{DiscountLevel: DiscountFree}
	NoteDiscountTimeUnparsable(free, "discountEndTime")
	assert.Equal(t, FreeUnknown, free.DiscountValidity)
	assert.Equal(t, "discountEndTime: 时间格式无法解析", free.DiscountErr)
	assert.Equal(t, FreeUnknown, free.Promotion(now).Validity)

	// 第二条说明用「; 」接在后面
	NoteDiscountTimeUnparsable(free, "until")
	assert.Equal(t, "discountEndTime: 时间格式无法解析; until: 时间格式无法解析", free.DiscountErr)

	// 非免费：记下说明，有效性不动（Promotion 照样是 not_free）
	half := &TorrentItem{DiscountLevel: DiscountPercent50}
	NoteDiscountTimeUnparsable(half, "discountEndTime")
	assert.Empty(t, half.DiscountValidity)
	assert.Equal(t, "discountEndTime: 时间格式无法解析", half.DiscountErr)
	assert.Equal(t, FreeNotFree, half.Promotion(now).Validity)

	// 没有优惠：结束时间无关，不记
	none := &TorrentItem{DiscountLevel: DiscountNone}
	NoteDiscountTimeUnparsable(none, "discountEndTime")
	assert.Empty(t, none.DiscountErr)
	NoteDiscountTimeUnparsable(nil, "discountEndTime")

	// 空说明不改动
	item := &TorrentItem{DiscountLevel: DiscountFree}
	markDiscountUnreliable(item, " ")
	assert.Empty(t, item.DiscountValidity)
	assert.Empty(t, item.DiscountErr)

	assert.Equal(t, "a; b", joinDiscountNotes("", "a", " ", "b"))
}
