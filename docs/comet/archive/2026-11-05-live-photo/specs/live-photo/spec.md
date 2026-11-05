# 实况照片（Live Photo）

## 能力说明

管理端上传照片时，系统在浏览器中自动识别实况照片的两种输入形式，拆分出静态图与实况视频后分别上传，并在 `photos` 记录上关联视频对象。公开照片墙与灯箱据此提供实况播放；没有视频的照片保持原有静态展示。

解析、拆分与类型嗅探都在浏览器完成，服务端只负责视频对象的上传签名、确认校验、签名读取与删除补偿。

依赖：`live-photo@^0.2.0`（`LivePhotoViewer` 播放、`extractFromLivePhoto` 拆分合并文件）。

## 输入识别

系统按下述顺序处理同一批次选择的文件：

1. 图像文件（jpeg/png/webp/heic/heif）与视频文件（mp4/mov）按“去掉扩展名后的文件名”分组，大小写不敏感；同名的图像 + 视频视为一对实况照片。
2. 没有同名视频的图像，如果是 jpeg/heic/heif，调用 `extractFromLivePhoto` 尝试拆分合并实况文件。
3. 拆分结果必须通过校验才算实况：静态图字节以 JPEG SOI（`FF D8 FF`）或 HEIC/HEIF `ftyp` 盒开头，视频字节以 `ftyp` 盒开头且在前 8KB 内包含 `moov` 或 `mdat`；任一不满足则丢弃拆分结果，按普通图片上传。
4. 没有匹配图像的视频不单独上传，只在上传任务区提示。

### Requirement: 成对输入

- **WHEN** 管理端同一批选择同名图像与视频（如 `IMG_0001.HEIC` + `IMG_0001.MOV`）
- **THEN** 只创建一条照片记录，图片对象与视频对象分别上传并关联到该记录
- **AND** 视频对象使用与图片相同的旅行前缀
- 验收：A1

### Requirement: 合并输入

- **WHEN** 管理端选择一个内嵌视频的合并实况文件
- **THEN** 浏览器拆出静态图与视频两个 Blob，分别按嗅探出的真实类型上传
- **AND** 只创建一条照片记录，并以 `video_key` 关联视频对象
- 验收：A2

### Requirement: 普通图片保持原行为

- **WHEN** 选择普通 JPEG/PNG/WebP 图片，且没有同名视频、拆分校验也未通过
- **THEN** 仍按单对象上传并创建一条无 `video_key` 的照片记录
- **AND** 前端不显示实况标识，不发起视频请求
- 验收：A3

### Requirement: 未合并 HEIC 不误判

- **WHEN** 选择一个未合并的单个 HEIC 图片
- **THEN** 拆分结果因静态图部分为空或校验失败被丢弃，按普通图片上传
- **AND** 不创建视频对象，不把原文件当视频保存
- 验收：A4

## 上传与降级

实况照片固定先上传图片、再上传视频。进度按两个对象的字节总数汇总。

### Requirement: 视频失败降级

- **WHEN** 图片上传成功但视频对象签名或直传失败
- **THEN** 确认请求不带 `video_key`，照片仍作为普通照片入库
- **AND** 上传任务区显示实况视频失败的原因
- 验收：A5

## 服务端数据与校验

`photos` 表新增可空列 `video_key`。上传签名接口接受 `video/mp4`（`.mp4`）与 `video/quicktime`（`.mov`）。确认接口新增可选字段 `video_key`。

### Requirement: video_key 校验

- **WHEN** 确认请求携带 `video_key`
- **THEN** 服务端校验该 key 属于 `trips/{tripID}/` 前缀、对象存在、大小 ≤ 30MB、HEAD 返回的内容类型在视频白名单内
- **AND** 任一条件不满足时返回 400，且不创建照片记录
- 验收：A6

### Requirement: 列表签名输出

- **WHEN** 照片带 `video_key` 且存储后端已启用
- **THEN** 照片列表响应包含短时效签名的 `video_src`
- **AND** 无视频照片的响应结构与改动前一致
- 验收：A1、A6

### Requirement: 删除补偿

- **WHEN** 删除一张带 `video_key` 的照片
- **THEN** 图片 key 与视频 key 都写入 `cos_delete_tasks`
- **AND** 无视频照片仍只清理图片 key
- 验收：A7

## 播放展示

### Requirement: 卡片播放

- **WHEN** 照片墙中的照片带有 `video_src`
- **THEN** 卡片使用 `LivePhotoViewer` 渲染，视频懒加载，桌面悬停或触屏长按播放，并显示实况徽标
- **AND** 无 `video_src` 的卡片保持现有缩略图与点击行为
- 验收：A8

### Requirement: 灯箱播放

- **WHEN** 在灯箱中切换到带 `video_src` 的照片
- **THEN** 使用 `LivePhotoViewer` 自动静音播放，图片与视频按 `object-fit: contain` 适配视口
- **AND** 切换或关闭灯箱时销毁播放器实例并释放资源
- **AND** 无视频照片仍使用现有 `<img>` 加载态、失败提示与相邻预取
- 验收：A8
