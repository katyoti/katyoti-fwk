// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// Handler serves what the server keeps over HTTP, for the surfaces that do
// not sit on the bus (the phone, the approval server, the sandboxed iframe):
//
//	GET /panels                      the panels kept (?owner=)
//	GET /panels/{id}                 a panel's snapshot
//	GET /panels/{id}/history?from=N  a panel's log after N
//
// It reads only; whoever mounts it decides who may reach it.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /panels", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.List(r.Context(), panelapi.ListInput{Owner: r.URL.Query().Get("owner")})
		answer(w, out, err)
	})
	mux.HandleFunc("GET /panels/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Snapshot(r.Context(), panelapi.SnapshotInput{Panel: r.PathValue("id")})
		answer(w, out, err)
	})
	mux.HandleFunc("GET /panels/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		in := panelapi.HistoryInput{Panel: r.PathValue("id")}
		if v := r.URL.Query().Get("from"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				http.Error(w, "from is a sequence number", http.StatusBadRequest)
				return
			}
			in.From = n
		}
		out, err := s.History(r.Context(), in)
		answer(w, out, err)
	})
	return mux
}

func answer(w http.ResponseWriter, v any, err error) {
	switch {
	case errors.Is(err, ErrUnknownPanel):
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
}
