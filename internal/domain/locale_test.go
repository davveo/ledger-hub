package domain

import "testing"

func TestLangFrom(t *testing.T) {
	if got := LangFrom("en-US,en;q=0.9"); got != "en" {
		t.Fatalf("accept-language en got %s", got)
	}
	if got := LangFrom("zh-CN"); got != "zh" {
		t.Fatalf("zh-CN got %s", got)
	}
	if got := LangFrom("", "", "en"); got != "en" {
		t.Fatalf("fallback header got %s", got)
	}
	if got := LangFrom(); got != "zh" {
		t.Fatalf("default got %s", got)
	}
}

func TestLocalizeKeyed(t *testing.T) {
	err := Keyed(CodeAmountNotInteger, KeyAmountNotInteger)
	if zh := Localize("zh", err); zh != "金额必须为最小单位整数" {
		t.Fatalf("zh %s", zh)
	}
	if en := Localize("en", err); en != "amount must be a min-unit integer" {
		t.Fatalf("en %s", en)
	}
	fail := Keyed(CodeSagaFailed, KeySagaFailed, "boom")
	if got := Localize("en", fail); got != "cross-shard transfer failed: boom" {
		t.Fatalf("args %s", got)
	}
}

func TestHTTPStatus(t *testing.T) {
	if Keyed(CodeReplay, KeyReplay).HTTPStatus() != 401 {
		t.Fatal("replay")
	}
	if Keyed(CodeNotReady, KeyNotReady).HTTPStatus() != 503 {
		t.Fatal("not ready")
	}
	if ErrNotFound.HTTPStatus() != 404 {
		t.Fatal("not found")
	}
}
