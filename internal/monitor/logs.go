package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// RegisterLogRoutes adds Loki log query endpoints.
func (h *Handler) RegisterLogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/logs", h.handleLogs)
	mux.HandleFunc("/api/logs/labels", h.handleLogLabels)
}

// handleLogs queries Loki for log entries.
// Query params: query (LogQL), limit, start, end, direction
func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	lokiURL := h.lokiURL
	if lokiURL == "" {
		http.Error(w, "loki not configured", 503)
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		query = `{namespace=~".+"}`
	}

	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	params := url.Values{
		"query":     {query},
		"limit":     {limit},
		"direction": {"backward"},
	}
	if v := r.URL.Query().Get("start"); v != "" {
		params.Set("start", v)
	}
	if v := r.URL.Query().Get("end"); v != "" {
		params.Set("end", v)
	}

	resp, err := http.Get(fmt.Sprintf("%s/loki/api/v1/query_range?%s", lokiURL, params.Encode()))
	if err != nil {
		http.Error(w, fmt.Sprintf("loki query: %v", err), 502)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleLogLabels returns available label values from Loki.
func (h *Handler) handleLogLabels(w http.ResponseWriter, r *http.Request) {
	lokiURL := h.lokiURL
	if lokiURL == "" {
		http.Error(w, "loki not configured", 503)
		return
	}

	resp, err := http.Get(fmt.Sprintf("%s/loki/api/v1/labels", lokiURL))
	if err != nil {
		http.Error(w, fmt.Sprintf("loki labels: %v", err), 502)
		return
	}
	defer resp.Body.Close()

	var result json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Log but don't fail — partial data is better than no data
		_ = err
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
