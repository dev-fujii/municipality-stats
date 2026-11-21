package estat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAppID = "test-app-id-12345"

// TestNewClient は、既定値、設定の検証、設定の適用順を確かめる。
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		appID   string
		opts    []Option
		wantErr bool
	}{
		{name: "defaults", appID: testAppID},
		{name: "empty appId", wantErr: true},
		{name: "https", appID: testAppID, opts: []Option{WithBaseURL("https://example.com/api///")}},
		{name: "IPv4 loopback", appID: testAppID, opts: []Option{WithBaseURL("http://127.0.0.1:12345")}},
		{name: "localhost", appID: testAppID, opts: []Option{WithBaseURL("http://localhost")}},
		{name: "IPv6 loopback", appID: testAppID, opts: []Option{WithBaseURL("http://[::1]:12345")}},
		{name: "external HTTP", appID: testAppID, opts: []Option{WithBaseURL("http://example.com")}, wantErr: true},
		{name: "FTP", appID: testAppID, opts: []Option{WithBaseURL("ftp://example.com")}, wantErr: true},
		{name: "missing host", appID: testAppID, opts: []Option{WithBaseURL("https:///api")}, wantErr: true},
		{name: "empty URL", appID: testAppID, opts: []Option{WithBaseURL("")}, wantErr: true},
		{name: "invalid URL", appID: testAppID, opts: []Option{WithBaseURL("https://%zz")}, wantErr: true},
		{name: "minimum limit", appID: testAppID, opts: []Option{WithLimit(1)}},
		{name: "maximum limit", appID: testAppID, opts: []Option{WithLimit(100000)}},
		{name: "zero limit", appID: testAppID, opts: []Option{WithLimit(0)}, wantErr: true},
		{name: "negative limit", appID: testAppID, opts: []Option{WithLimit(-1)}, wantErr: true},
		{name: "excessive limit", appID: testAppID, opts: []Option{WithLimit(100001)}, wantErr: true},
		{name: "nil HTTP client", appID: testAppID, opts: []Option{WithHTTPClient(nil)}, wantErr: true},
		{name: "option order", appID: testAppID, opts: []Option{WithLimit(1), WithLimit(2), WithBaseURL("https://example.com/first"), WithBaseURL("https://example.com/last/")}},
		{name: "stop on invalid option", appID: testAppID, opts: []Option{WithLimit(0), WithLimit(2)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.appID, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if c != nil || !strings.HasPrefix(err.Error(), "e-Stat: ") {
					t.Fatalf("NewClient() = (%v, %v), want nil client and e-Stat error", c, err)
				}
				return
			}
			if strings.HasSuffix(c.baseURL, "/") {
				t.Errorf("baseURL = %q, want no trailing slash", c.baseURL)
			}
			if tt.name == "defaults" {
				if c.baseURL != DefaultBaseURL || c.limit != DefaultLimit || c.httpClient.Timeout != 60*time.Second {
					t.Errorf("unexpected defaults: %+v", c)
				}
				if err := c.httpClient.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
					t.Errorf("CheckRedirect() = %v, want ErrUseLastResponse", err)
				}
			}
			if tt.name == "option order" && (c.limit != 2 || c.baseURL != "https://example.com/last") {
				t.Errorf("options applied out of order: %+v", c)
			}
		})
	}
}

// TestWithHTTPClient は、渡した値を変更せず、写しのリダイレクトだけを禁止することを確かめる。
func TestWithHTTPClient(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	originalRedirectErr := errors.New("original redirect policy")
	h := &http.Client{
		Timeout:       7 * time.Second,
		Transport:     &http.Transport{},
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return originalRedirectErr },
	}
	c, err := NewClient(testAppID, WithHTTPClient(h))
	if err != nil {
		t.Fatal(err)
	}
	if c.httpClient == h || c.httpClient.Timeout != h.Timeout || c.httpClient.Transport != h.Transport || c.httpClient.Jar != h.Jar {
		t.Errorf("HTTP client settings were not copied: %+v", c.httpClient)
	}
	if got := h.CheckRedirect(nil, nil); got != originalRedirectErr {
		t.Errorf("original CheckRedirect() = %v, want %v", got, originalRedirectErr)
	}
	if got := c.httpClient.CheckRedirect(nil, nil); got != http.ErrUseLastResponse {
		t.Errorf("copied CheckRedirect() = %v, want ErrUseLastResponse", got)
	}
}

// TestClientQueryValidation は、不正な取得条件で通信しないことを確かめる。
func TestClientQueryValidation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c := newTestClient(t, server)
	tests := []struct {
		name  string
		query Query
	}{
		{name: "empty table", query: Query{Codes: []string{"A"}}},
		{name: "no codes", query: Query{TableID: "table"}},
		{name: "too many codes", query: Query{TableID: "table", Codes: slices.Repeat([]string{"A"}, 101)}},
		{name: "empty code", query: Query{TableID: "table", Codes: []string{"A", ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := c.GetStatsData(context.Background(), tt.query)
			assertClientError(t, records, err, "e-Stat: ")
		})
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("requests = %d, want 0", got)
	}
}

// TestClientRequestParameters は、項目コードの記号を保ち、指定したパラメータだけを送ることを確かめる。
func TestClientRequestParameters(t *testing.T) {
	tests := []struct {
		name  string
		codes []string
	}{
		{name: "hash code", codes: []string{"#A01202", "H2130"}},
		{name: "100 codes", codes: slices.Repeat([]string{"A"}, 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/getStatsData" {
					t.Errorf("request = %s %s, want GET /api/getStatsData", r.Method, r.URL.Path)
				}
				want := url.Values{
					"appId":       {testAppID},
					"statsDataId": {"table"},
					"cdCat01":     {strings.Join(tt.codes, ",")},
					"limit":       {"2"},
					"metaGetFlg":  {"N"},
				}
				if got := r.URL.Query(); !reflect.DeepEqual(got, want) {
					t.Errorf("query = %v, want %v", got, want)
				}
				if r.URL.Fragment != "" {
					t.Errorf("Fragment = %q, want empty", r.URL.Fragment)
				}
				writeClientResponse(t, w, 1, 0, clientValue("00001", "42"))
			}))
			defer server.Close()
			c := newTestClient(t, server, WithBaseURL(server.URL+"/api/"), WithLimit(2))
			records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: tt.codes})
			if err != nil || len(records) != 1 {
				t.Fatalf("GetStatsData() = (%v, %v), want 1 record", records, err)
			}
		})
	}
}

// TestClientPagination は、NEXT_KEY を次の要求に渡し、全件を応答の順に返すことを確かめる。
func TestClientPagination(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Errorf("limit = %q, want 2", got)
		}
		switch r.URL.Query().Get("startPosition") {
		case "":
			if call != 1 {
				t.Errorf("first page requested on call %d", call)
			}
			writeClientResponse(t, w, 3, 3, clientValue("00001", "10"), clientValue("00002", "20"))
		case "3":
			if call != 2 {
				t.Errorf("second page requested on call %d", call)
			}
			writeClientResponse(t, w, 3, 0, clientValue("00003", "30"))
		default:
			t.Errorf("unexpected startPosition = %q", r.URL.Query().Get("startPosition"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := newTestClient(t, server, WithLimit(2))
	records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []Record{
		{TableID: "table", Code: "H2130", Area: "00001", Year: 2023, Unit: "人", Value: Value{Raw: "10", Status: StatusOK}},
		{TableID: "table", Code: "H2130", Area: "00002", Year: 2023, Unit: "人", Value: Value{Raw: "20", Status: StatusOK}},
		{TableID: "table", Code: "H2130", Area: "00003", Year: 2023, Unit: "人", Value: Value{Raw: "30", Status: StatusOK}},
	}
	if !slices.Equal(records, want) || calls.Load() != 2 {
		t.Errorf("GetStatsData() = %+v with %d requests, want %+v with 2 requests", records, calls.Load(), want)
	}
}

// TestClientPaginationErrors は、進まない取得位置や件数の不整合で途中の結果を返さないことを確かめる。
func TestClientPaginationErrors(t *testing.T) {
	tests := []struct {
		name   string
		totals []int
		nexts  []int
		want   string
	}{
		{name: "first NEXT_KEY equal", totals: []int{1}, nexts: []int{1}, want: "NEXT_KEY"},
		{name: "first NEXT_KEY lower", totals: []int{1}, nexts: []int{-1}, want: "NEXT_KEY"},
		{name: "second NEXT_KEY equal", totals: []int{2, 2}, nexts: []int{3, 3}, want: "NEXT_KEY"},
		{name: "second NEXT_KEY lower", totals: []int{2, 2}, nexts: []int{3, 2}, want: "NEXT_KEY"},
		{name: "count mismatch", totals: []int{2}, nexts: []int{0}, want: "record count"},
		{name: "total changed", totals: []int{2, 3}, nexts: []int{3, 0}, want: "TOTAL_NUMBER changed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(calls.Add(1)) - 1
				if i >= len(tt.nexts) {
					t.Error("unexpected extra request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				writeClientResponse(t, w, tt.totals[i], tt.nexts[i], clientValue(fmt.Sprintf("%05d", i+1), "10"))
			}))
			defer server.Close()
			c := newTestClient(t, server)
			records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
			assertClientError(t, records, err, tt.want)
			if got := int(calls.Load()); got != len(tt.nexts) {
				t.Errorf("requests = %d, want %d", got, len(tt.nexts))
			}
		})
	}
}

func TestClientPaginationMissingResult(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		switch r.URL.Query().Get("startPosition") {
		case "":
			if call != 1 {
				t.Errorf("first page requested on call %d", call)
			}
			writeClientResponse(t, w, 3, 3, clientValue("00001", "10"), clientValue("00002", "20"))
		case "3":
			if call != 2 {
				t.Errorf("second page requested on call %d", call)
			}
			if _, err := io.WriteString(w, `{"GET_STATS_DATA":{}}`); err != nil {
				t.Error(err)
			}
		default:
			t.Errorf("unexpected startPosition = %q", r.URL.Query().Get("startPosition"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := newTestClient(t, server, WithLimit(2))
	records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
	assertClientError(t, records, err, "RESULT.STATUS")
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// TestClientAPIErrors は、API の処理状態を保持し、メッセージ内の appId を伏せることを確かめる。
func TestClientAPIErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		want    string
	}{
		{name: "no data", status: 1, message: "no matching data", want: "no matching data"},
		{name: "invalid appId", status: 100, message: "invalid appId", want: "invalid appId"},
		{name: "appId in message", status: 100, message: testAppID + " invalid: " + testAppID, want: "[REDACTED] invalid: [REDACTED]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if err := json.NewEncoder(w).Encode(map[string]any{
					"GET_STATS_DATA": map[string]any{"RESULT": map[string]any{"STATUS": tt.status, "ERROR_MSG": tt.message}},
				}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			c := newTestClient(t, server)
			records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
			assertClientError(t, records, err, "API status")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tt.status || apiErr.Message != tt.want {
				t.Fatalf("APIError = %+v, want status %d and message %q", apiErr, tt.status, tt.want)
			}
			if got := errors.Is(err, ErrNoData); got != (tt.status == 1) {
				t.Errorf("errors.Is(ErrNoData) = %v, status = %d", got, tt.status)
			}
			if strings.Contains(err.Error(), testAppID) {
				t.Errorf("error contains appId: %v", err)
			}
		})
	}
}

// TestClientResponseErrors は、HTTP と JSON の失敗や builder のエラーを返すことを確かめる。
func TestClientResponseErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "HTTP 500", status: http.StatusInternalServerError, body: "unread response", want: "500"},
		{name: "invalid JSON", status: http.StatusOK, body: "{", want: "decode response"},
		{name: "empty JSON body", status: http.StatusOK, want: "decode response"},
		{name: "missing GET_STATS_DATA", status: http.StatusOK, body: `{}`, want: "RESULT.STATUS"},
		{name: "missing RESULT", status: http.StatusOK, body: `{"GET_STATS_DATA":{}}`, want: "RESULT.STATUS"},
		{name: "missing STATUS", status: http.StatusOK, body: `{"GET_STATS_DATA":{"RESULT":{}}}`, want: "RESULT.STATUS"},
		{name: "null STATUS", status: http.StatusOK, body: `{"GET_STATS_DATA":{"RESULT":{"STATUS":null}}}`, want: "RESULT.STATUS"},
		{name: "missing NOTE", status: http.StatusOK, body: `{"GET_STATS_DATA":{"RESULT":{"STATUS":0},"STATISTICAL_DATA":{"RESULT_INF":{"TOTAL_NUMBER":1},"DATA_INF":{"VALUE":{"@tab":"00001","@cat01":"H2130","@area":"00001","@time":"2023100000","$":"-"}}}}}`, want: `e-Stat: VALUE[0]: symbol "-" is not in NOTE`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
				if _, err := io.WriteString(w, tt.body); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			c := newTestClient(t, server)
			records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
			assertClientError(t, records, err, tt.want)
			if tt.name == "missing NOTE" && err.Error() != tt.want {
				t.Errorf("builder error = %q, want unchanged %q", err, tt.want)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("requests = %d, want 1", got)
			}
		})
	}
}

// TestClientFixture は、保存した応答と続きの応答を Record に変え、記号の状態を保つことを確かめる。
func TestClientFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/get_stats_data.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			if _, err := w.Write(data); err != nil {
				t.Error(err)
			}
			return
		}
		if got := r.URL.Query().Get("startPosition"); got != "5" {
			t.Errorf("startPosition = %q, want 5", got)
		}
		values := make([]map[string]string, 32)
		for i := range values {
			values[i] = clientValue(fmt.Sprintf("%05d", 80000+i), "10")
		}
		writeClientResponse(t, w, 34, 0, values...)
	}))
	defer server.Close()
	c := newTestClient(t, server)
	records, err := c.GetStatsData(context.Background(), Query{TableID: "0000020208", Codes: []string{"H2130"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 34 {
		t.Fatalf("records = %d, want 34", len(records))
	}
	want := []Record{
		{TableID: "0000020208", Code: "H2130", Area: "10201", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "102.55", Status: StatusOK}},
		{TableID: "0000020208", Code: "H2130", Area: "10424", Year: 2023, Unit: "ｍ2", Value: Value{Raw: "-", Status: StatusUnavailable}},
	}
	if !slices.Equal(records[:2], want) {
		t.Errorf("fixture records = %+v, want %+v", records[:2], want)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

// TestClientRedirect は、既定と差し替えた HTTP クライアントの両方で転送先へ接続しないことを確かめる。
func TestClientRedirect(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		writeClientResponse(t, w, 0, 0)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	tests := []struct {
		name string
		opts []Option
	}{
		{name: "default"},
		{name: "custom", opts: []Option{WithHTTPClient(&http.Client{})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, server, tt.opts...)
			records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
			assertClientError(t, records, err, "302")
		})
	}
	if got := targetCalls.Load(); got != 0 {
		t.Errorf("redirect target requests = %d, want 0", got)
	}
}

// TestClientCancel は、応答待ちの取り消しが伝わり、エラーの URL に appId が残らないことを確かめる。
func TestClientCancel(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	c := newTestClient(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	records, err := c.GetStatsData(ctx, Query{TableID: "table", Codes: []string{"H2130"}})
	assertClientError(t, records, err, "e-Stat: ")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	assertSafeURLError(t, err)
	<-done
}

// TestClientClosedServer は、接続失敗のエラーと URL に appId が残らないことを確かめる。
func TestClientClosedServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	c := newTestClient(t, server)
	records, err := c.GetStatsData(context.Background(), Query{TableID: "table", Codes: []string{"H2130"}})
	assertClientError(t, records, err, "e-Stat: ")
	assertSafeURLError(t, err)
}

// TestClientRequestError は、URL エラーの原因と操作を保持し、元のエラーを鎖に残さないことを確かめる。
func TestClientRequestError(t *testing.T) {
	c, err := NewClient(testAppID)
	if err != nil {
		t.Fatal(err)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			original := &url.Error{Op: "Get", URL: "https://example.com/getStatsData?appId=" + testAppID + "&limit=2", Err: cause}
			err := c.requestError(original)
			assertSafeURLError(t, err)
			var safe *url.Error
			if !errors.As(err, &safe) || safe == original || safe.Op != original.Op || safe.Err != cause {
				t.Fatalf("error = %v, want new URL error preserving Op and Err", err)
			}
			if !errors.Is(err, cause) || errors.Is(err, original) {
				t.Errorf("error chain does not preserve only the cause: %v", err)
			}
			if !strings.Contains(safe.URL, "limit=2") {
				t.Errorf("URL = %q, want other query parameters preserved", safe.URL)
			}
		})
	}
}

// TestClientOutput は、Client の値とポインターを整形・JSON・ログに出しても appId が出ないことを確かめる。
func TestClientOutput(t *testing.T) {
	c, err := NewClient(testAppID)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{c, *c} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
			if got := fmt.Sprintf(format, value); strings.Contains(got, testAppID) {
				t.Errorf("format %q contains appId: %s", format, got)
			}
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), testAppID) {
			t.Errorf("JSON contains appId: %s", data)
		}
		for _, jsonHandler := range []bool{false, true} {
			var output bytes.Buffer
			var handler slog.Handler = slog.NewTextHandler(&output, nil)
			if jsonHandler {
				handler = slog.NewJSONHandler(&output, nil)
			}
			slog.New(handler).Info("client", slog.Any("client", value))
			if strings.Contains(output.String(), testAppID) {
				t.Errorf("slog output contains appId: %s", output.String())
			}
		}
	}
	if c.appID.String() != redacted || c.appID.GoString() != redacted || c.appID.LogValue().String() != redacted {
		t.Error("secret methods must return [REDACTED]")
	}
	data, err := json.Marshal(c.appID)
	if err != nil || string(data) != `"[REDACTED]"` {
		t.Errorf("secret JSON = %s, error = %v, want redacted string", data, err)
	}
}

// newTestClient は、テスト用サーバーに接続する Client を作る。
func newTestClient(t *testing.T, server *httptest.Server, opts ...Option) *Client {
	t.Helper()
	opts = append([]Option{WithBaseURL(server.URL)}, opts...)
	c, err := NewClient(testAppID, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// clientValue は、テスト用の VALUE を1件作る。
func clientValue(area, raw string) map[string]string {
	return map[string]string{"@tab": "00001", "@cat01": "H2130", "@area": area, "@time": "2023100000", "@unit": "人", "$": raw}
}

// writeClientResponse は、指定した件数と取得位置でテスト用の応答を返す。
func writeClientResponse(t *testing.T, w http.ResponseWriter, total, next int, values ...map[string]string) {
	t.Helper()
	if values == nil {
		values = []map[string]string{}
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"GET_STATS_DATA": map[string]any{
			"RESULT": map[string]any{"STATUS": 0},
			"STATISTICAL_DATA": map[string]any{
				"RESULT_INF": map[string]int{"TOTAL_NUMBER": total, "NEXT_KEY": next},
				"DATA_INF":   map[string]any{"VALUE": values},
			},
		},
	}); err != nil {
		t.Error(err)
	}
}

// assertClientError は、エラー時に途中の Record が返らず、文の接頭辞がそろうことを確かめる。
func assertClientError(t *testing.T, records []Record, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("GetStatsData() succeeded, want error")
	}
	if records != nil {
		t.Errorf("records = %+v, want nil on error", records)
	}
	if !strings.HasPrefix(err.Error(), "e-Stat: ") || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want e-Stat prefix and %q", err, want)
	}
}

// assertSafeURLError は、エラーの文と取り出せる URL の appId が伏せられていることを確かめる。
func assertSafeURLError(t *testing.T, err error) {
	t.Helper()
	if strings.Contains(err.Error(), testAppID) {
		t.Errorf("error contains appId: %v", err)
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("error = %v, want *url.Error", err)
	}
	if strings.Contains(urlErr.URL, testAppID) {
		t.Errorf("URL contains appId: %s", urlErr.URL)
	}
	u, parseErr := url.Parse(urlErr.URL)
	if parseErr != nil || u.Query().Get("appId") != redacted {
		t.Errorf("URL = %q, error = %v, want redacted appId query", urlErr.URL, parseErr)
	}
}

// ExampleClient_GetStatsData は、統計表を取得し、Record の地域・年・値を読む使い方を示す。
func ExampleClient_GetStatsData() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"GET_STATS_DATA":{"RESULT":{"STATUS":0},"STATISTICAL_DATA":{"RESULT_INF":{"TOTAL_NUMBER":1},"DATA_INF":{"VALUE":{"@tab":"00001","@cat01":"H2130","@area":"10201","@time":"2023100000","@unit":"ｍ2","$":"102.55"}}}}}`)
	}))
	defer server.Close()

	client, err := NewClient("test-app-id-12345", WithBaseURL(server.URL))
	if err != nil {
		fmt.Println(err)
		return
	}
	records, err := client.GetStatsData(context.Background(), Query{TableID: "0000020208", Codes: []string{"H2130"}})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, record := range records {
		if record.Value.Status == StatusOK {
			fmt.Println(record.Area, record.Year, record.Value.Raw, record.Unit)
		}
	}
	// Output: 10201 2023 102.55 ｍ2
}
