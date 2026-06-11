package enrich

import "testing"

func TestParseTitleArray(t *testing.T) {
	// Plain JSON array.
	got, err := parseTitleArray(`["英伟达超预期","苹果回购创纪录"]`, 2)
	if err != nil || got[0] != "英伟达超预期" || got[1] != "苹果回购创纪录" {
		t.Fatalf("plain: got=%v err=%v", got, err)
	}
	// Fenced array (models love Markdown).
	got, err = parseTitleArray("```json\n[\"特斯拉下调评级\"]\n```", 1)
	if err != nil || got[0] != "特斯拉下调评级" {
		t.Fatalf("fenced: got=%v err=%v", got, err)
	}
	// Length mismatch → error (never write misaligned translations).
	if _, err = parseTitleArray(`["只有一条"]`, 2); err == nil {
		t.Fatal("length mismatch should error")
	}
	// Garbage → error.
	if _, err = parseTitleArray(`好的,翻译如下:`, 1); err == nil {
		t.Fatal("non-JSON should error")
	}
}
