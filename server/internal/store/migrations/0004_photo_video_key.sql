-- 0004_photo_video_key.sql —— 实况照片（Live Photo）支持：
-- 一张照片可关联一段短时视频；video_key 为空表示普通静态照片。
-- 对象仍由服务端生成（trips/{tripID}/{yyyyMM}/{uuid}.{mp4|mov}），删除时与图片一并补偿清理。

ALTER TABLE photos ADD COLUMN video_key TEXT;
