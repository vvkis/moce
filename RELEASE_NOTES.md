# 墨册 v1.1.1

## 主要更新

### AZW3（Kindle 原生 KF8）兼容性彻底修复
- 按 Calibre `writer8` 规范完善 AZW3 生成：4096 字节记录对齐、TBS 索引、标准 `P-//*[@aid='...']` Chunk 选择器与广度优先 NCX 目录索引。
- 补齐 `extra_data_flags=1` 多字节尾标（同步修正 `reflow/mobi` 尾标位序）以及 EXTH 116 / 528 与 8192B 头部填充。
- 封面与缩略图自动规范化为 JFIF JPEG，修复 Kindle 原生系统打开报错与书架封面不显示的问题。

### 默认排版间距对齐 Calibre
- 行距与页边距默认改为 `0`（不写死样式，交给 Kindle / 阅读器原生排版并支持实时调节，避免双重留白）。
- 段间距默认调整为 `1em`（同段紧密、段间分明），首行缩进保持 `2em`，各项仍可在「高级正则与版式」中自定义。

### 文档与界面截图更新
- 同步更新全部新版现代工作台界面截图与 README 说明。

---

章节识别、刮搜书源、排版 CSS 和 Cookie 登录校验来自 [moon（月读）](https://github.com/AnkioTomas/moon)。感谢作者 AnkioTomas。
