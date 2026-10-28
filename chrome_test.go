// [INPUT]: 依赖 loadProfilesFrom 读取任意根目录下的 Chrome Local State。
// [OUTPUT]: 保护 Local State 解析的既有契约：缺失、损坏、空条目必须报错，目录不存在的条目被跳过，Default 优先且名称有回落。
// [POS]: Profile 发现的行为边界回归，与平台路径无关。
// [PROTOCOL]: 变更时更新此头部，然后检查 AGENTS.md
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLocalState(t *testing.T, base, body string) {
	t.Helper()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "Local State"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProfilesFromRejectsMissingCorruptAndEmpty(t *testing.T) {
	base := t.TempDir()
	if _, err := loadProfilesFrom(base); err == nil {
		t.Fatal("missing Local State must not look like an empty profile list")
	}

	writeLocalState(t, base, "{not json")
	if _, err := loadProfilesFrom(base); err == nil {
		t.Fatal("a corrupt Local State must be reported")
	}

	writeLocalState(t, base, `{"profile":{"info_cache":{}}}`)
	if _, err := loadProfilesFrom(base); err == nil {
		t.Fatal("no profile entry must be reported")
	}
}

func TestLoadProfilesFromSkipsMissingDirsAndKeepsDefaultFirst(t *testing.T) {
	base := t.TempDir()
	writeLocalState(t, base, `{"profile":{"info_cache":{
		"Profile 1":{"name":"工作","user_name":"work@example.test"},
		"Default":{"name":"个人","user_name":"me@example.test"},
		"Profile 2":{"user_name":"gone@example.test"}}}}`)
	for _, dir := range []string{"Default", "Profile 1"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	list, err := loadProfilesFrom(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("profiles = %d, want 2: entries whose directory is gone are skipped", len(list))
	}
	if list[0].Dir != "Default" || list[1].Dir != "Profile 1" {
		t.Fatalf("order = [%s %s], want [Default Profile 1]", list[0].Dir, list[1].Dir)
	}
	if list[1].Name != "工作" || list[1].Email != "work@example.test" {
		t.Fatalf("profile 1 = %+v, want its name and email from Local State", list[1])
	}
}

func TestLoadProfilesFromFallsBackToDirName(t *testing.T) {
	base := t.TempDir()
	writeLocalState(t, base, `{"profile":{"info_cache":{"Profile 5":{}}}}`)
	if err := os.MkdirAll(filepath.Join(base, "Profile 5"), 0o755); err != nil {
		t.Fatal(err)
	}

	list, err := loadProfilesFrom(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Profile 5" || list[0].Email != "" {
		t.Fatalf("profile = %+v, want the directory name as its name and no email", list)
	}
}
