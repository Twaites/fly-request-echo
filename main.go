// fly-request-echo: minimal HTTP server for inspecting what Fly.io (and clients)
// send to your app. Intended for lab / hypothesis testing only — do not expose
// sensitive payloads on the public internet without an access gate.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"
)

const maxBodyBytes = 2 << 20 // 2 MiB

var errBodyTooLarge = errors.New("body exceeds max size")

// snapshot is returned as JSON (and rendered as HTML when requested).
type snapshot struct {
	TimeUTC         string            `json:"time_utc"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestURI      string            `json:"request_uri"`
	Proto           string            `json:"proto"`
	Host            string            `json:"host"`
	RemoteAddr      string            `json:"remote_addr"`
	ContentLength   int64             `json:"content_length"`
	TransferEncoding []string         `json:"transfer_encoding,omitempty"`
	Headers         map[string]string `json:"headers"`
	TrailerKeys     []string          `json:"trailer_keys,omitempty"`
	Query           map[string]string `json:"query"`
	BodyUTF8        string            `json:"body_utf8,omitempty"`
	BodyBase64      string            `json:"body_base64,omitempty"`
	BodyLength      int               `json:"body_length"`
}

func main() {
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	secret := strings.TrimSpace(os.Getenv("ECHO_SECRET"))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if secret != "" {
			if !checkSecret(r, secret) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		snap, err := buildSnapshot(r)
		if err != nil {
			if errors.Is(err, errBodyTooLarge) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		wantHTML := strings.Contains(r.Header.Get("Accept"), "text/html") ||
			r.URL.Query().Get("format") == "html"
		if wantHTML {
			writeHTML(w, snap)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           logRequest(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	log.Printf("listening on %s (set ECHO_SECRET to require X-Echo-Secret or ?echo_secret=)", addr)
	log.Fatal(srv.ListenAndServe())
}

func checkSecret(r *http.Request, secret string) bool {
	if r.Header.Get("X-Echo-Secret") == secret {
		return true
	}
	if r.URL.Query().Get("echo_secret") == secret {
		return true
	}
	return false
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func buildSnapshot(r *http.Request) (*snapshot, error) {
	headers := make(map[string]string)
	for k, vs := range r.Header {
		// Join multi-value headers so nothing is hidden.
		headers[k] = strings.Join(vs, ", ")
	}

	query := make(map[string]string)
	for k, vs := range r.URL.Query() {
		if len(vs) == 1 {
			query[k] = vs[0]
			continue
		}
		query[k] = strings.Join(vs, ", ")
	}

	var bodyBuf bytes.Buffer
	if r.Body != nil {
		limited := io.LimitReader(r.Body, maxBodyBytes+1)
		n, err := io.Copy(&bodyBuf, limited)
		_ = r.Body.Close()
		if err != nil {
			return nil, err
		}
		if n > maxBodyBytes {
			return nil, errBodyTooLarge
		}
	}
	raw := bodyBuf.Bytes()

	s := &snapshot{
		TimeUTC:          time.Now().UTC().Format(time.RFC3339Nano),
		Method:           r.Method,
		URL:              r.URL.String(),
		RequestURI:       r.RequestURI,
		Proto:            r.Proto,
		Host:             r.Host,
		RemoteAddr:       r.RemoteAddr,
		ContentLength:    r.ContentLength,
		TransferEncoding: append([]string(nil), r.TransferEncoding...),
		Headers:          headers,
		Query:            query,
		BodyLength:       len(raw),
	}

	if len(r.Trailer) > 0 {
		for k := range r.Trailer {
			s.TrailerKeys = append(s.TrailerKeys, k)
		}
		sort.Strings(s.TrailerKeys)
	}

	if len(raw) == 0 {
		return s, nil
	}

	if utf8Safe := string(raw); isMostlyPrintable(utf8Safe) {
		s.BodyUTF8 = utf8Safe
	} else {
		s.BodyBase64 = base64.StdEncoding.EncodeToString(raw)
	}
	return s, nil
}

func isMostlyPrintable(s string) bool {
	n := len(s)
	if n == 0 {
		return true
	}
	bad := 0
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			bad++
		}
	}
	return bad*20 < n // tolerate a few control chars
}

var pageTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>fly-request-echo</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 1.5rem; max-width: 960px; }
    pre { background: #111; color: #e8e8e8; padding: 1rem; overflow: auto; border-radius: 8px; }
    h1 { font-size: 1.25rem; }
    p.muted { color: #666; }
  </style>
</head>
<body>
  <h1>fly-request-echo</h1>
  <p class="muted">Lab only. JSON: same URL with <code>Accept: application/json</code> or omit <code>?format=html</code>.</p>
  <pre>{{.}}</pre>
</body>
</html>`))

func writeHTML(w http.ResponseWriter, snap *snapshot) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = pageTmpl.Execute(w, string(b))
}
