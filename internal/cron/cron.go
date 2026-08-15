// Package cron 实现标准五段式 cron 表达式的解析与下次触发时间计算。
//
// 仅依赖 Go 标准库。表达式为“分 时 日 月 周”五段，每段支持星号 *、单个数字、
// 闭区间 a-b、步长 a-b/n 与 */n，以及逗号分隔的列表。日期与星期两个字段在
// 两者均被限定时按“或”语义匹配（命中其一即触发），在其中之一为 * 时按“与”语义。
package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 解析与计算相关哨兵错误。
var (
	ErrEmptyExpr  = errors.New("cron 表达式为空")
	ErrFieldCount = errors.New("cron 表达式必须为五段：分 时 日 月 周")
	ErrNoOccur    = errors.New("该表达式在可搜索年限内无可触发时刻")
)

// searchYears 为 Next 搜索未来触发时刻的最大年限，超过即判定无可触发时刻。
const searchYears = 5

// fieldSpec 描述一个字段的合法输入范围、判定全集的范围与集合大小。
type fieldSpec struct {
	validLo, validHi int        // 输入校验范围（含端点）
	fullLo, fullHi   int        // 判定是否等价于 * 的范围（含端点）
	setSize          int        // 命中集合长度
	normalize        func(int) int // 可选：把输入值归一到集合下标（用于星期 7→0）
}

var (
	specMinute = fieldSpec{validLo: 0, validHi: 59, fullLo: 0, fullHi: 59, setSize: 60}
	specHour   = fieldSpec{validLo: 0, validHi: 23, fullLo: 0, fullHi: 23, setSize: 24}
	specDOM    = fieldSpec{validLo: 1, validHi: 31, fullLo: 1, fullHi: 31, setSize: 32} // 索引 1..31，0 不用
	specMonth  = fieldSpec{validLo: 1, validHi: 12, fullLo: 1, fullHi: 12, setSize: 13} // 索引 1..12，0 不用
	specDOW    = fieldSpec{validLo: 0, validHi: 7, fullLo: 0, fullHi: 6, setSize: 7, normalize: func(v int) int {
		if v == 7 {
			return 0 // 7 与 0 均表示星期日
		}
		return v
	}}
)

// field 表示一个字段的命中集合。star 为 true 表示该字段等价于 *（命中全集）。
type field struct {
	set  []bool
	star bool
}

func (f field) match(v int) bool { return v >= 0 && v < len(f.set) && f.set[v] }

// Schedule 是解析后的 cron 调度。
type Schedule struct {
	minute, hour, dom, month, dow field
	domStar, dowStar              bool
}

// Parse 解析五段式 cron 表达式，非法输入返回错误且不做静默修正。
func Parse(expr string) (Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Schedule{}, ErrEmptyExpr
	}
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return Schedule{}, fmt.Errorf("%w: 实际 %d 段", ErrFieldCount, len(parts))
	}
	var (
		s   Schedule
		err error
	)
	if s.minute, err = parseField(parts[0], specMinute, "分钟"); err != nil {
		return Schedule{}, err
	}
	if s.hour, err = parseField(parts[1], specHour, "小时"); err != nil {
		return Schedule{}, err
	}
	if s.dom, err = parseField(parts[2], specDOM, "日期"); err != nil {
		return Schedule{}, err
	}
	if s.month, err = parseField(parts[3], specMonth, "月份"); err != nil {
		return Schedule{}, err
	}
	if s.dow, err = parseField(parts[4], specDOW, "星期"); err != nil {
		return Schedule{}, err
	}
	s.domStar = s.dom.star
	s.dowStar = s.dow.star
	return s, nil
}

// parseField 解析单个字段（含逗号列表），返回命中集合与是否等价于 *。
func parseField(s string, spec fieldSpec, name string) (field, error) {
	if s == "" {
		return field{}, fmt.Errorf("%s字段为空", name)
	}
	f := field{set: make([]bool, spec.setSize)}
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			return field{}, fmt.Errorf("%s字段 %q 含空段", name, s)
		}
		if err := parsePart(part, spec, &f); err != nil {
			return field{}, fmt.Errorf("%s字段 %w", name, err)
		}
	}
	full := true
	for v := spec.fullLo; v <= spec.fullHi; v++ {
		idx := v
		if spec.normalize != nil {
			idx = spec.normalize(v)
		}
		if !f.set[idx] {
			full = false
			break
		}
	}
	hasHit := false
	for _, ok := range f.set {
		if ok {
			hasHit = true
			break
		}
	}
	if !hasHit {
		return field{}, fmt.Errorf("%s字段 %q 无有效取值", name, s)
	}
	f.star = full
	return f, nil
}

// parsePart 解析字段中的单个片段：*、*/n、a、a-b、a-b/n。
func parsePart(part string, spec fieldSpec, f *field) error {
	step := 1
	rangePart := part
	if idx := strings.Index(part, "/"); idx >= 0 {
		rangePart = part[:idx]
		stepStr := part[idx+1:]
		if stepStr == "" {
			return fmt.Errorf("步长为空 %q", part)
		}
		n, err := strconv.Atoi(stepStr)
		if err != nil || n <= 0 {
			return fmt.Errorf("步长非法 %q", stepStr)
		}
		step = n
	}

	var lo, hi int
	switch rangePart {
	case "*":
		lo, hi = spec.fullLo, spec.fullHi
	default:
		if rangePart == "" {
			return fmt.Errorf("字段段为空 %q", part)
		}
		var err error
		if strings.Contains(rangePart, "-") {
			dash := strings.Index(rangePart, "-")
			loStr, hiStr := rangePart[:dash], rangePart[dash+1:]
			if loStr == "" || hiStr == "" {
				return fmt.Errorf("区间非法 %q", rangePart)
			}
			if lo, err = parseInt(loStr, spec); err != nil {
				return err
			}
			if hi, err = parseInt(hiStr, spec); err != nil {
				return err
			}
			if lo > hi {
				return fmt.Errorf("区间下界大于上界 %q", rangePart)
			}
		} else {
			if lo, err = parseInt(rangePart, spec); err != nil {
				return err
			}
			hi = lo
		}
	}

	for v := lo; v <= hi; v += step {
		idx := v
		if spec.normalize != nil {
			idx = spec.normalize(v)
		}
		if idx >= 0 && idx < len(f.set) {
			f.set[idx] = true
		}
	}
	return nil
}

// parseInt 解析单个整数并校验是否落在字段合法范围内。
func parseInt(s string, spec fieldSpec) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("取值非法 %q", s)
	}
	if n < spec.validLo || n > spec.validHi {
		return 0, fmt.Errorf("取值越界 %d（应在 %d-%d）", n, spec.validLo, spec.validHi)
	}
	return n, nil
}

// Next 返回严格晚于 from 的下一次触发时间，精度为分钟。
// 若在 searchYears 年内无触发时刻，返回 ErrNoOccur。
func (s Schedule) Next(from time.Time) (time.Time, error) {
	loc := from.Location()
	if loc == nil {
		loc = time.UTC
	}
	// 丢弃秒以下并进到下一分钟，保证“严格晚于 from”。
	t := time.Date(from.Year(), from.Month(), from.Day(), from.Hour(), from.Minute(), 0, 0, loc).Add(time.Minute)
	yearMax := t.Year() + searchYears
	for t.Year() <= yearMax {
		if !s.month.match(int(t.Month())) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 1, 0)
			continue
		}
		if !s.dayMatch(t) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
			continue
		}
		if !s.hour.match(t.Hour()) {
			t = t.Add(1 * time.Hour)
			continue
		}
		if !s.minute.match(t.Minute()) {
			t = t.Add(1 * time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, ErrNoOccur
}

// dayMatch 按 cron 语义判定某日是否触发：
// 日期与星期均为 * 时任意日触发；仅一个为 * 时按该字段；两者均限定时取“或”。
func (s Schedule) dayMatch(t time.Time) bool {
	domOK := s.dom.match(t.Day())
	dowOK := s.dow.match(int(t.Weekday()))
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dowOK
	case s.dowStar:
		return domOK
	default:
		return domOK || dowOK
	}
}

// NextN 返回从 from 之后最近的 count 次触发时间（count 截断到 1..100）。
// 若首次即无触发，返回错误；后续超出搜索年限则返回已找到的部分。
func (s Schedule) NextN(from time.Time, count int) ([]time.Time, error) {
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 99
	}
	out := make([]time.Time, 0, count)
	cur := from
	for i := 0; i < count; i++ {
		next, err := s.Next(cur)
		if err != nil {
			if len(out) == 0 {
				return nil, err
			}
			return out, nil
		}
		out = append(out, next)
		cur = next
	}
	return out, nil
}

// FieldValues 返回各字段的命中值列表（升序），用于校验端点展示。
func (s Schedule) FieldValues() map[string][]int {
	return map[string][]int{
		"minute": sorted(s.minute.set),
		"hour":   sorted(s.hour.set),
		"dom":    sorted(s.dom.set),
		"month":  sorted(s.month.set),
		"dow":    sorted(s.dow.set),
	}
}

func sorted(set []bool) []int {
	out := []int{}
	for v, ok := range set {
		if ok {
			out = append(out, v)
		}
	}
	return out
}
