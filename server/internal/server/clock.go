package server

import "time"

// 便于测试时替换时钟。
var timeNow = time.Now

func timeSince(t time.Time) time.Duration { return time.Since(t) }
