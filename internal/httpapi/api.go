// Package httpapi 提供 cron 服务的 HTTP 接口。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"task013-cron/internal/cron"
)

// ErrBadJSON 表示请求体不是合法的单个 JSON 对象。
var ErrBadJSON = errors.New("请求体不是合法的单个 JSON 对象")

// API 是 cron 服务，内含简单的请求计数用于监控。
type API struct {
	reqCount int
}

// New 创建 API 实例。
func New() *API { return &API{} }

// RequestCount 返回已处理的请求总数。
func (a *API) RequestCount() int { return a.reqCount }

// Handler 返回 HTTP 路由。
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/validate", a.validate)
	mux.HandleFunc("POST /api/next", a.next)
	return mux
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return ErrBadJSON
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 将输入类错误统一以 400 返回，避免 panic 或 5xx。
func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "status": http.StatusBadRequest})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	a.reqCount++
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) validate(w http.ResponseWriter, r *http.Request) {
	a.reqCount++
	var req struct {
		Expr string `json:"expr"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Expr == "" {
		writeError(w, errors.New("expr 字段不能为空"))
		return
	}
	s, err := cron.Parse(req.Expr)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "fields": s.FieldValues()})
}

func (a *API) next(w http.ResponseWriter, r *http.Request) {
	a.reqCount++
	var req struct {
		Expr  string `json:"expr"`
		From  string `json:"from"`
		Count int    `json:"count"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Expr == "" {
		writeError(w, errors.New("expr 字段不能为空"))
		return
	}
	s, err := cron.Parse(req.Expr)
	if err != nil {
		writeError(w, err)
		return
	}
	var from time.Time
	if req.From == "" {
		from = time.Now()
	} else {
		from, err = time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeError(w, fmt.Errorf("from 字段不是合法 RFC3339 时间: %v", err))
			return
		}
	}
	count := req.Count
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}
	times, err := s.NextN(from, count)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]string, len(times))
	for i, t := range times {
		out[i] = t.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{"times": out})
}
