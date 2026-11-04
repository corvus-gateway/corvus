package api

import (
	"fmt"
	"net/http"

	"corvus/control/internal/compiler"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ streams (TCP/UDP)

type streamInput struct {
	IncomingPort  int    `json:"incoming_port"`
	TCP           *bool  `json:"tcp_forwarding"`
	UDP           bool   `json:"udp_forwarding"`
	ForwardHost   string `json:"forward_host"`
	ForwardPort   int    `json:"forward_port"`
	CertificateID *int64 `json:"certificate_id"`
	Enabled       *bool  `json:"enabled"`
}

func (in streamInput) apply(st *store.Stream) {
	st.IncomingPort, st.UDP, st.ForwardHost, st.ForwardPort, st.CertificateID = in.IncomingPort, in.UDP, in.ForwardHost, in.ForwardPort, in.CertificateID
	st.TCP = in.TCP == nil || *in.TCP
	st.Enabled = in.Enabled == nil || *in.Enabled
}

// checkStream validates a stream against the rest of the configuration (inside the tx).
func checkStream(r *http.Request, tx *store.Store, p compiler.Paths, st *store.Stream) error {
	if err := validate.Stream(st); err != nil {
		return err
	}
	ctx := r.Context()
	var errs validate.Errors
	if st.CertificateID != nil {
		// A pending certificate is allowed: the TCP listener starts once it is issued.
		if _, err := tx.CertificateByID(ctx, *st.CertificateID); err != nil {
			errs = append(errs, validate.FieldError{Field: "certificate_id", Code: "not_found", Message: "certificate does not exist"})
		}
	}
	others, err := tx.ListStreams(ctx)
	if err != nil {
		return err
	}
	if why := compiler.StreamConflict(p, others, st); why != "" {
		errs = append(errs, validate.FieldError{Field: "incoming_port", Code: "conflict", Message: why})
	}
	return errs.OrNil()
}

func (s *Server) listStreams(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListStreams(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) getStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	st, err := s.Store.StreamByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) createStream(w http.ResponseWriter, r *http.Request) {
	var in streamInput
	if !decode(w, r, &in) {
		return
	}
	st := &store.Stream{}
	in.apply(st)
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("create stream :%d", in.IncomingPort), func(tx *store.Store) error {
		if err := checkStream(r, tx, s.Paths, st); err != nil {
			return err
		}
		return tx.SaveStream(r.Context(), st)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "stream.create", fmt.Sprintf("stream:%d", st.ID), st)
	writeJSON(w, 201, map[string]any{"stream": st, "version": v})
}

func (s *Server) updateStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in streamInput
	if !decode(w, r, &in) {
		return
	}
	var before, st *store.Stream
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update stream #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.StreamByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		st = &cp
		in.apply(st)
		if err := checkStream(r, tx, s.Paths, st); err != nil {
			return err
		}
		return tx.SaveStream(r.Context(), st)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "stream.update", fmt.Sprintf("stream:%d", id), map[string]any{"before": before, "after": st})
	writeJSON(w, 200, map[string]any{"stream": st, "version": v})
}

func (s *Server) deleteStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.Stream
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete stream #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.StreamByID(r.Context(), id); err != nil {
			return err
		}
		return tx.DeleteStream(r.Context(), id)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "stream.delete", fmt.Sprintf("stream:%d", id), before)
	writeJSON(w, 200, map[string]any{"version": v})
}
