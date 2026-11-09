package handlers

import (
	"testing"
	"time"
)

// siteOpenStatus 按 Asia/Shanghai 自然日判定开放日期；边界必须精确到零点。
func TestSiteOpenStatus(t *testing.T) {
	cst := siteTimeZone
	cases := []struct {
		name     string
		openDate string
		now      time.Time
		wantOpen bool
	}{
		{"未设置", "", time.Date(2026, 11, 1, 0, 0, 0, 0, cst), true},
		{"空白", "   ", time.Date(2026, 11, 1, 0, 0, 0, 0, cst), true},
		{"非法值不限制", "2026/11/22", time.Date(2026, 11, 1, 0, 0, 0, 0, cst), true},
		{"开放日前一天最后一秒", "2026-11-22", time.Date(2026, 11, 21, 23, 59, 59, 0, cst), false},
		{"开放日零点", "2026-11-22", time.Date(2026, 11, 22, 0, 0, 0, 0, cst), true},
		{"开放日之后", "2026-11-22", time.Date(2026, 11, 23, 0, 0, 0, 0, cst), true},
		// UTC 2026-11-21T16:00 即北京时间 2026-11-22T00:00。
		{"UTC 对应北京时间开放日零点", "2026-11-22", time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC), true},
		{"UTC 对应北京时间开放日前一秒", "2026-11-22", time.Date(2026, 11, 21, 15, 59, 59, 0, time.UTC), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			open, dateLabel := siteOpenStatus(tc.openDate, tc.now)
			if open != tc.wantOpen {
				t.Fatalf("open = %v, want %v", open, tc.wantOpen)
			}
			if tc.wantOpen && dateLabel != "" {
				t.Fatalf("open dateLabel = %q, want empty", dateLabel)
			}
			if !tc.wantOpen && dateLabel != "2026 年 11 月 22 日" {
				t.Fatalf("dateLabel = %q, want open date", dateLabel)
			}
		})
	}
}

// renderNotOpenMessage：自定义文案为空时使用默认文案，{date} 替换为中文日期。
func TestRenderNotOpenMessage(t *testing.T) {
	const dateLabel = "2026 年 11 月 22 日"
	cases := []struct {
		name   string
		custom string
		want   string
	}{
		{
			"空串使用默认文案",
			"",
			"还没到打开的时候哦。等到 2026 年 11 月 22 日，这里就会为你打开，到时候再来输入口令吧。",
		},
		{
			"空白使用默认文案",
			"   ",
			"还没到打开的时候哦。等到 2026 年 11 月 22 日，这里就会为你打开，到时候再来输入口令吧。",
		},
		{
			"占位符替换",
			"先别急，{date} 再见。",
			"先别急，2026 年 11 月 22 日 再见。",
		},
		{
			"无占位符原样返回",
			"惊喜还在路上。",
			"惊喜还在路上。",
		},
		{
			"首尾空白裁剪",
			"  惊喜还在路上。  ",
			"惊喜还在路上。",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderNotOpenMessage(tc.custom, dateLabel); got != tc.want {
				t.Fatalf("got = %q, want %q", got, tc.want)
			}
		})
	}
}
