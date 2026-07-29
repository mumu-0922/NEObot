package memoryauthor

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	reviewSessionCookie = "mm_memory_author_session"
	maximumReviewBody   = 2 << 20
)

type ReviewServer struct {
	root           string
	reviewerID     string
	clock          func() time.Time
	listener       net.Listener
	httpServer     *http.Server
	serveDone      chan error
	expectedHost   string
	expectedOrigin string
	bootstrapToken string
	sessionToken   string
	csrfToken      string
	pageNonce      string
	bootstrapMu    sync.Mutex
	bootstrapUsed  bool
}

type ReviewServerOptions struct {
	Root       string
	ReviewerID string
	Clock      func() time.Time
	Listener   net.Listener
}

type reviewCaseResponse struct {
	Index         int          `json:"index"`
	Total         int          `json:"total"`
	Sequence      uint64       `json:"sequence"`
	Decision      Decision     `json:"decision"`
	ContentSHA256 string       `json:"contentSha256"`
	ReviewerID    string       `json:"reviewerId,omitempty"`
	ReviewedAt    string       `json:"reviewedAt,omitempty"`
	Snapshot      CaseSnapshot `json:"snapshot"`
}

type reviewActionRequest struct {
	Action                ReviewAction  `json:"action"`
	CaseID                string        `json:"caseId"`
	ExpectedSequence      uint64        `json:"expectedSequence"`
	ExpectedContentSHA256 string        `json:"expectedContentSha256"`
	Snapshot              *CaseSnapshot `json:"snapshot,omitempty"`
}

func StartReviewServer(options ReviewServerOptions) (*ReviewServer, error) {
	if !validUUID(strings.TrimSpace(options.ReviewerID)) {
		return nil, errors.New("reviewer ID must be an explicit UUID")
	}
	if frozenExists(options.Root) {
		return nil, errors.New("frozen corpus cannot start a review server")
	}
	if _, err := LoadReviewState(options.Root); err != nil {
		return nil, err
	}
	listener := options.Listener
	var err error
	if listener == nil {
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return nil, errors.New("listen on loopback for review UI")
		}
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !tcpAddress.IP.IsLoopback() {
		_ = listener.Close()
		return nil, errors.New("review UI listener must be loopback-only")
	}
	bootstrap, err := randomToken(32)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	session, err := randomToken(32)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	nonce, err := randomToken(18)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	clock := options.Clock
	if clock == nil {
		clock = time.Now
	}
	host := listener.Addr().String()
	server := &ReviewServer{
		root: options.Root, reviewerID: strings.TrimSpace(options.ReviewerID), clock: clock,
		listener: listener, expectedHost: host, expectedOrigin: "http://" + host,
		bootstrapToken: bootstrap, sessionToken: session, csrfToken: csrf, pageNonce: nonce,
		serveDone: make(chan error, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.handlePage)
	mux.HandleFunc("POST /session", server.handleSession)
	mux.HandleFunc("GET /api/status", server.requireSession(server.handleStatus))
	mux.HandleFunc("GET /api/case", server.requireSession(server.handleCase))
	mux.HandleFunc("POST /api/action", server.requireSession(server.handleAction))
	server.httpServer = &http.Server{
		Handler:           server.securityBoundary(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	go func() {
		err := server.httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		server.serveDone <- err
		close(server.serveDone)
	}()
	return server, nil
}

func (server *ReviewServer) URL() string {
	return server.expectedOrigin + "/#" + url.PathEscape(server.bootstrapToken)
}

func (server *ReviewServer) Done() <-chan error {
	return server.serveDone
}

func (server *ReviewServer) Close(ctx context.Context) error {
	if server == nil || server.httpServer == nil {
		return nil
	}
	return server.httpServer.Shutdown(ctx)
}

func (server *ReviewServer) securityBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setReviewHeaders(writer.Header(), server.pageNonce)
		if request.Host != server.expectedHost {
			http.Error(writer, "invalid host", http.StatusForbidden)
			return
		}
		host, _, err := net.SplitHostPort(request.RemoteAddr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			http.Error(writer, "loopback client required", http.StatusForbidden)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			if request.Header.Get("Origin") != server.expectedOrigin {
				http.Error(writer, "invalid origin", http.StatusForbidden)
				return
			}
		}
		if frozenExists(server.root) && request.URL.Path != "/" {
			http.Error(writer, "corpus is frozen", http.StatusConflict)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (server *ReviewServer) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(reviewSessionCookie)
		if err != nil || !constantTimeEqual(cookie.Value, server.sessionToken) {
			http.Error(writer, "review session required", http.StatusUnauthorized)
			return
		}
		if request.Method != http.MethodGet && !constantTimeEqual(request.Header.Get("X-CSRF-Token"), server.csrfToken) {
			http.Error(writer, "invalid csrf token", http.StatusForbidden)
			return
		}
		next(writer, request)
	}
}

func (server *ReviewServer) handlePage(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := reviewPage.Execute(writer, struct{ Nonce string }{Nonce: server.pageNonce}); err != nil {
		http.Error(writer, "render review page", http.StatusInternalServerError)
	}
}

func (server *ReviewServer) handleSession(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := decodeRequestJSON(request, &payload); err != nil {
		http.Error(writer, "invalid session request", http.StatusBadRequest)
		return
	}
	server.bootstrapMu.Lock()
	defer server.bootstrapMu.Unlock()
	if server.bootstrapUsed || !constantTimeEqual(payload.Token, server.bootstrapToken) {
		http.Error(writer, "invalid session token", http.StatusForbidden)
		return
	}
	server.bootstrapUsed = true
	http.SetCookie(writer, &http.Cookie{
		Name: reviewSessionCookie, Value: server.sessionToken, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(writer, http.StatusOK, struct {
		CSRFToken string `json:"csrfToken"`
		Reviewer  string `json:"reviewerId"`
	}{CSRFToken: server.csrfToken, Reviewer: server.reviewerID})
}

func (server *ReviewServer) handleStatus(writer http.ResponseWriter, _ *http.Request) {
	status, err := CurrentStatus(server.root)
	if err != nil {
		http.Error(writer, "load review status", http.StatusConflict)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (server *ReviewServer) handleCase(writer http.ResponseWriter, request *http.Request) {
	state, err := LoadReviewState(server.root)
	if err != nil {
		http.Error(writer, "load review state", http.StatusConflict)
		return
	}
	rawIndex := request.URL.Query().Get("index")
	index := -1
	if rawIndex == "pending" {
		for candidateIndex, candidate := range state.Cases {
			if candidate.Decision == DecisionPending {
				index = candidateIndex
				break
			}
		}
		if index < 0 {
			index = 0
		}
	} else {
		index, err = strconv.Atoi(rawIndex)
		if err != nil {
			http.Error(writer, "invalid case index", http.StatusBadRequest)
			return
		}
	}
	if index < 0 || index >= len(state.Cases) {
		http.Error(writer, "case index out of range", http.StatusBadRequest)
		return
	}
	item := state.Cases[index]
	writeJSON(writer, http.StatusOK, reviewCaseResponse{
		Index: index, Total: len(state.Cases), Sequence: state.LastSequence,
		Decision: item.Decision, ContentSHA256: item.ContentSHA256,
		ReviewerID: item.ReviewerID, ReviewedAt: item.ReviewedAt, Snapshot: item.Snapshot,
	})
}

func (server *ReviewServer) handleAction(writer http.ResponseWriter, request *http.Request) {
	var payload reviewActionRequest
	if err := decodeRequestJSON(request, &payload); err != nil {
		http.Error(writer, "invalid review action", http.StatusBadRequest)
		return
	}
	result, err := ApplyReview(server.root, ReviewInput{
		Action: payload.Action, CaseID: payload.CaseID,
		ExpectedSequence:      payload.ExpectedSequence,
		ExpectedContentSHA256: payload.ExpectedContentSHA256,
		ReviewerID:            server.reviewerID, Snapshot: payload.Snapshot, Clock: server.clock,
	})
	if err != nil {
		http.Error(writer, "review action rejected: "+boundedError(err), http.StatusConflict)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func decodeRequestJSON(request *http.Request, target any) error {
	if request.Header.Get("Content-Type") != "application/json" {
		return errors.New("content type is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maximumReviewBody+1))
	if err != nil {
		return err
	}
	if len(body) == 0 || len(body) > maximumReviewBody {
		return errors.New("request body size is invalid")
	}
	return strictjson.Decode(body, maximumReviewBody, target)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(writer, "encode response", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(body, '\n'))
}

func setReviewHeaders(header http.Header, nonce string) {
	header.Set("Cache-Control", "no-store, max-age=0")
	header.Set("Pragma", "no-cache")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	header.Set("Content-Security-Policy", "default-src 'none'; connect-src 'self'; style-src 'nonce-"+nonce+"'; script-src 'nonce-"+nonce+"'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
}

func randomToken(size int) (string, error) {
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		return "", errors.New("generate secure review token")
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func constantTimeEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func boundedError(err error) string {
	value := err.Error()
	if len(value) > 240 {
		return value[:240]
	}
	return value
}

var reviewPage = template.Must(template.New("review").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Memory Benchmark Review</title>
  <style nonce="{{.Nonce}}">
    :root { color-scheme: dark; font-family: Inter, ui-sans-serif, system-ui, sans-serif; background:#0b0d10; color:#f2f0e8; }
    * { box-sizing:border-box; }
    body { margin:0; min-height:100vh; background:#0b0d10; }
    main { width:min(1180px, calc(100% - 40px)); margin:32px auto 64px; }
    header { display:flex; gap:20px; justify-content:space-between; align-items:flex-end; margin-bottom:24px; }
    h1 { margin:0; font-size:clamp(28px,4vw,54px); letter-spacing:-.04em; font-weight:620; }
    .meta { color:#9ba3ad; font:13px ui-monospace,monospace; }
    .panel { border:1px solid #2a2f36; border-radius:18px; background:#12151a; padding:20px; }
    .toolbar { display:flex; flex-wrap:wrap; gap:10px; align-items:center; margin-bottom:16px; }
    button { border:1px solid #3a414b; border-radius:999px; padding:10px 16px; background:#171b21; color:#f2f0e8; cursor:pointer; font-weight:650; }
    button:hover { border-color:#8ca0b8; }
    button.accept { background:#173d31; border-color:#2c7b61; }
    button.reject { background:#492426; border-color:#8a4448; }
    button.save { background:#263a55; border-color:#496f9c; }
    input { width:92px; border:1px solid #343b45; border-radius:10px; padding:10px; background:#0d1014; color:#fff; }
    textarea { width:100%; min-height:60vh; resize:vertical; border:1px solid #303740; border-radius:14px; background:#090b0e; color:#dce5ef; padding:18px; font:13px/1.55 ui-monospace,SFMono-Regular,monospace; }
    .state { margin-left:auto; padding:7px 11px; border-radius:999px; background:#20252c; font:12px ui-monospace,monospace; }
    .error { color:#ff9b9b; min-height:1.5em; white-space:pre-wrap; }
    .hint { color:#8f98a4; font-size:12px; margin:12px 2px 0; }
  </style>
</head>
<body>
<main>
  <header><div><div class="meta">LOOPBACK / HUMAN AUTHORITY</div><h1>Memory Golden Review</h1></div><div id="progress" class="meta">连接中…</div></header>
  <section class="panel">
    <div class="toolbar">
      <button id="prev">← 上一条</button><input id="index" type="number" min="1" value="1"><button id="go">跳转</button><button id="next">下一条 →</button><button id="pending">第一条待审</button>
      <button id="accept" class="accept">接受 · Alt+A</button><button id="reject" class="reject">拒绝 · Alt+R</button><button id="save" class="save">保存修改 · Alt+S</button>
      <span id="state" class="state">pending</span>
    </div>
    <textarea id="snapshot" spellcheck="false" aria-label="case and fixture JSON"></textarea>
    <div id="error" class="error"></div>
    <div class="hint">修改只保存内容并使旧审核失效；修改后必须再次单独接受或拒绝。工具没有批量审批。</div>
  </section>
</main>
<script nonce="{{.Nonce}}">
(() => {
  "use strict";
  let csrf = "", current = null, currentIndex = 0, loadedSnapshot = "";
  const el = id => document.getElementById(id);
  const request = async (path, options = {}) => {
    options.headers = Object.assign({}, options.headers || {}, csrf ? {"X-CSRF-Token": csrf} : {});
    const response = await fetch(path, options);
    const text = await response.text();
    if (!response.ok) throw new Error(text.trim() || ("HTTP " + response.status));
    return text ? JSON.parse(text) : null;
  };
  const connect = async () => {
    const token = decodeURIComponent(location.hash.slice(1));
    history.replaceState(null, "", "/");
    const session = await request("/session", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({token})});
    csrf = session.csrfToken;
    await load("pending");
  };
  const load = async index => {
    current = await request("/api/case?index=" + index);
    currentIndex = current.index;
    el("index").value = current.index + 1;
    loadedSnapshot = JSON.stringify(current.snapshot, null, 2);
    el("snapshot").value = loadedSnapshot;
    el("state").textContent = current.decision;
    el("progress").textContent = (current.index + 1) + " / " + current.total + " · sequence " + current.sequence;
    el("error").textContent = "";
  };
  const act = async (action, includeSnapshot) => {
    if (!current) return;
    let snapshot;
    const normalized = JSON.stringify(JSON.parse(el("snapshot").value), null, 2);
    if (includeSnapshot) snapshot = JSON.parse(normalized);
    if (!includeSnapshot && normalized !== loadedSnapshot) throw new Error("存在未保存修改，请先保存或重新加载当前条目。");
    await request("/api/action", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({
      action, caseId:current.snapshot.case.id, expectedSequence:current.sequence,
      expectedContentSha256:current.contentSha256, snapshot
    })});
    await load(includeSnapshot ? currentIndex : Math.min(currentIndex + 1, current.total - 1));
  };
  const guarded = fn => async () => { try { await fn(); } catch (error) { el("error").textContent = error.message; } };
  el("prev").onclick = guarded(() => load(Math.max(0, currentIndex - 1)));
  el("next").onclick = guarded(() => load(Math.min(current.total - 1, currentIndex + 1)));
  el("pending").onclick = guarded(() => load("pending"));
  el("go").onclick = guarded(() => load(Math.max(0, Math.min(current.total - 1, Number(el("index").value) - 1))));
  el("accept").onclick = guarded(() => act("accept", false));
  el("reject").onclick = guarded(() => act("reject", false));
  el("save").onclick = guarded(() => act("edit", true));
  addEventListener("keydown", event => {
    if (!event.altKey) return;
    if (event.key.toLowerCase() === "a") { event.preventDefault(); el("accept").click(); }
    if (event.key.toLowerCase() === "r") { event.preventDefault(); el("reject").click(); }
    if (event.key.toLowerCase() === "s") { event.preventDefault(); el("save").click(); }
  });
  guarded(connect)();
})();
</script>
</body>
</html>`))
