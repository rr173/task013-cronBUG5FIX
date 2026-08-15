package cron

import (
	"errors"
	"testing"
	"time"
)

func TestParseValid(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"每分钟", "* * * * *"},
		{"全星号带空格容错", "  *   *  *  *  * "},
		{"单值", "30 5 * * *"},
		{"区间", "0 0 1-5 * *"},
		{"步长", "*/15 * * * *"},
		{"区间步长", "2-10/3 * * * *"},
		{"列表混合", "0,15,30 0,12 1-5 * *"},
		{"星期日 0", "0 0 * * 0"},
		{"星期日 7", "0 0 * * 7"},
		{"二月 29", "0 0 29 2 *"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(c.expr); err != nil {
				t.Fatalf("Parse(%q) 出错: %v", c.expr, err)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"空", ""},
		{"四段", "* * * *"},
		{"六段", "* * * * * *"},
		{"分钟越界", "60 * * * *"},
		{"小时越界", "0 24 * * *"},
		{"日期越界", "0 0 32 * *"},
		{"月份越界", "0 0 * 13 *"},
		{"星期越界", "0 0 * * 8"},
		{"步长为 0", "*/0 * * * *"},
		{"区间步长为 0", "1-5/0 * * * *"},
		{"区间反向", "5-1 * * * *"},
		{"空段逗号", "0, * * * *"},
		{"首逗号", ",0 * * * *"},
		{"非法字符", "a * * * *"},
		{"步长为空", "*/ * * * *"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.expr)
			if err == nil {
				t.Fatalf("Parse(%q) 期望错误，实际为 nil", c.expr)
			}
		})
	}
}

func TestFieldValues(t *testing.T) {
	s, err := Parse("*/15 0 1-5 * 1,7")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fv := s.FieldValues()
	if got, want := fv["minute"], []int{0, 15, 30, 45}; !eq(got, want) {
		t.Errorf("minute = %v, want %v", got, want)
	}
	if got, want := fv["hour"], []int{0}; !eq(got, want) {
		t.Errorf("hour = %v, want %v", got, want)
	}
	if got, want := fv["dom"], []int{1, 2, 3, 4, 5}; !eq(got, want) {
		t.Errorf("dom = %v, want %v", got, want)
	}
	if got, want := fv["dow"], []int{0, 1}; !eq(got, want) {
		t.Errorf("dow = %v, want %v", got, want)
	}
}

func TestNextEveryMinute(t *testing.T) {
	s, _ := Parse("* * * * *")
	from := parse(t, "2026-01-15T10:00:00Z")
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := parse(t, "2026-01-15T10:01:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextSpecificTime(t *testing.T) {
	s, _ := Parse("30 5 * * *")
	from := parse(t, "2026-01-15T10:00:00Z")
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := parse(t, "2026-01-16T05:30:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextStrictlyAfter(t *testing.T) {
	// 10:00:00 恰好是触发点，严格晚于应跳到 10:15。
	s, _ := Parse("*/15 * * * *")
	from := parse(t, "2026-01-15T10:00:00Z")
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := parse(t, "2026-01-15T10:15:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextOrSemantic(t *testing.T) {
	// 0 0 13 * 5：每月 13 日或星期五 00:00。2026-01-10 为星期六，
	// 13 日（星期二）早于下一个星期五（1 月 16 日），故应命中 1 月 13 日。
	s, _ := Parse("0 0 13 * 5")
	from := parse(t, "2026-01-10T00:00:00Z")
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := parse(t, "2026-01-13T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextDomOnly(t *testing.T) {
	s, _ := Parse("0 0 13 * *")
	from := parse(t, "2026-01-10T00:00:00Z")
	got, _ := s.Next(from)
	want := parse(t, "2026-01-13T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextDowOnly(t *testing.T) {
	// 仅星期五：2026-01-10 为星期六，下一个星期五是 1 月 16 日。
	s, _ := Parse("0 0 * * 5")
	from := parse(t, "2026-01-10T00:00:00Z")
	got, _ := s.Next(from)
	want := parse(t, "2026-01-16T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextDow7IsSunday(t *testing.T) {
	// 0 0 * * 7 与 0 0 * * 0 等价，均命中星期日；2026-01-11 为星期日。
	s, _ := Parse("0 0 * * 7")
	from := parse(t, "2026-01-10T00:00:00Z")
	got, _ := s.Next(from)
	want := parse(t, "2026-01-11T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextImpossible(t *testing.T) {
	// 二月 30 日不存在，永远无可触发时刻。
	s, _ := Parse("0 0 30 2 *")
	from := parse(t, "2026-01-01T00:00:00Z")
	_, err := s.Next(from)
	if !errors.Is(err, ErrNoOccur) {
		t.Errorf("期望 ErrNoOccur，实际 %v", err)
	}
}

func TestNextLeapFeb29(t *testing.T) {
	// 0 0 29 2 *：2026/2027 非闰，2028 为闰年，下一个命中为 2028-02-29。
	s, _ := Parse("0 0 29 2 *")
	from := parse(t, "2026-01-01T00:00:00Z")
	got, err := s.Next(from)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := parse(t, "2028-02-29T00:00:00Z")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

func TestNextN(t *testing.T) {
	s, _ := Parse("*/15 * * * *")
	from := parse(t, "2026-01-15T10:00:00Z")
	got, err := s.NextN(from, 4)
	if err != nil {
		t.Fatalf("NextN: %v", err)
	}
	want := []string{
		"2026-01-15T10:15:00Z",
		"2026-01-15T10:30:00Z",
		"2026-01-15T10:45:00Z",
		"2026-01-15T11:00:00Z",
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if !got[i].Equal(parse(t, w)) {
			t.Errorf("times[%d] = %v, want %v", i, got[i], w)
		}
	}
}

func TestNextPreservesOffset(t *testing.T) {
	s, _ := Parse("30 5 * * *")
	from := parse(t, "2026-01-15T10:00:00+08:00")
	got, _ := s.Next(from)
	want := parse(t, "2026-01-16T05:30:00+08:00")
	if !got.Equal(want) {
		t.Errorf("Next = %v, want %v", got, want)
	}
	if got.Format(time.RFC3339) != "2026-01-16T05:30:00+08:00" {
		t.Errorf("RFC3339 = %s", got.Format(time.RFC3339))
	}
}

func eq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func parse(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("解析测试时间 %q: %v", s, err)
	}
	return tm
}
