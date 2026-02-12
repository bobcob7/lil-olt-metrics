package query

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// BuildInfo holds version and build metadata returned by /api/v1/status/buildinfo.
type BuildInfo struct {
	Version  string
	Revision string
	Branch   string
}

// API implements the Prometheus-compatible HTTP query API.
type API struct {
	logger        *slog.Logger
	queryable     storage.Queryable
	engine        *promql.Engine
	lookbackDelta time.Duration
	buildInfo     BuildInfo
}

// NewAPI creates a new API handler.
func NewAPI(logger *slog.Logger, queryable storage.Queryable, engine *promql.Engine, lookbackDelta time.Duration, info BuildInfo) *API {
	return &API{
		logger:        logger,
		queryable:     queryable,
		engine:        engine,
		lookbackDelta: lookbackDelta,
		buildInfo:     info,
	}
}

// Handler returns an http.Handler with all API routes registered.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", a.query)
	mux.HandleFunc("/api/v1/query_range", a.queryRange)
	mux.HandleFunc("/api/v1/series", a.series)
	mux.HandleFunc("/api/v1/labels", a.labelNames)
	mux.HandleFunc("/api/v1/label/", a.labelValues)
	mux.HandleFunc("/api/v1/metadata", a.metadata)
	mux.HandleFunc("/api/v1/status/buildinfo", a.statusBuildInfo)
	return mux
}

func (a *API) query(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "bad_data", "failed to parse form: "+err.Error())
		return
	}
	qs := r.FormValue("query")
	if qs == "" {
		writeError(w, "bad_data", "missing required parameter: query")
		return
	}
	ts, err := parseTimeParam(r, "time", time.Now())
	if err != nil {
		writeError(w, "bad_data", err.Error())
		return
	}
	qOpts := promql.NewPrometheusQueryOpts(false, a.lookbackDelta)
	qry, err := a.engine.NewInstantQuery(r.Context(), a.queryable, qOpts, qs, ts)
	if err != nil {
		writeError(w, "bad_data", "invalid expression: "+err.Error())
		return
	}
	defer qry.Close()
	result := qry.Exec(r.Context())
	if result.Err != nil {
		writeError(w, errorTypeFromErr(result.Err), result.Err.Error())
		return
	}
	writeQueryResult(w, result)
}

func (a *API) queryRange(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "bad_data", "failed to parse form: "+err.Error())
		return
	}
	qs := r.FormValue("query")
	if qs == "" {
		writeError(w, "bad_data", "missing required parameter: query")
		return
	}
	start, err := parseTimeParam(r, "start", time.Time{})
	if err != nil || start.IsZero() {
		writeError(w, "bad_data", "invalid or missing start parameter")
		return
	}
	end, err := parseTimeParam(r, "end", time.Time{})
	if err != nil || end.IsZero() {
		writeError(w, "bad_data", "invalid or missing end parameter")
		return
	}
	step, err := parseStep(r.FormValue("step"))
	if err != nil {
		writeError(w, "bad_data", "invalid step parameter: "+err.Error())
		return
	}
	qOpts := promql.NewPrometheusQueryOpts(false, a.lookbackDelta)
	qry, err := a.engine.NewRangeQuery(r.Context(), a.queryable, qOpts, qs, start, end, step)
	if err != nil {
		writeError(w, "bad_data", "invalid expression: "+err.Error())
		return
	}
	defer qry.Close()
	result := qry.Exec(r.Context())
	if result.Err != nil {
		writeError(w, errorTypeFromErr(result.Err), result.Err.Error())
		return
	}
	writeQueryResult(w, result)
}

func (a *API) series(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "bad_data", "failed to parse form: "+err.Error())
		return
	}
	matchValues := r.Form["match[]"]
	if len(matchValues) == 0 {
		writeError(w, "bad_data", "missing required parameter: match[]")
		return
	}
	start, _ := parseTimeParam(r, "start", time.Unix(0, 0))
	end, _ := parseTimeParam(r, "end", time.Now())
	mint := start.UnixMilli()
	maxt := end.UnixMilli()
	q, err := a.queryable.Querier(mint, maxt)
	if err != nil {
		writeError(w, "internal", err.Error())
		return
	}
	defer func() {
		if err := q.Close(); err != nil {
			a.logger.Error("closing querier", "error", err)
		}
	}()
	var result []map[string]string
	for _, m := range matchValues {
		matchers, parseErr := parser.ParseMetricSelector(m)
		if parseErr != nil {
			writeError(w, "bad_data", "invalid match[] selector: "+parseErr.Error())
			return
		}
		ss := q.Select(r.Context(), false, nil, matchers...)
		for ss.Next() {
			s := ss.At()
			entry := map[string]string{}
			s.Labels().Range(func(l labels.Label) {
				entry[l.Name] = l.Value
			})
			result = append(result, entry)
		}
		if ss.Err() != nil {
			writeError(w, "internal", ss.Err().Error())
			return
		}
	}
	writeSuccess(w, result)
}

func (a *API) labelNames(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "bad_data", "failed to parse form: "+err.Error())
		return
	}
	start, _ := parseTimeParam(r, "start", time.Unix(0, 0))
	end, _ := parseTimeParam(r, "end", time.Now())
	mint := start.UnixMilli()
	maxt := end.UnixMilli()
	q, err := a.queryable.Querier(mint, maxt)
	if err != nil {
		writeError(w, "internal", err.Error())
		return
	}
	defer func() {
		if err := q.Close(); err != nil {
			a.logger.Error("closing querier", "error", err)
		}
	}()
	names, _, err := q.LabelNames(r.Context(), nil)
	if err != nil {
		writeError(w, "internal", err.Error())
		return
	}
	if names == nil {
		names = []string{}
	}
	sort.Strings(names)
	writeSuccess(w, names)
}

func (a *API) labelValues(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "bad_data", "failed to parse form: "+err.Error())
		return
	}
	path := r.URL.Path
	prefix := "/api/v1/label/"
	suffix := "/values"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		writeError(w, "bad_data", "invalid label values path")
		return
	}
	labelName := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if labelName == "" {
		writeError(w, "bad_data", "missing label name")
		return
	}
	start, _ := parseTimeParam(r, "start", time.Unix(0, 0))
	end, _ := parseTimeParam(r, "end", time.Now())
	mint := start.UnixMilli()
	maxt := end.UnixMilli()
	q, err := a.queryable.Querier(mint, maxt)
	if err != nil {
		writeError(w, "internal", err.Error())
		return
	}
	defer func() {
		if err := q.Close(); err != nil {
			a.logger.Error("closing querier", "error", err)
		}
	}()
	values, _, err := q.LabelValues(r.Context(), labelName, nil)
	if err != nil {
		writeError(w, "internal", err.Error())
		return
	}
	if values == nil {
		values = []string{}
	}
	sort.Strings(values)
	writeSuccess(w, values)
}

func (a *API) metadata(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, map[string]any{})
}

func (a *API) statusBuildInfo(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, map[string]string{
		"version":   a.buildInfo.Version,
		"revision":  a.buildInfo.Revision,
		"branch":    a.buildInfo.Branch,
		"buildDate": time.Now().UTC().Format(time.RFC3339),
		"goVersion": runtime.Version(),
	})
}

type apiResponse struct {
	Status    string   `json:"status"`
	Data      any      `json:"data,omitempty"`
	ErrorType string   `json:"errorType,omitempty"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

func writeSuccess(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "success", Data: data})
}

func writeError(w http.ResponseWriter, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "error", ErrorType: errType, Error: msg})
}

func writeQueryResult(w http.ResponseWriter, result *promql.Result) {
	var warnings []string
	for _, ann := range result.Warnings {
		warnings = append(warnings, ann.Error())
	}
	data := map[string]any{
		"resultType": result.Value.Type(),
		"result":     result.Value,
	}
	w.Header().Set("Content-Type", "application/json")
	resp := apiResponse{Status: "success", Data: data, Warnings: warnings}
	_ = json.NewEncoder(w).Encode(resp)
}

func parseTimeParam(r *http.Request, param string, defaultVal time.Time) (time.Time, error) {
	v := r.FormValue(param)
	if v == "" {
		return defaultVal, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err == nil {
		return t, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s parameter %q: cannot parse as RFC3339 or unix timestamp", param, v)
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec), nil
}

func parseStep(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("missing required parameter: step")
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	f, ferr := strconv.ParseFloat(s, 64)
	if ferr != nil {
		return 0, fmt.Errorf("cannot parse %q as duration or seconds", s)
	}
	if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("zero or negative step not allowed")
	}
	return time.Duration(f * float64(time.Second)), nil
}

func errorTypeFromErr(err error) string {
	switch err.(type) {
	case promql.ErrQueryCanceled:
		return "canceled"
	case promql.ErrQueryTimeout:
		return "timeout"
	case promql.ErrStorage:
		return "internal"
	default:
		return "execution"
	}
}
