package v2

// InfiniteRatio 记「无穷」分享率：有上传、下载量为 0（NexusPHP 页面写「无限」「無限」「Inf.」或 ∞）。JSON 存不了 +Inf，
// 所以用一个足够大的数：网页、App、ChatOps 大于 999999 时显示 ∞，平均分享率只算小于 1000 的（不计入），
// 分享率要求按达标算。CrabPT、GGPT 的站点定义把「无限」直接换成同一个数。
const InfiniteRatio = 9999999

// normalizeRatio 统一无穷分享率的记法（保存用户信息前，所有站点）：有上传、下载量为 0、分享率没认出来（0）或是
// 以前的 -1 的，记成 InfiniteRatio；下载量大于 0 而分享率是负数的，按上传 / 下载重算。
// 2026-10-27 PTTime 只上传没下载，页面写「分享率：无限」，站点定义的正则不认「无限」，记成了 0；
// 别的站点定义也都把「无限」「∞」交给 parseNumber 变成 0，只有 CrabPT、GGPT 折算过。
func normalizeRatio(info *UserInfo) {
	switch {
	case info.Uploaded > 0 && info.Downloaded == 0 && info.Ratio <= 0:
		info.Ratio = InfiniteRatio
	case info.Downloaded > 0 && info.Ratio < 0:
		info.Ratio = float64(info.Uploaded) / float64(info.Downloaded)
	}
}
