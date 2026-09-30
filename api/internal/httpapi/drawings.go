package httpapi

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tony/lumen/api/internal/storage"
	"github.com/tony/lumen/api/internal/store"
)

// Excalidraw drawings. Signed-in users keep their own under /api/drawings; admins also see
// everyone's under /api/admin/drawings. Files never pass through this server: the browser saves
// the scene and a PNG preview straight to the media bucket (drawings/{id}-{token}/, created on
// first save) with presigned PUT URLs, and loads them back with short-lived presigned GET URLs.

const (
	drawingURLTTL     = time.Hour
	maxDrawingSize    = 50 << 20 // a scene with pasted images can be large; refuse anything absurd
	maxMemberDrawings = 100
)

type drawingOut struct {
	store.Drawing
	PreviewURL string `json:"previewUrl,omitempty"`
}

// drawingScope is which drawings a route works on: the signed-in user's own, or (studio) all.
type drawingScope struct{ all bool }

var (
	ownDrawings = drawingScope{all: false}
	allDrawings = drawingScope{all: true}
)

// needDrawings answers 503 when storage isn't configured.
func (s *Server) needDrawings(w http.ResponseWriter) bool {
	if s.r2 == nil {
		writeErr(w, http.StatusServiceUnavailable, "storage is not configured (R2 settings in api/.env)")
		return false
	}
	return true
}

// loadDrawing fetches {id} and checks the caller may use it; users only reach their own
// drawings (others answer 404, so ids can't be probed).
func (s *Server) loadDrawing(w http.ResponseWriter, r *http.Request, sc drawingScope) (*store.Drawing, bool) {
	if !s.needDrawings(w) {
		return nil, false
	}
	id, ok := pathUUID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return nil, false
	}
	d, err := s.store.GetDrawing(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	me, _ := currentUser(r)
	if !sc.all && !d.OwnedBy(me.ID) {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, false
	}
	return d, true
}

func (s *Server) previewURL(r *http.Request, d store.Drawing) string {
	if d.SavedAt == nil {
		return ""
	}
	u, err := s.r2.PresignGet(r.Context(), storage.DrawingPreviewKey(d.Folder()), drawingURLTTL)
	if err != nil {
		return ""
	}
	return u
}

// GET .../drawings?q=&page=
func (s *Server) listDrawings(sc drawingScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.needDrawings(w) {
			return
		}
		me, _ := currentUser(r)
		owner := me.ID
		if sc.all {
			owner = 0
		}
		page := pageParam(r, 0)
		list, total, err := s.store.DrawingsPage(r.Context(), owner, r.URL.Query().Get("q"), adminPageSize, (page-1)*adminPageSize)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		items := make([]drawingOut, len(list))
		for i, d := range list {
			items[i] = drawingOut{Drawing: d, PreviewURL: s.previewURL(r, d)}
		}
		writeJSON(w, http.StatusOK, Page[drawingOut]{Items: items, Page: page, TotalPages: pages(total, adminPageSize), Total: total})
	}
}

func drawingTitle(t string) (string, bool) {
	t = strings.TrimSpace(t)
	if t == "" {
		t = "Untitled drawing"
	}
	return t, utf8.RuneCountInString(t) <= 200
}

// POST .../drawings {title}: a new, empty drawing owned by the caller.
func (s *Server) createDrawing(w http.ResponseWriter, r *http.Request) {
	if !s.needDrawings(w) {
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	title, ok := drawingTitle(in.Title)
	if !ok {
		writeErr(w, http.StatusBadRequest, "the title is too long (200 characters max)")
		return
	}
	me, _ := currentUser(r)
	if me.Role != store.RoleAdmin {
		n, err := s.store.CountDrawings(r.Context(), me.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if n >= maxMemberDrawings {
			writeErr(w, http.StatusConflict, "you have 100 drawings, the most an account can keep; delete some to make room")
			return
		}
	}
	d, err := s.store.CreateDrawing(r.Context(), title, me.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, drawingOut{Drawing: *d})
}

// GET .../drawings/{id}: the drawing and, once saved, a URL to download its scene.
func (s *Server) getDrawing(sc drawingScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.loadDrawing(w, r, sc)
		if !ok {
			return
		}
		out := map[string]any{"drawing": d, "sceneUrl": ""}
		if d.SavedAt != nil {
			u, err := s.r2.PresignGet(r.Context(), storage.DrawingSceneKey(d.Folder()), drawingURLTTL)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			out["sceneUrl"] = u
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// PATCH .../drawings/{id} {title}
func (s *Server) renameDrawing(sc drawingScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.loadDrawing(w, r, sc)
		if !ok {
			return
		}
		var in struct {
			Title string `json:"title"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, http.StatusBadRequest, "bad request")
			return
		}
		title, ok := drawingTitle(in.Title)
		if !ok {
			writeErr(w, http.StatusBadRequest, "the title is too long (200 characters max)")
			return
		}
		renamed, err := s.store.RenameDrawing(r.Context(), d.ID, title)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, drawingOut{Drawing: *renamed, PreviewURL: s.previewURL(r, *renamed)})
	}
}

// POST .../drawings/{id}/uploads: presigned PUT URLs for the scene and the preview.
func (s *Server) drawingUploads(sc drawingScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.loadDrawing(w, r, sc)
		if !ok {
			return
		}
		scene, err := s.r2.PresignPut(r.Context(), storage.DrawingSceneKey(d.Folder()), "application/json", 15*time.Minute)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		preview, err := s.r2.PresignPut(r.Context(), storage.DrawingPreviewKey(d.Folder()), "image/png", 15*time.Minute)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"sceneUrl": scene, "previewUrl": preview})
	}
}

// POST .../drawings/{id}/saved: the browser finished uploading; record the save.
func (s *Server) drawingSaved(sc drawingScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.loadDrawing(w, r, sc)
		if !ok {
			return
		}
		size, found, err := s.r2.Size(r.Context(), storage.DrawingSceneKey(d.Folder()))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !found {
			writeErr(w, http.StatusConflict, "the drawing hasn't reached storage yet; save again")
			return
		}
		if size > maxDrawingSize {
			_ = s.r2.Delete(r.Context(), storage.DrawingSceneKey(d.Folder()))
			writeErr(w, http.StatusRequestEntityTooLarge, "the drawing is over 50 MB; remove some pasted images")
			return
		}
		saved, err := s.store.MarkDrawingSaved(r.Context(), d.ID, size)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, drawingOut{Drawing: *saved, PreviewURL: s.previewURL(r, *saved)})
	}
}

// DELETE .../drawings/{id}: removes the drawing and its files.
func (s *Server) deleteDrawing(sc drawingScope) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.loadDrawing(w, r, sc)
		if !ok {
			return
		}
		if err := s.r2.DeletePrefix(r.Context(), d.Folder()); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := s.store.DeleteDrawing(r.Context(), d.ID); err != nil {
			s.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
