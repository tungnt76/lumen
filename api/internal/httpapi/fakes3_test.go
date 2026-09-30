package httpapi

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeS3 is an in-memory bucket speaking just enough of the S3 API (path style) for the handlers:
// PUT (incl. presigned and copy), GET, HEAD, DELETE, ListObjectsV2 and DeleteObjects.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte // "bucket/key" → body
	srv     *httptest.Server
}

func newFakeS3(t *testing.T) *fakeS3 {
	f := &fakeS3{objects: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeS3) keys(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func (f *fakeS3) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/")
	bucket, _, _ := strings.Cut(p, "/")
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodPut && r.Header.Get("X-Amz-Copy-Source") != "":
		src, _ := url.PathUnescape(strings.TrimPrefix(r.Header.Get("X-Amz-Copy-Source"), "/"))
		body, ok := f.objects[src]
		if !ok {
			http.Error(w, "<Error><Code>NoSuchKey</Code></Error>", http.StatusNotFound)
			return
		}
		f.objects[p] = append([]byte(nil), body...)
		fmt.Fprint(w, `<CopyObjectResult><ETag>"x"</ETag><LastModified>2026-01-01T00:00:00.000Z</LastModified></CopyObjectResult>`)
	case r.Method == http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.objects[p] = b
	case r.Method == http.MethodGet && q.Has("list-type"):
		type obj struct {
			Key  string
			Size int
		}
		res := struct {
			XMLName  xml.Name `xml:"ListBucketResult"`
			Name     string
			KeyCount int
			Contents []obj
		}{Name: bucket}
		for k, v := range f.objects {
			if rest, ok := strings.CutPrefix(k, bucket+"/"); ok && strings.HasPrefix(rest, q.Get("prefix")) {
				res.Contents = append(res.Contents, obj{rest, len(v)})
			}
		}
		res.KeyCount = len(res.Contents)
		xml.NewEncoder(w).Encode(res)
	case r.Method == http.MethodPost && q.Has("delete"):
		var req struct {
			Objects []struct{ Key string } `xml:"Object"`
		}
		xml.NewDecoder(r.Body).Decode(&req)
		for _, o := range req.Objects {
			delete(f.objects, bucket+"/"+o.Key)
		}
		fmt.Fprint(w, `<DeleteResult></DeleteResult>`)
	case r.Method == http.MethodDelete:
		delete(f.objects, p)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		body, ok := f.objects[p]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			if r.Method == http.MethodGet {
				fmt.Fprint(w, "<Error><Code>NoSuchKey</Code></Error>")
			}
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodGet {
			w.Write(body)
		}
	default:
		http.Error(w, "unsupported", http.StatusBadRequest)
	}
}
