package diffmatchpatch

import (
	"strings"
	"testing"
)

// 验证 alignRuneStart 在 UTF-8 字符边界上正确退回。
func TestAlignRuneStart(t *testing.T) {
	// "abc中文def" — UTF-8: a=1 b=1 c=1 中=3(byte 3,4,5) 文=3(6,7,8) d=1 e=1 f=1
	s := "abc中文def"
	cases := []struct {
		in, want int
		desc     string
	}{
		{0, 0, "0 unchanged"},
		{1, 1, "after 'a' already aligned"},
		{3, 3, "start of 中 aligned"},
		{4, 3, "middle of 中 backs to start"},
		{5, 3, "end-middle of 中 backs to start"},
		{6, 6, "start of 文 aligned"},
		{7, 6, "middle of 文 backs to start"},
		{9, 9, "after 文 = start of d, aligned"},
		{12, 12, "end-of-string"},
		{-1, -1, "negative unchanged"},
		{999, 999, "past end unchanged"},
	}
	for _, c := range cases {
		got := alignRuneStart(s, c.in)
		if got != c.want {
			t.Errorf("alignRuneStart(%q, %d) = %d, want %d  (%s)", s, c.in, got, c.want, c.desc)
		}
	}
}

// 重现 sergi/go-diff issue #132 minimal repro: 多字节字符 + 短 anchor 在原版
// 触发 panic "slice bounds out of range"。fork 应安全返回 results (不 panic)。
func TestPatchApply_Issue132_NoPanic(t *testing.T) {
	dmp := New()
	patches, err := dmp.PatchFromText("@@ -1,2 +1,3 @@\n %E2%98%9E \n+r\n")
	if err != nil {
		t.Fatalf("PatchFromText: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic leaked: %v", r)
		}
	}()
	_, results := dmp.PatchApply(patches, "☞ 𝗢𝗥𝗗𝗘𝗥 ")
	// 不要求 apply 成功(语义可能 false),只要求不 panic 且返回非 nil results。
	if results == nil {
		t.Fatalf("results nil")
	}
}

// PatchSplitMax 把含 multi-byte 字符的大 patch 切段时,不应在 UTF-8 字符中间切。
// 原 sergi v1.4.0 line 419 byte slice 可能切出 invalid UTF-8;fork 切前对齐到
// rune boundary,保证每段 diffs.Text 都是合法 UTF-8。
func TestPatchSplitMax_NoInvalidUTF8(t *testing.T) {
	dmp := New()
	// 构造一个大于 MatchMaxBits (32) 的纯中文 base, 强制走 PatchSplitMax
	old := "中文段落一" + strings.Repeat("汉字", 30) + "结束"
	new := "中文段落一" + strings.Repeat("汉字", 30) + "结尾"
	patches := dmp.PatchMake(old, dmp.DiffMain(old, new, false))
	if len(patches) == 0 {
		t.Skip("no patches generated")
	}
	// PatchMake 出来的 patches 已是合法 UTF-8 (PatchAddContext 的 prefix/suffix
	// 切点已 align rune boundary)。
	for i, p := range patches {
		for j, d := range p.diffs {
			if !isValidUTF8(d.Text) {
				t.Errorf("PatchMake patch %d diff %d invalid UTF-8: %q", i, j, d.Text)
			}
		}
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PatchSplitMax panic: %v", r)
		}
	}()
	// PatchSplitMax 后每段也应保持 UTF-8 合法 (precontext/postcontext/diffText 切点已 align)。
	for i, p := range dmp.PatchSplitMax(patches) {
		for j, d := range p.diffs {
			if !isValidUTF8(d.Text) {
				t.Errorf("PatchSplitMax patch %d diff %d invalid UTF-8: %q  type=%d", i, j, d.Text, d.Type)
			}
		}
	}
}

// helper - 当前 sergi go.mod 用 go 1.13,utf8.ValidString 一直存在,但
// 测试代码避免直接 import (患者风格,跟 patch.go import 错位)。
func isValidUTF8(s string) bool {
	for i := 0; i < len(s); {
		r, sz := utf8DecodeFirst(s[i:])
		if r == 0xFFFD && sz == 1 {
			return false
		}
		i += sz
	}
	return true
}

// minimal rune decoder (避免再加 import,跟 patch.go 已有的 utf8 import 区分)
func utf8DecodeFirst(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xc0:
		return 0xFFFD, 1
	case b < 0xe0:
		if len(s) < 2 {
			return 0xFFFD, 1
		}
		return rune(b&0x1f)<<6 | rune(s[1]&0x3f), 2
	case b < 0xf0:
		if len(s) < 3 {
			return 0xFFFD, 1
		}
		return rune(b&0x0f)<<12 | rune(s[1]&0x3f)<<6 | rune(s[2]&0x3f), 3
	default:
		if len(s) < 4 {
			return 0xFFFD, 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3f)<<12 | rune(s[2]&0x3f)<<6 | rune(s[3]&0x3f), 4
	}
}
