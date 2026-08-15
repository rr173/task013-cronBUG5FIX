// Package selfcheck 提供无需外部依赖、执行后自行退出的 --smoke-test。
package selfcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"task013-cron/internal/httpapi"
)

// Run 执行内置自测，返回退出码：0 表示全部通过，1 表示存在失败。
func Run() int {
	passed, failed := 0, 0
	check := func(name string, fn func() error) {
		if err := fn(); err != nil {
			failed++
			fmt.Printf("FAIL %-36s %v\n", name, err)
		} else {
			passed++
			fmt.Printf("PASS %s\n", name)
		}
	}

	api := httpapi.New()
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	do := func(method, path, body string) (*http.Response, []byte, error) {
		var r io.Reader
		if body != "" {
			r = bytes.NewReader([]byte(body))
		}
		req, err := http.NewRequest(method, srv.URL+path, r)
		if err != nil {
			return nil, nil, err
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, data, readErr
	}
	post := func(path, body string) (*http.Response, []byte, error) {
		return do(http.MethodPost, path, body)
	}

	check("健康检查", func() error {
		resp, _, err := do(http.MethodGet, "/healthz", "")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验合法表达式返回字段", func() error {
		resp, body, err := post("/api/validate", `{"expr":"*/15 * * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Valid  bool            `json:"valid"`
			Fields map[string][]int `json:"fields"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if !out.Valid {
			return fmt.Errorf("valid=false")
		}
		if got := out.Fields["minute"]; !eqInt(got, []int{0, 15, 30, 45}) {
			return fmt.Errorf("minute=%v", got)
		}
		return nil
	})

	check("校验步长取值正确", func() error {
		resp, body, err := post("/api/validate", `{"expr":"2-10/3 * * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Fields map[string][]int `json:"fields"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if got := out.Fields["minute"]; !eqInt(got, []int{2, 5, 8}) {
			return fmt.Errorf("minute=%v", got)
		}
		return nil
	})

	check("校验小时越界被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"0 24 * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验日期越界被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"0 0 32 * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验月份越界被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"0 0 * 13 *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验星期越界被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"0 0 * * 8"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验步长为 0 被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"*/0 * * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验区间反向被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"5-1 * * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验段数错误被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"0 0 * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("校验空段被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"0, * * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("下次触发:每分钟", func() error {
		resp, body, err := post("/api/next", `{"expr":"* * * * *","from":"2026-01-15T10:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-15T10:01:00Z"})
	})

	check("下次触发:固定时分", func() error {
		resp, body, err := post("/api/next", `{"expr":"30 5 * * *","from":"2026-01-15T10:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-16T05:30:00Z"})
	})

	check("下次触发:严格晚于起点", func() error {
		resp, body, err := post("/api/next", `{"expr":"*/15 * * * *","from":"2026-01-15T10:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-15T10:15:00Z"})
	})

	check("下次触发:日期星期或语义", func() error {
		resp, body, err := post("/api/next", `{"expr":"0 0 13 * 5","from":"2026-01-10T00:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-13T00:00:00Z"})
	})

	check("下次触发:仅星期五", func() error {
		resp, body, err := post("/api/next", `{"expr":"0 0 * * 5","from":"2026-01-10T00:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-16T00:00:00Z"})
	})

	check("下次触发:7 等价星期日", func() error {
		resp, body, err := post("/api/next", `{"expr":"0 0 * * 7","from":"2026-01-10T00:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-11T00:00:00Z"})
	})

	check("下次触发:闰年二月 29", func() error {
		resp, body, err := post("/api/next", `{"expr":"0 0 29 2 *","from":"2026-01-01T00:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2028-02-29T00:00:00Z"})
	})

	check("下次触发:二月的 30 日无可触发", func() error {
		resp, body, err := post("/api/next", `{"expr":"0 0 30 2 *","from":"2026-01-01T00:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusUnprocessableEntity {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return nil
	})

	check("下次触发:多次返回", func() error {
		resp, body, err := post("/api/next", `{"expr":"*/15 * * * *","from":"2026-01-15T10:00:00Z","count":4}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{
			"2026-01-15T10:15:00Z",
			"2026-01-15T10:30:00Z",
			"2026-01-15T10:45:00Z",
			"2026-01-15T11:00:00Z",
		})
	})

	check("下次触发:保留时区偏移", func() error {
		resp, body, err := post("/api/next", `{"expr":"30 5 * * *","from":"2026-01-15T10:00:00+08:00"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		return checkTimes(body, []string{"2026-01-16T05:30:00+08:00"})
	})

	check("非法 from 被拒绝", func() error {
		resp, _, err := post("/api/next", `{"expr":"* * * * *","from":"not-a-time"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("非法表达式被拒绝", func() error {
		resp, _, err := post("/api/next", `{"expr":"0 24 * * *","from":"2026-01-01T00:00:00Z"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("缺失字段被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"x":"1"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("非法 JSON 被拒绝", func() error {
		resp, _, err := post("/api/validate", `{not json}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("多段 JSON 被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"* * * * *"}{"expr":"0 0 * * *"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("未知字段被拒绝", func() error {
		resp, _, err := post("/api/validate", `{"expr":"* * * * *","extra":1}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func checkTimes(body []byte, want []string) error {
	var out struct {
		Times []string `json:"times"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	if len(out.Times) != len(want) {
		return fmt.Errorf("times=%v want=%v", out.Times, want)
	}
	for i, w := range want {
		if out.Times[i] != w {
			return fmt.Errorf("times[%d]=%q want %q", i, out.Times[i], w)
		}
	}
	return nil
}

func eqInt(a, b []int) bool {
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
