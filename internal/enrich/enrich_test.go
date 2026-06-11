package enrich

import "testing"

func TestParseTitleArray(t *testing.T) {
	// The requested {"titles":[...]} object.
	got, err := parseTitleArray(`{"titles":["英伟达超预期","苹果回购创纪录"]}`, 2)
	if err != nil || got[0] != "英伟达超预期" || got[1] != "苹果回购创纪录" {
		t.Fatalf("object: got=%v err=%v", got, err)
	}
	// Object wrapped in a Markdown code fence.
	got, err = parseTitleArray("```json\n{\"titles\":[\"特斯拉下调评级\"]}\n```", 1)
	if err != nil || got[0] != "特斯拉下调评级" {
		t.Fatalf("fenced object: got=%v err=%v", got, err)
	}
	// Bare array (older protocol / model ignored the object shape).
	got, err = parseTitleArray(`["甲","乙"]`, 2)
	if err != nil || got[1] != "乙" {
		t.Fatalf("bare array: got=%v err=%v", got, err)
	}
	// Array embedded in prose → sliced out.
	got, err = parseTitleArray("好的:\n[\"丙\"]\n以上。", 1)
	if err != nil || got[0] != "丙" {
		t.Fatalf("prose-wrapped: got=%v err=%v", got, err)
	}
	// Length mismatch → error (never write misaligned translations).
	if _, err = parseTitleArray(`{"titles":["只有一条"]}`, 2); err == nil {
		t.Fatal("length mismatch should error")
	}
	// Garbage → error.
	if _, err = parseTitleArray(`抱歉我无法完成`, 1); err == nil {
		t.Fatal("non-JSON should error")
	}
}
