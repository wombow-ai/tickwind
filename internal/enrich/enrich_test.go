package enrich

import "testing"

func TestParseIndexedTranslations(t *testing.T) {
	// The requested {"items":[{i,zh}]} object, in order.
	got, err := parseIndexedTranslations(`{"items":[{"i":0,"zh":"英伟达超预期"},{"i":1,"zh":"苹果回购创纪录"}]}`, 2)
	if err != nil || got[0] != "英伟达超预期" || got[1] != "苹果回购创纪录" {
		t.Fatalf("object: got=%v err=%v", got, err)
	}

	// Fenced + reordered: index decides position, not array order.
	got, err = parseIndexedTranslations("```json\n{\"items\":[{\"i\":1,\"zh\":\"乙\"},{\"i\":0,\"zh\":\"甲\"}]}\n```", 2)
	if err != nil || got[0] != "甲" || got[1] != "乙" {
		t.Fatalf("fenced/reordered: got=%v err=%v", got, err)
	}

	// Miscount: model dropped index 1 of 3 → present ones fill, missing stays
	// empty (retried next sweep), NEVER misaligns or discards the batch.
	got, err = parseIndexedTranslations(`{"items":[{"i":0,"zh":"甲"},{"i":2,"zh":"丙"}]}`, 3)
	if err != nil {
		t.Fatalf("miscount err: %v", err)
	}
	if got[0] != "甲" || got[1] != "" || got[2] != "丙" {
		t.Fatalf("miscount: got=%v, want [甲, '', 丙]", got)
	}

	// Bare array of items (model ignored the object wrapper).
	got, err = parseIndexedTranslations(`[{"i":0,"zh":"只此一条"}]`, 1)
	if err != nil || got[0] != "只此一条" {
		t.Fatalf("bare array: got=%v err=%v", got, err)
	}

	// Out-of-range index is ignored (no panic), others still applied.
	got, err = parseIndexedTranslations(`{"items":[{"i":9,"zh":"越界"},{"i":0,"zh":"有效"}]}`, 2)
	if err != nil || got[0] != "有效" || got[1] != "" {
		t.Fatalf("out-of-range: got=%v err=%v", got, err)
	}

	// Total garbage → error (sweep retries).
	if _, err = parseIndexedTranslations(`抱歉无法完成`, 2); err == nil {
		t.Fatal("non-JSON should error")
	}
}
