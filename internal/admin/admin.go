package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/anujdhakrey/load-balancer/internal/pool"
)

type Server struct {
	pool   *pool.Pool
	mux    *http.ServeMux
	logger *slog.Logger
}

type addBackendRequest struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

type removeBackendRequest struct {
	URL string `json:"url"`
}

type apiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func NewServer(p *pool.Pool, logger *slog.Logger) *Server {
	s := &Server{
		pool:   p,
		mux:    http.NewServeMux(),
		logger: logger,
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /backends", s.handleListBackends)
	s.mux.HandleFunc("POST /backends", s.handleAddBackend)
	s.mux.HandleFunc("DELETE /backends", s.handleRemoveBackend)
	s.mux.HandleFunc("POST /backends/drain", s.handleDrainBackend)
	s.mux.HandleFunc("GET /stats", s.handleStats)
	s.mux.HandleFunc("GET /health", s.handleHealth)
}

func (s *Server) handleListBackends(w http.ResponseWriter, r *http.Request) {
	stats := s.pool.Stats()
	writeJSON(w, http.StatusOK, apiResponse{
		Status: "ok",
		Data:   stats,
	})
}

func (s *Server) handleAddBackend(w http.ResponseWriter, r *http.Request) {
	var req addBackendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			Status:  "error",
			Message: "invalid request body",
		})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			Status:  "error",
			Message: "url is required",
		})
		return
	}

	if err := s.pool.AddBackend(req.URL, req.Weight); err != nil {
		writeJSON(w, http.StatusConflict, apiResponse{
			Status:  "error",
			Message: err.Error(),
		})
		return
	}

	s.logger.Info("backend added via admin API", "url", req.URL, "weight", req.Weight)
	writeJSON(w, http.StatusCreated, apiResponse{
		Status:  "ok",
		Message: "backend added",
	})
}

func (s *Server) handleRemoveBackend(w http.ResponseWriter, r *http.Request) {
	var req removeBackendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			Status:  "error",
			Message: "invalid request body",
		})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			Status:  "error",
			Message: "url is required",
		})
		return
	}

	if err := s.pool.RemoveBackend(req.URL); err != nil {
		writeJSON(w, http.StatusNotFound, apiResponse{
			Status:  "error",
			Message: err.Error(),
		})
		return
	}

	s.logger.Info("backend removed via admin API", "url", req.URL)
	writeJSON(w, http.StatusOK, apiResponse{
		Status:  "ok",
		Message: "backend removed",
	})
}

func (s *Server) handleDrainBackend(w http.ResponseWriter, r *http.Request) {
	var req removeBackendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			Status:  "error",
			Message: "invalid request body",
		})
		return
	}

	if err := s.pool.DrainBackend(req.URL); err != nil {
		writeJSON(w, http.StatusNotFound, apiResponse{
			Status:  "error",
			Message: err.Error(),
		})
		return
	}

	s.logger.Info("backend draining via admin API", "url", req.URL)
	writeJSON(w, http.StatusOK, apiResponse{
		Status:  "ok",
		Message: "backend set to draining",
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.pool.Stats()
	writeJSON(w, http.StatusOK, apiResponse{
		Status: "ok",
		Data: map[string]interface{}{
			"total_backends":   s.pool.Len(),
			"healthy_backends": s.pool.HealthyCount(),
			"backends":         stats,
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	healthy := s.pool.HealthyCount()
	if healthy == 0 {
		writeJSON(w, http.StatusServiceUnavailable, apiResponse{
			Status:  "error",
			Message: "no healthy backends",
		})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{
		Status:  "ok",
		Message: "load balancer healthy",
		Data: map[string]int{
			"healthy_backends": healthy,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
