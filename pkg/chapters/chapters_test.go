package chapters

import (
	"strings"
	"testing"
)

func opts() Options {
	return Options{MergeBrokenLines: true, DropEmptyLines: true, FallbackRunes: 8000}
}

func titles(nodes []Node) []string {
	var out []string
	var walk func([]Node, string)
	walk = func(ns []Node, prefix string) {
		for _, n := range ns {
			out = append(out, prefix+n.Title)
			walk(n.Children, prefix+"  ")
		}
	}
	walk(nodes, "")
	return out
}

func TestChineseHeadings(t *testing.T) {
	text := strings.Join([]string{
		"序章",
		"　　很久以前。",
		"第一章 下山",
		"　　他出了村。",
		"第十二章 夜路",
		"　　月亮很高。",
		"第一百零八章 终",
		"　　到了。",
		"第1节 补记",
		"　　一节。",
		"第2回 旧事",
		"　　一回。",
		"第１２章 全角数字",
		"　　全角。",
		"楔子",
		"　　楔。",
		"番外 日常",
		"　　番。",
		"尾声",
		"　　尾。",
		"后记",
		"　　后。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Fallback {
		t.Fatal("unexpected fallback")
	}
	got := strings.Join(titles(res.Nodes), "\n")
	for _, want := range []string{"序章", "第一章 下山", "第十二章 夜路", "第一百零八章 终", "第1节 补记", "第2回 旧事", "第１２章 全角数字", "楔子", "番外 日常", "尾声", "后记"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in\n%s", want, got)
		}
	}
	if res.Nodes[1].Paragraphs[0] != "他出了村。" {
		t.Fatalf("paragraph %#v", res.Nodes[1].Paragraphs)
	}
}

func TestVolumeChapterTree(t *testing.T) {
	text := strings.Join([]string{
		"这是一段前言。",
		"",
		"第一卷 起源",
		"卷首的话写在这里。",
		"第一章 下山",
		"　　他走出了村子。",
		"路还很长。",
		"第二章 进城",
		"　　城里很热闹。",
		"第二卷 风云",
		"第三章 重逢",
		"　　多年以后。",
		"第一部 不应被当成章",
		"　　部被识别成分卷，因为后面还有章。",
		"第四章 继续",
		"　　继续走。",
		"后记",
		"　　写完了。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	want := strings.Join([]string{
		"前言",
		"第一卷 起源",
		"  第一章 下山",
		"  第二章 进城",
		"第二卷 风云",
		"  第三章 重逢",
		"第一部 不应被当成章",
		"  第四章 继续",
		"后记",
	}, "\n")
	if got != want {
		t.Fatalf("tree:\n%s\nwant:\n%s", got, want)
	}
	if res.VolumeCount != 3 {
		t.Fatalf("volumes %d", res.VolumeCount)
	}
	if res.Nodes[1].Paragraphs[0] != "卷首的话写在这里。" {
		t.Fatalf("intro %#v", res.Nodes[1].Paragraphs)
	}
	if strings.Join(res.Nodes[1].Children[0].Paragraphs, "|") != "他走出了村子。|路还很长。" {
		t.Fatalf("merged %#v", res.Nodes[1].Children[0].Paragraphs)
	}
}

func TestVolumeOnlyBecomesChapters(t *testing.T) {
	text := "第一部 上\n　　正文一。\n\n第二部 下\n　　正文二。\n"
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.VolumeCount != 0 {
		t.Fatalf("volumes %#v", titles(res.Nodes))
	}
	if len(res.Nodes) != 2 || res.Nodes[0].Title != "第一部 上" || res.Nodes[1].Title != "第二部 下" {
		t.Fatalf("%#v", titles(res.Nodes))
	}
}

func TestCustomRegex(t *testing.T) {
	text := "【第一章】\n　　甲。\n【第二章】\n　　乙。\n第一章 不会被默认规则收走\n　　丙。\n"
	opt := opts()
	opt.ChapterPattern = `【[^】]+】`
	res, err := Split(text, opt)
	if err != nil {
		t.Fatal(err)
	}
	got := titles(res.Nodes)
	if len(got) != 2 || got[0] != "【第一章】" || got[1] != "【第二章】" {
		t.Fatalf("%#v", got)
	}
	// The unmatched "第一章" line stays in the previous body because it is too
	// ordinary once the user pattern replaces the default. It is body text.
	if !strings.Contains(strings.Join(res.Nodes[1].Paragraphs, ""), "第一章") {
		t.Fatalf("%#v", res.Nodes[1].Paragraphs)
	}
}

func TestIgnoreProseAndLongLines(t *testing.T) {
	long := "第一章 " + strings.Repeat("长", 48)
	text := "他翻到第一章的时候笑了。\n这不是标题。\n" + long + "\n正文还在继续，没有章节标题。\n"
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fallback || len(res.Nodes) != 1 || res.Nodes[0].Title != "正文" {
		t.Fatalf("fallback %+v titles %#v", res.Fallback, titles(res.Nodes))
	}
}

func TestFallbackBySize(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteString(strings.Repeat("字", 10))
		b.WriteString("\n\n")
	}
	opt := opts()
	opt.FallbackRunes = 25
	res, err := Split(b.String(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fallback || len(res.Nodes) < 2 {
		t.Fatalf("nodes %d fallback %v", len(res.Nodes), res.Fallback)
	}
	if res.Nodes[0].Title != "第1部分" || res.Nodes[1].Title != "第2部分" {
		t.Fatalf("%#v", titles(res.Nodes))
	}
}

func TestInvalidRegex(t *testing.T) {
	opt := opts()
	opt.ChapterPattern = "("
	if _, err := Split("第一章\n正文", opt); err == nil || !strings.Contains(err.Error(), "章节正则") {
		t.Fatal(err)
	}
}

func TestVolumeClosersStayOutOfTOC(t *testing.T) {
	text := strings.Join([]string{
		"第五卷 风起",
		"第一章 甲",
		"　　正文甲。",
		"第二章 乙",
		"　　正文乙。",
		"第五卷 完",
		"第六卷 云",
		"第三章 丙",
		"　　正文丙。",
		"第五卷完",
		"本卷完",
		"终章",
		"　　结束了。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	want := strings.Join([]string{
		"第五卷 风起",
		"  第一章 甲",
		"  第二章 乙",
		"第六卷 云",
		"  第三章 丙",
		"终章",
	}, "\n")
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(strings.Join(res.Nodes[0].Children[1].Paragraphs, ""), "第五卷 完") {
		t.Fatalf("closer missing from previous chapter: %#v", res.Nodes[0].Children[1].Paragraphs)
	}
	joined := strings.Join(res.Nodes[1].Children[0].Paragraphs, "")
	if !strings.Contains(joined, "第五卷完") || !strings.Contains(joined, "本卷完") {
		t.Fatalf("tail closers missing: %#v", res.Nodes[1].Children[0].Paragraphs)
	}
	if res.Nodes[2].Title != "终章" {
		t.Fatalf("终章 %#v", titles(res.Nodes))
	}
}

func TestCloserShapes(t *testing.T) {
	for _, title := range []string{"第五卷 完", "第五卷完", "第五卷 终", "第5卷完", "卷一完", "本卷完", "本卷终", "第一部 完", "（第五卷完）", "第五卷完。"} {
		if !isVolumeCloser(title) {
			t.Fatalf("%s should be a closer", title)
		}
	}
	for _, title := range []string{"第五卷 风起", "第五卷 完结篇", "终章", "大结局", "第一章 完"} {
		if isVolumeCloser(title) {
			t.Fatalf("%s should stay a heading", title)
		}
	}
	text := "第五卷 完结篇\n第一章 甲\n　　还有正文。\n"
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	if got != "第五卷 完结篇\n  第一章 甲" {
		t.Fatalf("%s", got)
	}
}

func TestEmptyVolumeDoesNotStandAlone(t *testing.T) {
	text := "第一卷\n\n第二卷 风起\n第一章 甲\n　　正文。\n"
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	if strings.Contains(got, "第一卷\n") || strings.HasPrefix(got, "第一卷") && !strings.Contains(got, "第二卷") {
		t.Fatalf("empty volume stood alone:\n%s", got)
	}
	if !strings.Contains(got, "第二卷 风起") || !strings.Contains(got, "第一章 甲") {
		t.Fatalf("%s", got)
	}
}

func TestWindowsNewlines(t *testing.T) {
	res, err := Split("第一章 甲\r\n　　正文。\r\n", opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Nodes[0].Title != "第一章 甲" || res.Nodes[0].Paragraphs[0] != "正文。" {
		t.Fatalf("%+v", res.Nodes[0])
	}
}

func TestChapterTitleShapes(t *testing.T) {
	yes := []string{
		"第十二卷 风起",
		"序章",
		"最终章 归途",
		"Chapter IV - Return",
		"Section 12: Return",
		"===第001章 纸灯===",
		"第一部 风起",
		"第1章 纸灯入水！",
		"第3章 灯还在吗？",
		"第１章 全角",
		"8.11第3章 灯还在吗？",
		"2023.8.11第3章 标题",
		"11-08第3章 标题",
		"（8.11）第3章 标题",
		"番外：日常",
		"完本感言",
	}
	for _, line := range yes {
		if _, ok := chapterTitle(line); !ok {
			t.Fatalf("should be a heading: %s", line)
		}
	}
	if got, _ := chapterTitle("===第001章 开始==="); got != "第001章 开始" {
		t.Fatalf("equals wrapper: %q", got)
	}
	if got, _ := chapterTitle("8.11第3章 灯还在吗？"); got != "第3章 灯还在吗？" {
		t.Fatalf("date prefix kept: %q", got)
	}
	for _, line := range []string{
		"第一部门负责审批",
		"第一部队出发了",
		"第二天傍晚，我来到夜总会",
		"这只是普通正文",
		"他在第3章里写了一句特别特别特别特别特别特别特别特别特别特别特别特别特别特别特别特别长的话，这不是标题。",
	} {
		if _, ok := chapterTitle(line); ok {
			t.Fatalf("should not be a heading: %s", line)
		}
	}
}

func TestEqualsWrapperBecomesChapter(t *testing.T) {
	res, err := Split("===第001章 开始===\n正文\n第二天继续写正文", opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 1 || res.Nodes[0].Title != "第001章 开始" {
		t.Fatalf("%#v", titles(res.Nodes))
	}
	body := strings.Join(res.Nodes[0].Paragraphs, "")
	if !strings.Contains(body, "第二天继续写正文") {
		t.Fatalf("%#v", res.Nodes[0].Paragraphs)
	}
}

func TestPunctuationDateAndDuplicateChapters(t *testing.T) {
	text := strings.Join([]string{
		"第1章 纸灯入水！",
		"　　他把灯放进河里。",
		"　　第二行还是正文。",
		"8.11第3章 灯还在吗？",
		"　　日期前缀不该留在目录里。",
		"　　另一行。",
		"第1章 又是开头",
		"　　重复的章号各自成章。",
		"　　不要并掉。",
		"第51章 甲！",
		"　　五十一的正文很长，不能被卷末规则吃掉。",
		"　　第二段。",
		"第52章 乙？",
		"　　五十二。",
		"　　还有。",
		"（8.11）第56章 丙",
		"　　五十六。",
		"　　还有。",
		"第59章 丁……",
		"　　五十九。",
		"　　还有。",
		"第61章 戊",
		"　　六十一。",
		"　　还有。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	want := []string{
		"第1章 纸灯入水！",
		"第3章 灯还在吗？",
		"第1章 又是开头",
		"第51章 甲！",
		"第52章 乙？",
		"第56章 丙",
		"第59章 丁……",
		"第61章 戊",
	}
	for _, title := range want {
		if !strings.Contains(got, title) {
			t.Fatalf("missing %s in\n%s", title, got)
		}
	}
	if strings.Contains(got, "8.11") {
		t.Fatalf("date leaked into TOC:\n%s", got)
	}
	if len(res.Nodes) != len(want) {
		t.Fatalf("count %d\n%s", len(res.Nodes), got)
	}
}

func TestCombinedVolumeChapterCreatesParent(t *testing.T) {
	text := strings.Join([]string{
		"第一卷-第一章 标题",
		"　　正文。",
		"　　还有一段。",
		"第二章 继续",
		"　　下一段。",
		"　　再一段。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	want := "第一卷\n  第一章 标题\n  第二章 继续"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRepeatedVolumeDoesNotRestart(t *testing.T) {
	text := strings.Join([]string{
		"第五卷 上游夜航",
		"第50章 甲",
		"　　这一章正文足够长，不该被卷末规则收走。",
		"　　第二行也是正文。",
		"第六卷：下游未至，敬请期待……",
		"第六卷 下游芦苇",
		"第一章 重启",
		"　　新的一卷从第一章开始，正文足够长。",
		"　　继续写。",
		"第七卷 回水湾",
		"第53章 灯芯还在",
		"　　正常正文一行。",
		"　　正常正文二行。",
		"第七卷：回水湾",
		"纸灯顺水而下，字迹还在（完）",
		"第八卷 旧码头",
		"第1章 开始",
		"　　正文。",
		"　　还有。",
		"第八卷：旧码头",
		"番外",
		"第一章 早安",
		"　　番外正文。",
		"　　第二段。",
		"第二章 灯芯",
		"　　正文。",
		"　　还有。",
		"第三章 终",
		"　　正文。",
		"　　还有。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "\n")
	want := strings.Join([]string{
		"第五卷 上游夜航",
		"  第50章 甲",
		"第六卷 下游芦苇",
		"  第一章 重启",
		"第七卷 回水湾",
		"  第53章 灯芯还在",
		"第八卷 旧码头",
		"  第1章 开始",
		"番外",
		"  第一章 早安",
		"  第二章 灯芯",
		"  第三章 终",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, banned := range []string{"敬请期待", "第七卷：", "第八卷："} {
		if strings.Contains(got, banned) {
			t.Fatalf("%s still in TOC:\n%s", banned, got)
		}
	}
	body := strings.Join(res.Nodes[2].Children[0].Paragraphs, "")
	if !strings.Contains(body, "（完）") || !strings.Contains(body, "第七卷：回水湾") {
		t.Fatalf("recap was dropped: %#v", res.Nodes[2].Children[0].Paragraphs)
	}
	teaser := strings.Join(res.Nodes[0].Children[0].Paragraphs, "")
	if !strings.Contains(teaser, "敬请期待") {
		t.Fatalf("teaser was dropped: %#v", res.Nodes[0].Children[0].Paragraphs)
	}
}

func TestPrintedTocPageFolds(t *testing.T) {
	text := strings.Join([]string{
		"第一章",
		"甲。",
		"第二章",
		"第三章",
		"第四章",
		"第五章",
		"乙。",
	}, "\n")
	res, err := Split(text, opts())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(titles(res.Nodes), "|")
	if got != "第一章|第五章" {
		t.Fatalf("%s", got)
	}
	body := strings.Join(res.Nodes[0].Paragraphs, "")
	if !strings.Contains(body, "第二章") || !strings.Contains(body, "第四章") {
		t.Fatalf("listing dropped: %#v", res.Nodes[0].Paragraphs)
	}
}
