package nodeapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/serviceauth"
)

type NodeState interface {
	RegionID() string
	StartRegionalNodeSession(context.Context, string) (regional.NodeSession, error)
	RecordRegionalNodeHeartbeat(context.Context, string, regional.NodeHeartbeat) error
}

func NewHandler(state NodeState, identities *serviceauth.Nodes, maxBytes int64) (http.Handler, error) {
	if state == nil || identities == nil || maxBytes < 1 || state.RegionID() != identities.RegionID() {
		return nil, errors.New("invalid regional node API configuration")
	}
	serve := func(session bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			node, err := identities.Authenticate(r)
			if err != nil {
				http.Error(w, "node identity required", 401)
				return
			}
			if r.URL.RawQuery != "" {
				http.Error(w, "unexpected query", 400)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			defer r.Body.Close()
			var result regional.NodeSession
			if session {
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil || len(body) != 0 {
					http.Error(w, "session request must have no body", 400)
					return
				}
				result, err = state.StartRegionalNodeSession(r.Context(), node)
			} else {
				media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if mediaErr != nil || media != "application/json" {
					http.Error(w, "JSON required", 415)
					return
				}
				heartbeat, decodeErr := decodeHeartbeat(r.Body)
				if decodeErr != nil {
					var limit *http.MaxBytesError
					if errors.As(decodeErr, &limit) {
						http.Error(w, "request too large", 413)
					} else {
						http.Error(w, "invalid heartbeat", 400)
					}
					return
				}
				err = state.RecordRegionalNodeHeartbeat(r.Context(), node, heartbeat)
			}
			switch {
			case errors.Is(err, regional.ErrNodeUnavailable):
				http.Error(w, "node unavailable", 404)
			case errors.Is(err, regional.ErrNodeHeartbeatStale):
				http.Error(w, "stale heartbeat", 409)
			case errors.Is(err, regional.ErrInvalidNode):
				http.Error(w, "invalid heartbeat", 400)
			case err != nil:
				http.Error(w, "node state unavailable", 503)
			case session:
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(result)
			default:
				w.WriteHeader(http.StatusNoContent)
			}
		}
	}
	router := chi.NewRouter()
	router.Post("/internal/node/session", serve(true))
	router.Post("/internal/node/heartbeat", serve(false))
	return router, nil
}

func decodeHeartbeat(body io.Reader) (regional.NodeHeartbeat, error) {
	var heartbeat regional.NodeHeartbeat
	decoder := json.NewDecoder(body)
	token, err := decoder.Token()
	if err != nil {
		return heartbeat, err
	}
	if token != json.Delim('{') {
		return heartbeat, regional.ErrInvalidNode
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return heartbeat, err
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return heartbeat, regional.ErrInvalidNode
		}
		seen[name] = true
		switch name {
		case "sessionEpoch":
			err = decoder.Decode(&heartbeat.SessionEpoch)
		case "sequence":
			err = decoder.Decode(&heartbeat.Sequence)
		case "architecture":
			err = decoder.Decode(&heartbeat.Architecture)
		case "runtimeReady":
			var ready *bool
			err = decoder.Decode(&ready)
			if err == nil {
				if ready == nil {
					err = regional.ErrInvalidNode
				} else {
					heartbeat.RuntimeReady = *ready
				}
			}
		default:
			return heartbeat, regional.ErrInvalidNode
		}
		if err != nil {
			return heartbeat, err
		}
	}
	if _, err = decoder.Token(); err != nil {
		return heartbeat, err
	}
	if len(seen) != 4 || heartbeat.SessionEpoch < 1 || heartbeat.Sequence < 1 || heartbeat.Architecture == "" {
		return heartbeat, regional.ErrInvalidNode
	}
	if _, err = decoder.Token(); err != io.EOF {
		if err == nil {
			err = regional.ErrInvalidNode
		}
		return heartbeat, err
	}
	return heartbeat, nil
}
