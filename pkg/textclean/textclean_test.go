package textclean

import (
	"reflect"
	"testing"
)

func TestTrimAndMergeIndented(t *testing.T) {
	lines := []string{
		"　　第一段第一行",
		"第一段接着写。",
		"　　第二段。",
		"",
		"  第三段用两个半角空格开头。",
	}
	got := Paragraphs(lines, Options{MergeBrokenLines: true, DropEmptyLines: true})
	want := []string{
		"第一段第一行第一段接着写。",
		"第二段。",
		"第三段用两个半角空格开头。",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestPunctuationFallback(t *testing.T) {
	lines := []string{
		"这是一句",
		"话。",
		"这是第二句",
		"话。",
	}
	got := Paragraphs(lines, Options{MergeBrokenLines: true, DropEmptyLines: true})
	want := []string{"这是一句话。", "这是第二句话。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestNoMergeKeepsLines(t *testing.T) {
	lines := []string{"　　甲", "", "乙"}
	got := Paragraphs(lines, Options{MergeBrokenLines: false, DropEmptyLines: true})
	if !reflect.DeepEqual(got, []string{"甲", "乙"}) {
		t.Fatalf("drop %#v", got)
	}
	got = Paragraphs(lines, Options{MergeBrokenLines: false, DropEmptyLines: false})
	if !reflect.DeepEqual(got, []string{"甲", "", "乙"}) {
		t.Fatalf("keep %#v", got)
	}
}
