package estat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL は、e-Stat API 3.0 の JSON 形式の接続先。
const DefaultBaseURL = "https://api.e-stat.go.jp/rest/3.0/app/json"

// DefaultLimit は、1回の応答で取得する件数の既定値と上限。
const DefaultLimit = 100000

const redacted = "[REDACTED]"

// secret は、出力するときに appId を伏せる。
type secret string

// String は、文字列としての出力を伏せる。
func (secret) String() string { return redacted }

// GoString は、Go の構文での出力を伏せる。
func (secret) GoString() string { return redacted }

// LogValue は、構造化ログへの出力を伏せる。
func (secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON は、JSON への出力を伏せる。
func (secret) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }

// Client は、e-Stat の統計表を取得する。
type Client struct {
	baseURL string
	// fmt は非公開フィールドの String を呼ばないため、ポインターで保持する。
	appID      *secret
	httpClient *http.Client
	limit      int
}

// Option は、Client の設定を変更する。
type Option func(*Client) error

// NewClient は、appId と指定した設定で Client を作る。
func NewClient(appID string, opts ...Option) (*Client, error) {
	if appID == "" {
		return nil, errors.New("e-Stat: appId is empty")
	}
	id := secret(appID)
	c := &Client{
		baseURL: DefaultBaseURL,
		appID:   &id,
		httpClient: &http.Client{
			Timeout:       60 * time.Second,
			CheckRedirect: stopRedirect,
		},
		limit: DefaultLimit,
	}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// stopRedirect は、appId を含む Referer が次の接続先へ送られるのを防ぐ。
func stopRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

// WithBaseURL は、HTTPS またはループバックの HTTP を接続先にする。
func WithBaseURL(u string) Option {
	return func(c *Client) error {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Hostname() == "" {
			return errors.New("e-Stat: invalid base URL")
		}
		loopback := parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1"
		if parsed.Scheme != "https" && (parsed.Scheme != "http" || !loopback) {
			return errors.New("e-Stat: base URL requires HTTPS or loopback HTTP")
		}
		c.baseURL = strings.TrimRight(u, "/")
		return nil
	}
}

// WithHTTPClient は、渡された HTTP クライアントの写しでリダイレクトを禁止する。
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) error {
		if h == nil {
			return errors.New("e-Stat: HTTP client is nil")
		}
		cloned := *h
		cloned.CheckRedirect = stopRedirect
		c.httpClient = &cloned
		return nil
	}
}

// WithLimit は、1回の取得件数を1以上100000以下に設定する。
func WithLimit(n int) Option {
	return func(c *Client) error {
		if n < 1 || n > DefaultLimit {
			return errors.New("e-Stat: limit must be between 1 and 100000")
		}
		c.limit = n
		return nil
	}
}

// Query は、取得する統計表と1個から100個の項目コードを指定する。
type Query struct {
	TableID string
	Codes   []string
}

// ErrNoData は、指定した条件のデータが無いことを表す。
var ErrNoData = errors.New("e-Stat: no data")

// APIError は、e-Stat API が返した処理状態とメッセージを持つ。
type APIError struct {
	Status  int
	Message string
}

// Error は、API の処理状態とメッセージを返す。
func (e *APIError) Error() string {
	return fmt.Sprintf("e-Stat: API status %d: %s", e.Status, e.Message)
}

// Unwrap は、該当データなしのときだけ ErrNoData を返す。
func (e *APIError) Unwrap() error {
	if e.Status == 1 {
		return ErrNoData
	}
	return nil
}

// GetStatsData は、統計表の続きを順に取得し、全件を Record に変えて返す。
func (c *Client) GetStatsData(ctx context.Context, q Query) ([]Record, error) {
	if q.TableID == "" {
		return nil, errors.New("e-Stat: table ID is empty")
	}
	if len(q.Codes) == 0 || len(q.Codes) > 100 {
		return nil, errors.New("e-Stat: codes must contain between 1 and 100 items")
	}
	for _, code := range q.Codes {
		if code == "" {
			return nil, errors.New("e-Stat: code is empty")
		}
	}
	params := url.Values{
		"appId":       {string(*c.appID)},
		"statsDataId": {q.TableID},
		"cdCat01":     {strings.Join(q.Codes, ",")},
		"limit":       {strconv.Itoa(c.limit)},
		"metaGetFlg":  {"N"},
	}
	builder := newRecordBuilder(q.TableID)
	start, total := 1, 0
	for {
		body, err := c.getPage(ctx, params)
		if err != nil {
			return nil, err
		}
		if body.Result.Status == nil {
			return nil, errors.New("e-Stat: response has no RESULT.STATUS")
		}
		if *body.Result.Status != 0 {
			return nil, &APIError{
				Status:  *body.Result.Status,
				Message: strings.ReplaceAll(body.Result.ErrorMsg, string(*c.appID), redacted),
			}
		}
		data := body.StatisticalData
		if err := builder.add(data.DataInf); err != nil {
			return nil, err
		}
		if start == 1 {
			total = data.ResultInf.TotalNumber
		} else if total != data.ResultInf.TotalNumber {
			return nil, errors.New("e-Stat: TOTAL_NUMBER changed between pages")
		}
		next := data.ResultInf.NextKey
		if next == 0 {
			break
		}
		if next <= start {
			return nil, fmt.Errorf("e-Stat: NEXT_KEY %d must be greater than startPosition %d", next, start)
		}
		start = next
		params.Set("startPosition", strconv.Itoa(start))
	}
	if count := len(builder.result()); count != total {
		return nil, fmt.Errorf("e-Stat: record count %d differs from TOTAL_NUMBER %d", count, total)
	}
	return builder.result(), nil
}

// getPage は、1回分の応答を読み、次の要求の前に本文を閉じる。
func (c *Client) getPage(ctx context.Context, params url.Values) (getStatsDataJSON, error) {
	var response getStatsDataResponse
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/getStatsData?"+params.Encode(), nil)
	if err != nil {
		return getStatsDataJSON{}, c.requestError(err)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return getStatsDataJSON{}, c.requestError(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return getStatsDataJSON{}, fmt.Errorf("e-Stat: HTTP status %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return getStatsDataJSON{}, fmt.Errorf("e-Stat: decode response: %w", err)
	}
	return response.GetStatsData, nil
}

// requestError は、元の URL をエラーの鎖に残さず、取り消しや時間切れの原因を引き継ぐ。
func (c *Client) requestError(err error) error {
	var original *url.Error
	if errors.As(err, &original) {
		safeURL := redacted
		if u, parseErr := url.Parse(original.URL); parseErr == nil {
			query := u.Query()
			query.Set("appId", redacted)
			u.RawQuery = query.Encode()
			safeURL = u.String()
		}
		return fmt.Errorf("e-Stat: request failed: %w", &url.Error{Op: original.Op, URL: safeURL, Err: original.Err})
	}
	return fmt.Errorf("e-Stat: request failed: %s", strings.ReplaceAll(err.Error(), string(*c.appID), redacted))
}
