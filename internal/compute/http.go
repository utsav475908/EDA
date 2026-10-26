package compute

import (
	"net/http"

	"github.com/lakehouse-platform/lp/internal/api"
	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/store"
)

func (s *Service) Register(mux *http.ServeMux) {
	const base = "/api/v1/workspaces/{workspace_id}/clusters"
	mux.HandleFunc("POST "+base, s.handleCreate)
	mux.HandleFunc("GET "+base, s.handleList)
	mux.HandleFunc("GET "+base+"/{cluster_id}", s.handleGet)
	mux.HandleFunc("DELETE "+base+"/{cluster_id}", s.handleTerminate)
	mux.HandleFunc("POST "+base+"/{cluster_id}/heartbeat", s.handleHeartbeat)
}

func (s *Service) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cluster store.Cluster `json:"cluster"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.WriteError(w, r, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	c, err := s.CreateCluster(r.Context(), p, r.PathValue("workspace_id"), req.Cluster)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, c)
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	cs, err := s.ListClusters(r.Context(), p, r.PathValue("workspace_id"))
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"clusters": cs})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	c, err := s.GetCluster(r.Context(), p, r.PathValue("workspace_id"), r.PathValue("cluster_id"))
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, c)
}

func (s *Service) handleTerminate(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	if err := s.TerminateCluster(r.Context(), p, r.PathValue("workspace_id"), r.PathValue("cluster_id")); err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{})
}

func (s *Service) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	if err := s.Heartbeat(r.Context(), p, r.PathValue("workspace_id"), r.PathValue("cluster_id")); err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{})
}
