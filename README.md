<p align="center">
  <img src="docs/icon.png" width="128" alt="墨册">
</p>

<h1 align="center">墨册 · Moce</h1>

<p align="center">在自己的电脑上，把 TXT、EPUB、MOBI、AZW3 重新排成给 KOReader 看的 EPUB。</p>

<p align="center">
  <img src="https://img.shields.io/badge/License-GPLv3-blue.svg" alt="License GPLv3">
  <img src="https://img.shields.io/github/v/release/vvkis/moce" alt="Release">
  <img src="https://img.shields.io/badge/platform-Windows-blue" alt="平台 Windows">
</p>

<p align="center">
  <a href="#它能做什么">它能做什么</a> ·
  <a href="#安装">安装</a> ·
  <a href="#截图">截图</a> ·
  <a href="#给开发者">给开发者</a> ·
  <a href="#致谢">致谢</a> ·
  <a href="#许可证">许可证</a>
</p>

墨册是一个 Windows 桌面程序。书留在本机，不上传，也不调用 Calibre。当前版本 **v1.0.0**。

## 它能做什么

| | |
|---|---|
| 重新排版 | TXT、EPUB、MOBI、AZW3 抽出正文，再做成一本新的 EPUB。原文件不动 |
| 章节 | 移植自 moon 的目录识别：卷章层级、重复卷号、日期前缀、番外 |
| 刮搜 | 豆瓣、微信读书、七猫、晋江、起点、番茄、长佩、潇湘。只在你点「刮搜」时联网 |
| 版式 | 与 moon「重新排版」相同的正文样式：页边距 5%，行距 1.8，段距 0.45em，首行缩进 2em，标题用 h1 |
| 进度 | 后台队列，排队、耗时、取消和重试。排版完成的提示留在窗口里，直到你关掉 |
| 账号 | 登录信息写在程序旁边的 `data\accounts`，默认明文，便于整夹搬走。可选仅本机 Windows 用户可解密 |
| 便携 | 数据、缓存、界面运行库都在 exe 旁边。不写 C 盘，不写注册表，没有安装包 |

## 安装

需要系统里已有的 [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/)。Windows 11 一般已经带了。程序不附带 WebView2 安装包。

1. 从 [Releases](https://github.com/vvkis/moce/releases/latest) 下载 `墨册-portable.zip`（v1.0.0）。
2. 解压整个「墨册」文件夹。不要只拿出里面的 exe。
3. 双击 `墨册.exe`。

没有代码签名。Windows 如果提示未知发布者，选择仍要运行。

文件夹里有这些：

| 路径 | 做什么 |
|---|---|
| `墨册.exe` | 程序 |
| `data\settings.json` | 第一次打开时生成，记住输出目录和账号存储方式 |
| `data\accounts\` | 刮搜登录 |
| `data\jobs.json` | 进度历史 |
| `cache\webview2` | 界面运行数据 |
| `cache\刮搜` | 刮搜封面和短时网页缓存。没选中的封面会清掉，目录超过 64MB 从最旧的开始删 |
| `fonts` | 可选。放进 `.ttf` / `.otf` / `.ttc` 后，生成封面时优先用这里的字体 |

把整个文件夹挪到别的盘，这些数据会跟着走。

## 截图

<p>
  <img src="docs/screenshots/main.png" width="720" alt="主界面">
</p>
<p>
  <img src="docs/screenshots/scrape.png" width="720" alt="刮搜">
</p>
<p>
  <img src="docs/screenshots/progress.png" width="720" alt="进度">
</p>
<p>
  <img src="docs/screenshots/accounts.png" width="720" alt="账号">
</p>
<p>
  <img src="docs/screenshots/done.png" width="720" alt="排版完成">
</p>

界面里的《河灯》是程序自带的示例，不是已出版的书。刮搜截图用的是虚构结果，没有真实封面。

## 给开发者

Go + Wails v2，界面是 TypeScript。测试不依赖 Wails：

```bash
go test ./...
go vet ./...
```

交叉编译 Windows 程序：

```bash
bash scripts/build-windows.sh
```

Windows 上：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\release.ps1
```

产物是 `dist/墨册.exe` 和 `dist/墨册-portable.zip`。WebView2 用系统里已经安装的运行库。

只看界面：

```bash
cd frontend && pnpm install && pnpm run build && cd ..
go run ./cmd/devserver -addr 127.0.0.1:47321
```

浏览器打开 <http://127.0.0.1:47321>。这个预览只监听本机，发布包里没有它。

示例文本在 `samples/示例小说.txt`。

## 致谢

章节识别、刮搜书源、排版 CSS 和 Cookie 登录校验来自 [moon（月读）](https://github.com/AnkioTomas/moon)。谢谢作者 [AnkioTomas](https://github.com/AnkioTomas) 把这些逻辑以 GNU GPLv3 公开。墨册是独立的 Windows 程序，不是 KOReader 插件。

也感谢 [KOReader](https://koreader.rocks/) 和它使用的 crengine、[Wails](https://wails.io/)、[epubcheck](https://github.com/w3c/epubcheck)。界面标题用了 Noto Serif SC 的子集，字体许可见 `frontend/src/assets/FONT-NOTICE.txt`。

## 许可证

[GNU GPLv3](LICENSE)。使用、修改和再分发都要保留这份许可，并提供对应的源代码。
