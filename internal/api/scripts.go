package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *Server) listScripts(w http.ResponseWriter, r *http.Request) {
	if _, apiErr := s.requireAuth(r, "admin", "operator"); apiErr != nil {
		writeError(w, r, apiErr)
		return
	}
	root, err := filepath.Abs(s.Config.ScriptRoot)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	root, _ = filepath.EvalSymlinks(root)
	requested := r.URL.Query().Get("path")
	target := filepath.Join(root, requested)
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		writeError(w, r, notFound("script directory not found"))
		return
	}
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		writeError(w, r, validationErr("path escapes script root"))
		return
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		writeError(w, r, notFound("script directory not found"))
		return
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		writeError(w, r, internalError("internal server error"))
		return
	}
	sort.SliceStable(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	items := make([]ScriptEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		full := filepath.Join(target, entry.Name())
		relative, _ := filepath.Rel(root, full)
		item := ScriptEntry{Name: entry.Name(), Path: filepath.ToSlash(relative), Type: "file"}
		if entry.IsDir() {
			item.Type = "dir"
		} else if info, err := entry.Info(); err == nil {
			size := int(info.Size())
			item.Size = &size
		}
		items = append(items, item)
	}
	relativePath, _ := filepath.Rel(root, target)
	var parent *string
	if target != root {
		parentPath, _ := filepath.Rel(root, filepath.Dir(target))
		if parentPath == "." {
			parentPath = ""
		}
		parentPath = filepath.ToSlash(parentPath)
		parent = &parentPath
	}
	writeJSON(w, 200, ScriptList{
		Root: root, Path: filepath.ToSlash(relativePath), Parent: parent, Items: items,
	})
}
