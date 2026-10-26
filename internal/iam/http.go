package iam

import (
	"net/http"

	"github.com/lakehouse-platform/lp/internal/api"
	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/store"
)

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/tenants", s.handleCreateTenant)
	mux.HandleFunc("GET /api/v1/tenants", s.handleListTenants)
	mux.HandleFunc("POST /api/v1/tenants/{tenant_id}/workspaces", s.handleCreateWorkspace)
	mux.HandleFunc("GET /api/v1/tenants/{tenant_id}/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.FromContext(r.Context())
		api.WriteJSON(w, http.StatusOK, p)
	})
}

func (s *Service) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          string              `json:"name"`
		IsolationTier store.IsolationTier `json:"isolationTier"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.WriteError(w, r, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	t, err := s.CreateTenant(r.Context(), p, req.Name, req.IsolationTier)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, t)
}

func (s *Service) handleListTenants(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	ts, err := s.ListTenants(r.Context(), p)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"tenants": ts})
}

func (s *Service) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.WriteError(w, r, err)
		return
	}
	p, _ := auth.FromContext(r.Context())
	ws, err := s.CreateWorkspace(r.Context(), p, r.PathValue("tenant_id"), req.Name)
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, ws)
}

func (s *Service) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	ws, err := s.ListWorkspaces(r.Context(), p, r.PathValue("tenant_id"))
	if err != nil {
		api.WriteError(w, r, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"workspaces": ws})
}
