package definitions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func init() {
	RegisterFixtureSuite(FixtureSuite{
		SiteID:   "hhanclub",
		Search:   testHhanclubSearch,
		Detail:   testHhanclubDetail,
		UserInfo: testHhanclubUserInfo,
	})
}

// --- Fixtures ---
//
// 结构照搬真实页面（styles/HHan Tailwind 皮肤，列表页与详情页都没有 table/tr/td），
// 但标题、副标题、种子 ID、用户名、用户 ID、头像地址均已替换为虚构值，
// download.php 链接不带 passkey 参数。
//
// 搜索页刻意覆盖的真实特征：
//   - 首个同级 div 是表头（带 torrent-cat / torrent-title，但没有 torrent-table-sub-info），
//     用于证明行选择器排除表头
//   - 标题锚点内嵌 <span class='new'>[新]</span>（真实页面 100 行中 91 行携带）
//   - 促销标记是站点自定义 span.promotion-tag-*，且每行内联一段 <style type="text/tailwindcss">
//     声明这些 class（该 style 文本会出现在行的 HTML 里，不能干扰选择器）
//   - 促销结束时间是可见 span[title]，与 torrent-info-text-added 里的发布时间同为 span[title]
//   - 统计列使用语义 class，而非固定列序

const hhanclubSearchHeaderFixture = `<div class="flex w-[95%]  bg-[#4F5879]/[0.7] opacity-[0.7] h-[30px] m-auto z-20 !rounded-[3px]">
    <div class="torrent-cat"><span class="!text-[#FFFFFF] text-[16px] font-bold leading-6 m-auto">类型</span></div>
    <div class="torrent-title items-center justify-center"><a class="!text-[#FFFFFF] text-[16px] font-bold leading-6 m-auto" href="?sort=1&amp;type=asc">标题</a></div>
    <div class="flex w-[20%] items-center">
        <hr class="w-[2.5px] h-[80%] bg-[#9B9B9B] opacity-[0.7]"/>
        <div class="torrent-info">
            <a class="torrent-info-icon" href="?sort=5&amp;type=desc"><img src="styles/HHan/icons/icon-torrent-size.svg" alt="size" title="大小"/></a>
            <a class="torrent-info-icon" href="?sort=4&amp;type=desc"><img src="styles/HHan/icons/icon-new-time.svg" alt="time" title="存活时间"/></a>
            <a class="torrent-info-icon" href="?sort=7&amp;type=desc"><img src="styles/HHan/icons/icon-seed-num.svg" alt="seeders" title="种子数"/></a>
            <a class="torrent-info-icon" href="?sort=8&amp;type=desc"><img src="styles/HHan/icons/icon-now-down.svg" alt="leechers" title="下载数"/></a>
            <a class="torrent-info-icon" href="?sort=6&amp;type=desc"><img src="styles/HHan/icons/icon-finished.svg" alt="完成数" title="完成数"></a>
        </div>
        <hr class="w-[2.5px] h-[80%] bg-[#9B9B9B]/[0.7] "/>
    </div>
    <div class="torrent-manage"></div>
</div>`

// 行内促销样式声明，真实页面每行都会重复输出一份。
const hhanclubPromotionStyleFixture = `<style type="text/tailwindcss">
    .promotion-tag { @apply !text-[#FFFFFF] text-[12px] px-[5px] !rounded-md py-[2px]; }
    .promotion-tag-free { @apply bg-[#1FABB4]; }
    .promotion-tag-50 { @apply bg-[#87B05D]; }
    .promotion-tag-30 { @apply bg-[#2C86DA]; }
    .promotion-tag-2xfree { @apply bg-[#109e61]; }
</style>`

const hhanclubSearchFixture = `<html><body>
<div class="flex flex-col w-[95%] m-auto torrent-table-for-spider">
` + hhanclubSearchHeaderFixture + `
<div class="w-full bg-[#11e5e380] flex z-10 !rounded-[3px] torrent-table-sub-info">
    <div class="torrent-cat"><a href='?cat[]=402'><img class='!rounded-md ' src='styles/HHan/icons/icon-tv.svg' alt='类型'></a></div>
    <div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between">
        <div class="flex-col flex  w-[calc(100%-15px)] gap-y-1">
            <a href='details.php?id=99001&hit=1' class='!text-[#000000] hover:!text-orange-400 font-bold text-[9pt] torrent-info-text-name'>Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE<span class='new'>[新]</span></a>
            <div class="flex items-center gap-x-[8px}">
                <div class='text-[#000000]/[0.9] text-[9pt] font-[500] w-20 flex-1 overflow-hidden truncate torrent-info-text-small_name'>虚构副标题一 | 第01-04集 | 4K 60帧</div>
            </div>
            <div class="flex gap-x-[5px] text-[9pt] items-center pr-5">
                <span class="flex !text-[9pt] !font-bold !text-[#000000] items-center"><img src="styles/HHan/icons/icon-douban.svg" alt="douban" title="douban">&nbsp;6.1</span>
                <a href="?tag_id3=1"><span style="background-color:#0000ff" class="tag">官方</span></a>
` + hhanclubPromotionStyleFixture + `
                <div class='whitespace-nowrap'><span class="promotion-tag promotion-tag-free" >免费</span></div>
                <div class="flex"><span class='flex text-[12px]  text-[#000000]'>[&nbsp;剩余时间：<span title="2026-07-27 22:05:11">7时21分钟</span>&nbsp;]</span></div>
                <div class="flex torrent-info-text-author"><div class='flex text-[#000000]' ><b>匿名</b></div></div>
            </div>
        </div>
    </div>
    <div class="flex w-[20%] items-center">
        <div class="torrent-info">
            <div class="torrent-info-text torrent-info-text-size">
                17.33 GB                        </div>
            <div class="torrent-info-text torrent-info-text-added">
                <span title="2026-07-27 14:05:11">38分钟</span>                        </div>
            <div class="torrent-info-text torrent-info-text-seeders"><a href="details.php?id=99001&amp;hit=1&amp;dllist=1#seeders">1</a></div>
            <div class="torrent-info-text torrent-info-text-leechers"><a class='!text-[#000000]' href="details.php?id=99001&amp;hit=1&amp;dllist=1#leechers">119</a></div>
            <div class="torrent-info-text torrent-info-text-finished"><a href='/viewsnatches.php?id=99001'>0</a></div>
        </div>
    </div>
    <div class="torrent-manage">
        <a class='inline-flex' id="bookmark0" href="javascript: mark(99001,0);"><img style="width: 35px" src="styles/HHan/icons/icon-collection-off-new.svg" alt="Unbookmarked" title="收藏" /></a>
        <div class="flex flex-col gap-y-[5px]"><a class='xl:px-[5px] text-[14px] bg-[#549DF7] !text-[#FFFFFF] rounded-[5px] ' href="download.php?id=99001">下载</a></div>
    </div>
</div>
<hr class="w-full h-[2px] bg-[#191E32]/[0.6]"/>
<div class="w-full bg-[#11e5e380] flex z-10 !rounded-[3px] torrent-table-sub-info">
    <div class="torrent-cat"><a href='?cat[]=401'><img class='!rounded-md ' src='styles/HHan/icons/icon-movie.svg' alt='类型'></a></div>
    <div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between">
        <div class="flex-col flex  w-[calc(100%-15px)] gap-y-1">
            <a href='details.php?id=99002&hit=1' class='!text-[#000000] hover:!text-orange-400 font-bold text-[9pt] torrent-info-text-name'>Fixture.Movie.2026.1080p.BluRay.x264-FIXTURE</a>
            <div class="flex items-center gap-x-[8px]">
                <div class='text-[#000000]/[0.9] text-[9pt] font-[500] truncate torrent-info-text-small_name'>虚构副标题二 | 国语中字</div>
            </div>
            <div class="flex gap-x-[5px] text-[9pt] items-center pr-5">
` + hhanclubPromotionStyleFixture + `
                <div class='whitespace-nowrap'><span class="promotion-tag promotion-tag-2xfree" >2X免费</span></div>
                <div class="flex"><span class='flex text-[12px]  text-[#000000]'>[&nbsp;剩余时间：<span title="2026-07-28 09:30:00">19时25分钟</span>&nbsp;]</span></div>
                <div class="flex torrent-info-text-author"><div class='flex text-[#000000]' ><b>匿名</b></div></div>
            </div>
        </div>
    </div>
    <div class="flex w-[20%] items-center">
        <div class="torrent-info">
            <div class="torrent-info-text torrent-info-text-size">816.30 MB</div>
            <div class="torrent-info-text torrent-info-text-added"><span title="2026-07-20 08:00:00">7天</span></div>
            <div class="torrent-info-text torrent-info-text-seeders"><a href="details.php?id=99002&amp;hit=1&amp;dllist=1#seeders">206</a></div>
            <div class="torrent-info-text torrent-info-text-leechers"><a class='!text-[#000000]' href="details.php?id=99002&amp;hit=1&amp;dllist=1#leechers">3</a></div>
            <div class="torrent-info-text torrent-info-text-finished"><a href='/viewsnatches.php?id=99002'>412</a></div>
        </div>
    </div>
    <div class="torrent-manage"><a class='xl:px-[5px] text-[14px]' href="download.php?id=99002">下载</a></div>
</div>
<hr class="w-full h-[2px] bg-[#191E32]/[0.6]"/>
<div class="w-full bg-[#11e5e380] flex z-10 !rounded-[3px] torrent-table-sub-info">
    <div class="torrent-cat"><a href='?cat[]=404'><img class='!rounded-md ' src='styles/HHan/icons/icon-music.svg' alt='类型'></a></div>
    <div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between">
        <div class="flex-col flex  w-[calc(100%-15px)] gap-y-1">
            <a href='details.php?id=99003&hit=1' class='!text-[#000000] font-bold text-[9pt] torrent-info-text-name'>Fixture.Album.2026.FLAC.24bit-FIXTURE<span class='new'>[新]</span></a>
            <div class="flex items-center gap-x-[8px]">
                <div class='text-[#000000]/[0.9] truncate torrent-info-text-small_name'>虚构副标题三 | Hi-Res</div>
            </div>
            <div class="flex gap-x-[5px] text-[9pt] items-center pr-5">
` + hhanclubPromotionStyleFixture + `
                <div class='whitespace-nowrap'><span class="promotion-tag promotion-tag-50" >50%</span></div>
                <div class="flex"><span class='flex text-[12px]  text-[#000000]'>[&nbsp;剩余时间：<span title="2026-07-29 12:00:00">2天</span>&nbsp;]</span></div>
            </div>
        </div>
    </div>
    <div class="flex w-[20%] items-center">
        <div class="torrent-info">
            <div class="torrent-info-text torrent-info-text-size">1.24 GB</div>
            <div class="torrent-info-text torrent-info-text-added"><span title="2026-07-26 20:15:00">18时</span></div>
            <div class="torrent-info-text torrent-info-text-seeders"><a href="details.php?id=99003&amp;hit=1&amp;dllist=1#seeders">18</a></div>
            <div class="torrent-info-text torrent-info-text-leechers"><a class='!text-[#000000]' href="details.php?id=99003&amp;hit=1&amp;dllist=1#leechers">0</a></div>
            <div class="torrent-info-text torrent-info-text-finished"><a href='/viewsnatches.php?id=99003'>7</a></div>
        </div>
    </div>
    <div class="torrent-manage"><a class='xl:px-[5px] text-[14px]' href="download.php?id=99003">下载</a></div>
</div>
<hr class="w-full h-[2px] bg-[#191E32]/[0.6]"/>
<div class="w-full bg-[#11e5e380] flex z-10 !rounded-[3px] torrent-table-sub-info">
    <div class="torrent-cat"><a href='?cat[]=405'><img class='!rounded-md ' src='styles/HHan/icons/icon-doc.svg' alt='类型'></a></div>
    <div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between">
        <div class="flex-col flex  w-[calc(100%-15px)] gap-y-1">
            <a href='details.php?id=99004&hit=1' class='!text-[#000000] font-bold text-[9pt] torrent-info-text-name'>Fixture.Doc.2026.2160p.HDR-FIXTURE</a>
            <div class="flex items-center gap-x-[8px]">
                <div class='text-[#000000]/[0.9] truncate torrent-info-text-small_name'>虚构副标题四 | 纪录片</div>
            </div>
            <div class="flex gap-x-[5px] text-[9pt] items-center pr-5">
` + hhanclubPromotionStyleFixture + `
                <div class='whitespace-nowrap'><span class="promotion-tag promotion-tag-30" >30%</span></div>
                <div class="flex"><span class='flex text-[12px]  text-[#000000]'>[&nbsp;剩余时间：<span title="2026-08-01 00:00:00">4天</span>&nbsp;]</span></div>
            </div>
        </div>
    </div>
    <div class="flex w-[20%] items-center">
        <div class="torrent-info">
            <div class="torrent-info-text torrent-info-text-size">42.80 GB</div>
            <div class="torrent-info-text torrent-info-text-added"><span title="2026-07-01 00:00:00">26天</span></div>
            <div class="torrent-info-text torrent-info-text-seeders"><a href="details.php?id=99004&amp;hit=1&amp;dllist=1#seeders">9</a></div>
            <div class="torrent-info-text torrent-info-text-leechers"><a class='!text-[#000000]' href="details.php?id=99004&amp;hit=1&amp;dllist=1#leechers">1</a></div>
            <div class="torrent-info-text torrent-info-text-finished"><a href='/viewsnatches.php?id=99004'>33</a></div>
        </div>
    </div>
    <div class="torrent-manage"><a class='xl:px-[5px] text-[14px]' href="download.php?id=99004">下载</a></div>
</div>
<hr class="w-full h-[2px] bg-[#191E32]/[0.6]"/>
<div class="w-full bg-[#11e5e380] flex z-10 !rounded-[3px] torrent-table-sub-info">
    <div class="torrent-cat"><a href='?cat[]=403'><img class='!rounded-md ' src='styles/HHan/icons/icon-tv.svg' alt='类型'></a></div>
    <div class="flex torrent-table-for-spider-info torrent-title items-center gap-x-[15px] justify-between">
        <div class="flex-col flex  w-[calc(100%-15px)] gap-y-1">
            <a href='details.php?id=99005&hit=1' class='!text-[#000000] font-bold text-[9pt] torrent-info-text-name'>Fixture.Variety.S03.2026.1080p.WEB-DL-FIXTURE</a>
            <div class="flex items-center gap-x-[8px]">
                <div class='text-[#000000]/[0.9] truncate torrent-info-text-small_name'>虚构副标题五 | 综艺</div>
            </div>
            <div class="flex gap-x-[5px] text-[9pt] items-center pr-5">
                <div class="flex torrent-info-text-author"><div class='flex text-[#000000]' ><b>匿名</b></div></div>
            </div>
        </div>
    </div>
    <div class="flex w-[20%] items-center">
        <div class="torrent-info">
            <div class="torrent-info-text torrent-info-text-size">5.61 GB</div>
            <div class="torrent-info-text torrent-info-text-added"><span title="2026-06-15 10:20:30">42天</span></div>
            <div class="torrent-info-text torrent-info-text-seeders"><a href="details.php?id=99005&amp;hit=1&amp;dllist=1#seeders">77</a></div>
            <div class="torrent-info-text torrent-info-text-leechers"><a class='!text-[#000000]' href="details.php?id=99005&amp;hit=1&amp;dllist=1#leechers">2</a></div>
            <div class="torrent-info-text torrent-info-text-finished"><a href='/viewsnatches.php?id=99005'>901</a></div>
        </div>
    </div>
    <div class="torrent-manage"><a class='xl:px-[5px] text-[14px]' href="download.php?id=99005">下载</a></div>
</div>
</div>
</body></html>`

// 详情页同样是 div 布局：标签 div 与取值 div 相邻；标题/ID 来自 subtitles.php 表单的隐藏域；
// 「基本信息」体积是 <span class="font-bold"><b>大小：</b></span> 的下一个兄弟 span。
const hhanclubDetailFixture = `<html><head><title>HHCLUB :: 种子详情 &quot;Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE&quot; - Powered by NexusPHP</title></head><body>
<div class="xl:w-[90%] w-full m-auto p-10"><div class="bg-content_bg rounded-md py-10 px-20 text-black">
<div class="grid gap-y-5 grid-cols-[100px,calc(100%-100px-1.25rem)] justify-items-start">
    <div class="font-bold leading-6 text-center h-full flex items-center">下载</div>
    <div class="font-bold leading-6 flex flex-row justify-between w-full items-center"><div class="flex gap-x-[10px] items-center"><a class="index" href="download.php?id=99001">[HHC].Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE.torrent</a>
` + hhanclubPromotionStyleFixture + `
<div class='whitespace-nowrap'><span class="promotion-tag promotion-tag-free" >免费</span></div> <span class='flex text-[12px]  text-[#000000]'>[&nbsp;剩余时间：<span title="2026-07-27 22:05:11">7时21分钟</span>&nbsp;]</span></div><div class='leading-6'><a class='flex flex-row items-center' href='report.php?torrent=99001'><span class='text-md'>举报</span></a></div></div>
    <div class='font-bold leading-6'>标题</div><div class='font-bold leading-6'>Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE</div>
    <div class="font-bold leading-6">副标题</div>
    <div class="font-bold leading-6">虚构副标题一 | 第01-04集 | 4K 60帧</div>
    <div class="font-bold leading-6">基本信息</div><div class="grid gap-y-5 grid-cols-4 justify-items-start items-center w-full leading-6"><div><span class="font-bold"><b>大小：</b></span><span class="">17.33 GB</span></div><div><span class="font-bold">类型:&nbsp;&nbsp;</span><span class="">电视剧</span></div><div><span class="font-bold">媒介:&nbsp;&nbsp;</span><span class="">WEB-DL</span></div><div><span class="font-bold">分辨率:&nbsp;&nbsp;</span><span class="">2160p/4K</span></div><div class='flex items-center'><span class='font-bold'>发布时间:&nbsp;&nbsp;</span><span>2026-07-27 14:05:11</span></div></div>
    <div class="font-bold leading-6">种子链接</div><span><a class="btn-basic" href="https://hhanclub.net/download.php?id=99001">点击复制</a></span>
    <div class="font-bold leading-6">字幕</div>
    <div class="inline-flex flex-col gap-y-[15px]"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99001" /><input type="hidden" name="in_detail" value="in_detail" /></form></div>
    <div class="font-bold leading-6">文件列表</div><div class="grid gap-y-1"><div class="text-[13px] grid grid-cols-[60px,1fr]"><span class="w-[100px] ">体　积: </span><span>4.76 GiB</span></div></div>
</div>
</div></div>
</body></html>`

// 无促销 + 体积使用二进制单位（GiB）的变体：验证 SizeRegex 的单位分组已放宽，
// 且 ParseSizeMB 能按 1024 换算 GiB。
const hhanclubDetailGiBFixture = `<html><body>
<div class="grid gap-y-5 grid-cols-[100px,calc(100%-100px-1.25rem)] justify-items-start">
    <div class="font-bold leading-6">基本信息</div><div class="grid gap-y-5 grid-cols-4 justify-items-start items-center w-full leading-6"><div><span class="font-bold"><b>大小：</b></span><span class="">4.32 GiB</span></div><div><span class="font-bold">类型:&nbsp;&nbsp;</span><span class="">电影</span></div></div>
    <div class="font-bold leading-6">副标题</div>
    <div class="font-bold leading-6">虚构副标题六</div>
    <div class="font-bold leading-6">字幕</div>
    <div class="inline-flex"><form method="post" action="subtitles.php"><input type="hidden" name="torrent_name" value="Fixture.NoPromo.2026.1080p.Remux-FIXTURE" /><input type="hidden" name="detail_torrent_id" value="99006" /></form></div>
</div>
</body></html>`

// 全站左侧用户面板（index.php 与 userdetails.php 共用）：
// id / name 取自 a[class*='Name']，做种/下载数取自带 img[alt] 的 div。
const hhanclubIndexFixture = `<html><body>
<div class="absolute z-30 hidden bg-[#FFFFFF] flex flex-col rounded-lg w-[280px]">
    <div class="flex flex-col w-full items-center mt-[150px] gap-y-[5px]">
        <div class="!text-base"><div class="flex flex-wrap items-center"><a  href="https://hhanclub.net/userdetails.php?id=90001" class='CrazyUser_Name'><b>fixture_user</b></a></div></div>
    </div>
    <div class="flex flex-col divide-y-[1px] mt-[25px] items-center w-[calc(100%-30px)]">
        <div class="flex flex-col w-full items-start gap-y-[15px] mb-[25px]">
            <div class="flex flex-row items-center gap-x-[10px]">
                <img loading="lazy" class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-ratio.svg" alt="分享率">
                <div class="text-sm flex items-center justify-start gap-x-[10px] ">[分享率]:&nbsp;6.299</div>
            </div>
        </div>
        <div class="grid grid-cols-[50%_50%] gap-y-[15px] gap-x-[5px] pb-[25px] w-[100%] pt-[25px]">
            <div class="flex flex-row items-center ">
                <img src="styles/HHan/icons/icon-bean.svg" class="w-[18px] h-[18px]" alt="憨豆">
                <a href="mybonus.php"><div class="text-sm flex flex-wrap break-all">1,068,858</div></a>
            </div>
            <div class="text-sm flex items-center justify-start"><img class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-user-upload.svg" alt="上传">&nbsp;12.755 TB</div>
            <a href="userdetails.php?id=90001&action=2"><div class="text-sm flex items-center justify-start"><img class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-now-upload.svg" alt="做种数">&nbsp;140</div></a>
            <div class="text-sm flex items-center justify-start"><img class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-user-download.svg" alt="下载">&nbsp;2.025 TB</div>
            <a href="userdetails.php?id=90001&action=3"><div class="text-sm flex items-center justify-start"><img class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-downloading.svg" alt="下载数">&nbsp;0</div></a>
        </div>
    </div>
</div>
</body></html>`

// userdetails.php 正文：标签统一是 <span class="font-bold">标签：</span>，
// 刻意保留「实际上传量：」「实际下载量：」「实际分享率」等易误命中的兄弟字段，
// 用于验证 :matchesOwn 的整串精确匹配。
const hhanclubUserdetailsFixture = `<html><body>
<div class="absolute z-30 hidden bg-[#FFFFFF] flex flex-col rounded-lg w-[280px]">
    <div class="!text-base"><div class="flex flex-wrap items-center"><a  href="https://hhanclub.net/userdetails.php?id=90001" class='CrazyUser_Name'><b>fixture_user</b></a></div></div>
    <a href="userdetails.php?id=90001&action=2"><div class="text-sm flex items-center"><img class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-now-upload.svg" alt="做种数">&nbsp;140</div></a>
    <a href="userdetails.php?id=90001&action=3"><div class="text-sm flex items-center"><img class="w-[15px] h-[15px]" src="styles/HHan/icons/icon-downloading.svg" alt="下载数">&nbsp;0</div></a>
</div>
<div class="w-[max(85%,1000px)] m-auto p-10 bg-[#191E32]">
    <div class="flex flex-row items-center">
        <div class="font-bold text-xl leading-8 text-primary">个人中心</div>
        <a href="userdetails.php?id=90001"><div class="ml-6 font-medium text-lg leading-7">个人信息</div></a>
        <a href="userdetails.php?id=90001&action=2"><div class="ml-4 text-gray-500 font-medium text-lg leading-7">当前做种</div></a>
        <a href="userdetails.php?id=90001&action=3"><div class="ml-4 text-gray-500 font-medium text-lg leading-7">当前下载</div></a>
    </div>
    <div class="grid grid-cols-3 justify-items-start gap-4 leading-6 text-base mt-12">
        <div class="flex"><span class="font-bold m-auto">用户名：</span><div class="flex flex-wrap items-center"><a  href="https://hhanclub.net/userdetails.php?id=90001" class='CrazyUser_Name'><b>fixture_user</b></a></div></div>
        <div class="flex"><span class="font-bold">性别：</span><span>男</span></div>
        <div class="flex items-center"><span class="font-bold">憨豆：</span><div class="text-base text-black leading-6 flex items-center w-28"><img class="w-5 h-5" src="/styles/HHan/icon-bean.svg" alt="">1068857.7</div></div>
        <div class="flex"><span class="font-bold m-auto">等级：</span><span class='flex items-end text-[14px] gap-x-3'><img alt="Crazy User(虚构等级)" title="Crazy User(虚构等级)" src="pic/crazy.gif" /> <b class='CrazyUser_Name'>Crazy User(虚构等级)</b></span></div>
    </div>
    <hr class="w-full h-px bg-gray-200 my-5" />
    <div class="grid grid-cols-3 justify-items-start gap-4 leading-6 text-base">
        <div><span class="font-bold">H&amp;R：</span><span><a href="myhr.php?userid=90001" target="_blank">0/<font color="red">0</font>/5</a></span></div>
        <div><span class="font-bold">做种积分：</span><span>445,418.0</span></div>
    </div>
    <hr class="w-full h-px bg-gray-200 my-5" />
    <div class="grid grid-cols-3 justify-items-start gap-4 leading-6 text-base">
        <div><span class="font-bold">邀请：</span><span>没有邀请资格</span></div>
        <div><span class="font-bold">加入日期：</span><span>2025-08-19 22:40:26 (<span title="2025-08-19 22:40:26">11月11天前</span>, 48周)</span></div>
        <div><span class="font-bold">最近动向：</span><span>2026-07-27 14:43:26 (<span title="2026-07-27 14:43:26">&lt; 1分钟前</span>)</span></div>
        <div><span class="font-bold">最近访问记录：</span><a class="cursor-pointer" href="userloginlogs.php?uid=90001">共&nbsp;7&nbsp;条记录</a></div>
    </div>
    <hr class="w-full h-px bg-gray-200 my-5" />
    <div class="grid grid-cols-3 justify-items-start gap-4 leading-6 text-base">
        <div><span class="font-bold">上传量：</span><span>12.755 TB</span></div>
        <div><span class="font-bold">下载量：</span><span>2.025 TB</span></div>
        <div><span class="font-bold">分享率：</span><span><font color="">6.300</font>（<strong>实际分享率</strong>：0.785）</span></div>
        <div><span class="font-bold">实际上传量：</span><span>12.315 TB</span></div>
        <div><span class="font-bold">实际下载量：</span><span>15.684 TB</span></div>
    </div>
</div>
</body></html>`

// --- Helpers ---

func getHhanclubDef(t *testing.T) *v2.SiteDefinition {
	t.Helper()
	def, ok := v2.GetDefinitionRegistry().Get("hhanclub")
	require.True(t, ok, "hhanclub definition not found")
	return def
}

// --- Suite: Search ---

func testHhanclubSearch(t *testing.T) {
	def := getHhanclubDef(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(hhanclubSearchFixture))
	}))
	defer server.Close()

	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL:   server.URL,
		Cookie:    "test_cookie=1",
		Selectors: def.Selectors,
	})
	driver.SetSiteDefinition(def)

	res, err := driver.Execute(context.Background(), v2.NexusPHPRequest{Path: "/torrents.php", Method: "GET"})
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	// 表头 div 不带 torrent-table-sub-info，必须被排除，只剩 5 个真实行
	require.Len(t, items, 5, "header row must be excluded, only real torrent rows parsed")

	free := items[0]
	assert.Equal(t, "99001", free.ID)
	// 标题锚点内嵌 <span class='new'>[新]</span>；ParseSearch 走 Text() 且无 Filters 钩子，
	// 无法用选择器剔除，因此角标会保留在标题里（详情页走隐藏域 value，标题干净）。
	assert.Equal(t, "Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE[新]", free.Title)
	assert.Equal(t, "虚构副标题一 | 第01-04集 | 4K 60帧", free.Subtitle)
	assert.Equal(t, v2.DiscountFree, free.DiscountLevel)
	assert.Equal(t, int64(18607945809), free.SizeBytes)
	assert.Equal(t, 1, free.Seeders)
	assert.Equal(t, 119, free.Leechers)
	assert.Equal(t, 0, free.Snatched)
	// 促销结束时间必须取「剩余时间」内的 span[title]，而不是 torrent-info-text-added 的发布时间
	require.False(t, free.DiscountEndTime.IsZero(), "discount end time should be parsed")
	assert.Equal(t, "2026-07-27 22:05:11", free.DiscountEndTime.Format("2006-01-02 15:04:05"))
	assert.Equal(t, int64(1785161111), free.UploadedAt)
	// 分类图标 alt 恒为「类型」，未配置 Category，避免写入无意义取值
	assert.Empty(t, free.Category)

	twoXFree := items[1]
	assert.Equal(t, "99002", twoXFree.ID)
	// 无 [新] 角标时标题干净
	assert.Equal(t, "Fixture.Movie.2026.1080p.BluRay.x264-FIXTURE", twoXFree.Title)
	// promotion-tag-2xfree 不能被 promotion-tag-free 抢先命中（两者互不为子串）
	assert.Equal(t, v2.Discount2xFree, twoXFree.DiscountLevel)
	assert.Equal(t, int64(855952588), twoXFree.SizeBytes)
	assert.Equal(t, 206, twoXFree.Seeders)
	assert.Equal(t, 3, twoXFree.Leechers)
	assert.Equal(t, 412, twoXFree.Snatched)

	half := items[2]
	assert.Equal(t, "99003", half.ID)
	assert.Equal(t, v2.DiscountPercent50, half.DiscountLevel)
	assert.Equal(t, int64(1331439861), half.SizeBytes)

	third := items[3]
	assert.Equal(t, "99004", third.ID)
	assert.Equal(t, v2.DiscountPercent30, third.DiscountLevel)
	assert.Equal(t, int64(45956150067), third.SizeBytes)

	none := items[4]
	assert.Equal(t, "99005", none.ID)
	assert.Equal(t, v2.DiscountNone, none.DiscountLevel)
	assert.True(t, none.DiscountEndTime.IsZero(), "no promotion -> no end time")
	assert.Equal(t, int64(6023691632), none.SizeBytes)
	assert.Equal(t, 77, none.Seeders)
	assert.Equal(t, 901, none.Snatched)
	assert.Positive(t, none.UploadedAt)
}

// --- Suite: Detail ---

func testHhanclubDetail(t *testing.T) {
	def := getHhanclubDef(t)

	t.Run("Free", func(t *testing.T) {
		doc := FixtureDoc(t, "hhanclub_detail", hhanclubDetailFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99001", info.TorrentID)
		// 隐藏域 value，不含列表页的 [新] 角标
		assert.Equal(t, "Fixture.Show.S01.2026.2160p.WEB-DL.H.265-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountFree, info.DiscountLevel)
		require.False(t, info.DiscountEnd.IsZero(), "discount end time should be parsed")
		assert.Equal(t, "2026-07-27 22:05:11", info.DiscountEnd.Format("2006-01-02 15:04:05"))
		// 「大小：」外层 span 的 .Next() 兄弟为 "17.33 GB"，按 1024 换算
		assert.InDelta(t, 17.33*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("NoPromotionBinaryUnit", func(t *testing.T) {
		doc := FixtureDoc(t, "hhanclub_detail_gib", hhanclubDetailGiBFixture)
		info := v2.NewNexusPHPParserFromDefinition(def).ParseAll(doc.Selection)

		assert.Equal(t, "99006", info.TorrentID)
		assert.Equal(t, "Fixture.NoPromo.2026.1080p.Remux-FIXTURE", info.Title)
		assert.Equal(t, v2.DiscountNone, info.DiscountLevel)
		assert.True(t, info.DiscountEnd.IsZero())
		assert.InDelta(t, 4.32*1024, info.SizeMB, 0.1)
		assert.False(t, info.HasHR)
	})

	t.Run("DownloadLinkAndSubtitle", func(t *testing.T) {
		doc := FixtureDoc(t, "hhanclub_detail", hhanclubDetailFixture)
		// newTestNexusPHPDriver 不注入 Selectors，DetailSubtitle 会退回默认的 td.rowhead 选择器，
		// 本站没有 table，必须显式传入站点选择器。
		driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
			BaseURL:   def.URLs[0],
			Cookie:    "test_cookie=1",
			Selectors: def.Selectors,
		})
		driver.SetSiteDefinition(def)
		detail, err := driver.ParseDetail(v2.NexusPHPResponse{Document: doc, StatusCode: http.StatusOK})
		require.NoError(t, err)
		assert.Equal(t, "download.php?id=99001", detail.DownloadURL)
		// 详情页副标题是相邻兄弟 div，无 td.rowhead 可用
		assert.Equal(t, "虚构副标题一 | 第01-04集 | 4K 60帧", detail.Subtitle)
	})
}

// --- Suite: UserInfo ---

func testHhanclubUserInfo(t *testing.T) {
	def := getHhanclubDef(t)
	driver := newTestNexusPHPDriver(def)

	t.Run("SidebarOnIndexPage", func(t *testing.T) {
		doc := FixtureDoc(t, "hhanclub_index", hhanclubIndexFixture)
		fields := map[string]string{
			"id":       "90001",
			"name":     "fixture_user",
			"seeding":  "140",
			"leeching": "0",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}
	})

	t.Run("UserdetailsPage", func(t *testing.T) {
		doc := FixtureDoc(t, "hhanclub_userdetails", hhanclubUserdetailsFixture)
		fields := map[string]string{
			// 12.755 TB / 2.025 TB 按 1024 进制换算为字节；
			// 「实际上传量/实际下载量」不得被误命中
			"uploaded":   "14024270812282",
			"downloaded": "2226511046246",
			// 官方分享率取 <font> 内的 6.300，而非括号里的实际分享率 0.785
			"ratio":        "6.3",
			"levelName":    "Crazy User(虚构等级)",
			"bonus":        "1.0688577e+06",
			"seedingBonus": "445418",
			// parseTime 按站点 TimezoneOffset(+0800) 解析
			"joinTime":     "1755614426",
			"lastAccessAt": "1785134606",
		}
		for field, expected := range fields {
			t.Run(field, func(t *testing.T) {
				sel, ok := def.UserInfo.Selectors[field]
				require.True(t, ok, "selector %q not found", field)
				assert.Equal(t, expected, driver.ExtractFieldValuePublic(doc, sel))
			})
		}

		// 保号探测只读取 UserInfo.LastAccess，必须为正的 Unix 秒
		got := driver.ExtractFieldValuePublic(doc, def.UserInfo.Selectors["lastAccessAt"])
		require.NotEmpty(t, got, "lastAccessAt must be parsed for the login probe")
		assert.Regexp(t, `^\d+$`, got)
		assert.Greater(t, got, "0")
	})
}

// --- Standalone Tests ---

func TestHhanclub_Fixtures_NoSecrets(t *testing.T) {
	fixtures := map[string]string{
		"search_header":   hhanclubSearchHeaderFixture,
		"promotion_style": hhanclubPromotionStyleFixture,
		"search":          hhanclubSearchFixture,
		"detail":          hhanclubDetailFixture,
		"detail_gib":      hhanclubDetailGiBFixture,
		"index":           hhanclubIndexFixture,
		"userdetails":     hhanclubUserdetailsFixture,
	}
	for name, data := range fixtures {
		t.Run(name, func(t *testing.T) {
			RequireNoSecrets(t, name, data)
		})
	}
}
