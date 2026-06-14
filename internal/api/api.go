// Package api exposes the reputation HTTP endpoints.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/shimpa1/akash-reputation/internal/adr036"
	"github.com/shimpa1/akash-reputation/internal/store"
)

// LeaseVerifier confirms a lease identity exists on-chain (live fallback).
type LeaseVerifier interface {
	VerifyLeaseLive(ctx context.Context, owner, dseq, gseq, oseq string) (bool, error)
}

// Server holds handler dependencies.
type Server struct {
	Store    *store.Store
	Verifier LeaseVerifier
	Log      *slog.Logger
	Provider string // this instance's provider address (anchors ratings)
	ChainID  string // chain id wallets should use for signing, e.g. akashnet-2
}

// Routes returns the configured HTTP handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /info", s.handleInfo)
	mux.HandleFunc("POST /feedback", s.handlePostFeedback)
	mux.HandleFunc("GET /feedback", s.handleListFeedback)
	mux.HandleFunc("GET /reputation/{address}", s.handleReputation)
	mux.HandleFunc("GET /leases", s.handleListLeases)
	return mux
}

// handleInfo exposes this instance's provider address and chain id so a generic
// frontend can configure itself without baking in instance values at build time.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"provider": s.Provider,
		"chain_id": s.ChainID,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// feedbackRequest is the signed submission envelope. PubKey and Signature are
// standard base64. The feedback fields are the canonical message that was signed.
type feedbackRequest struct {
	Feedback  adr036.Feedback `json:"feedback"`
	PubKey    string          `json:"pubkey"`
	Signature string          `json:"signature"`
}

func (s *Server) handlePostFeedback(w http.ResponseWriter, r *http.Request) {
	var req feedbackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	f := req.Feedback
	if err := f.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(f.Comment) > 1000 {
		writeError(w, http.StatusBadRequest, "comment too long (max 1000 chars)")
		return
	}
	issuedAt, err := time.Parse(time.RFC3339, f.IssuedAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "issued_at must be RFC3339")
		return
	}

	pubKey, err := base64.StdEncoding.DecodeString(req.PubKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "pubkey must be base64")
		return
	}
	sig, err := base64.StdEncoding.DecodeString(req.Signature)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signature must be base64")
		return
	}

	// 1. Cryptographic attribution: signature must verify and recover to author.
	author, err := adr036.Verify(f, pubKey, sig)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "signature verification failed: "+err.Error())
		return
	}

	// 2. Earned: the two parties must actually have had this lease.
	ok, err := s.Store.LeaseExists(r.Context(), f.Deployer, f.DSeq, f.GSeq, f.OSeq, f.Provider)
	if err != nil {
		s.Log.Error("lease lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "lease lookup failed")
		return
	}
	if !ok && s.Verifier != nil {
		// Cache miss — confirm directly against the chain before rejecting.
		ok, err = s.Verifier.VerifyLeaseLive(r.Context(), f.Deployer, f.DSeq, f.GSeq, f.OSeq)
		if err != nil {
			s.Log.Warn("live lease verify failed", "err", err)
		}
		if ok {
			_ = s.Store.UpsertLease(r.Context(), store.Lease{
				Owner: f.Deployer, DSeq: f.DSeq, GSeq: f.GSeq, OSeq: f.OSeq,
				Provider: f.Provider, State: "verified",
			})
		}
	}
	if !ok {
		writeError(w, http.StatusUnprocessableEntity,
			"no lease found between this provider and deployer for the given deployment")
		return
	}

	// 3. One rating per deployment per author (enforced by unique index).
	rec := store.FeedbackRecord{
		Role:        string(f.Role),
		AuthorAddr:  author,
		SubjectAddr: f.Subject(),
		Provider:    f.Provider,
		Owner:       f.Deployer,
		DSeq:        f.DSeq,
		GSeq:        f.GSeq,
		OSeq:        f.OSeq,
		Score:       f.Score,
		Comment:     f.Comment,
		IssuedAt:    issuedAt,
		PubKey:      req.PubKey,
		Signature:   req.Signature,
	}
	id, err := s.Store.InsertFeedback(r.Context(), rec)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "feedback already recorded for this deployment")
			return
		}
		s.Log.Error("insert feedback failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not store feedback")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "author": author, "subject": f.Subject(), "score": f.Score,
	})
}

func (s *Server) handleReputation(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("address")
	if _, err := adr036.DecodeAddress(addr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid akash address")
		return
	}
	rep, err := s.Store.Reputation(r.Context(), addr)
	if err != nil {
		s.Log.Error("reputation query failed", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleListFeedback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	recs, err := s.Store.ListFeedback(r.Context(), q.Get("subject"), q.Get("author"), q.Get("role"))
	if err != nil {
		s.Log.Error("list feedback failed", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": recs})
}

func (s *Server) handleListLeases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ls, err := s.Store.ListLeases(r.Context(), q.Get("provider"), q.Get("owner"))
	if err != nil {
		s.Log.Error("list leases failed", "err", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"leases": ls})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
