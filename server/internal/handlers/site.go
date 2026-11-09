package handlers

import (
	"context"
	"strings"
	"time"
)

// 站点开放日期与未开放提示语：管理端设置 open_date 后，日期之前的前台不可解锁。
// 自然日按 Asia/Shanghai 判定（项目约定展示时区）；中国自 1991 年起无夏令时，
// 用固定 +08:00 避免容器缺少 tzdata 时 LoadLocation 失败。
var siteTimeZone = time.FixedZone("CST", 8*60*60)

// defaultNotOpenMessage 是未配置自定义提示语时的默认文案；{date} 替换为开放日期。
const defaultNotOpenMessage = "还没到打开的时候哦。等到 {date}，这里就会为你打开，到时候再来输入口令吧。"

// siteOpenStatus 判断当前是否已到开放日期；openDate 为空或非法时视为不限制。
// 未开放时返回中文日期标签，供提示语中的 {date} 占位符使用。
func siteOpenStatus(openDate string, now time.Time) (open bool, dateLabel string) {
	openDate = strings.TrimSpace(openDate)
	if openDate == "" {
		return true, ""
	}
	openAt, err := time.ParseInLocation("2006-01-02", openDate, siteTimeZone)
	if err != nil {
		// 非法值不把整站锁死：管理端写入已校验，这里按未设置处理。
		return true, ""
	}
	if now.In(siteTimeZone).Before(openAt) {
		return false, openAt.Format("2006 年 1 月 2 日")
	}
	return true, ""
}

// renderNotOpenMessage 渲染未开放提示语：自定义文案为空时使用默认文案，
// {date} 统一替换为中文日期；文案不含占位符时原样返回。
func renderNotOpenMessage(custom, dateLabel string) string {
	tmpl := strings.TrimSpace(custom)
	if tmpl == "" {
		tmpl = defaultNotOpenMessage
	}
	return strings.ReplaceAll(tmpl, "{date}", dateLabel)
}

// siteOpen 读取站点开放日期与提示语设置，判断当前是否已开放。
func (h *Handlers) siteOpen(ctx context.Context) (open bool, msg string, err error) {
	openDate, gerr := h.st.GetSetting(ctx, "open_date")
	if gerr != nil {
		return true, "", gerr
	}
	open, dateLabel := siteOpenStatus(openDate, time.Now())
	if open {
		return true, "", nil
	}
	custom, merr := h.st.GetSetting(ctx, "not_open_message")
	if merr != nil {
		return true, "", merr
	}
	return false, renderNotOpenMessage(custom, dateLabel), nil
}
