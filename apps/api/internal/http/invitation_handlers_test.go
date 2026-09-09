package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestNodePublicDomainJoinInfo(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "join_info.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create compute node with PublicDomain
	node := domain.ComputeNode{
		ID:           "node-domain-1",
		Name:         "Shanghai Domain Node",
		Host:         "10.0.0.5",
		Port:         8080,
		PublicIP:     "124.223.1.2",
		PublicDomain: "shanghai-1.playgames.cloud",
		Region:       "Shanghai",
		Status:       "online",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		store:    db,
		provider: mustRegistry(t),
		cfg:      config.Config{DataDir: t.TempDir()},
	}

	server := domain.GameServer{
		ID:          "srv-domain-test",
		NodeID:      node.ID,
		Name:        "Master Terraria",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			Network: domain.ServerNetworkSpec{
				Port:     7777,
				HostPort: 7777,
			},
		},
	}

	joinInfo := h.serverJoinInfo(server)
	if joinInfo.Address != "shanghai-1.playgames.cloud" {
		t.Fatalf("expected domain address 'shanghai-1.playgames.cloud', got '%s'", joinInfo.Address)
	}
	if joinInfo.Port != 7777 {
		t.Fatalf("expected port 7777, got %d", joinInfo.Port)
	}
}

func TestOrganizationInvitationHTTPFlow(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "invitation_flow.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create owner user
	owner := domain.AdminAccount{
		ID:        "usr-owner",
		Username:  "squad_leader",
		Role:      domain.RoleMember,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.CreateAdminAccount(ctx, &owner); err != nil {
		t.Fatal(err)
	}

	// Create friend user
	friend := domain.AdminAccount{
		ID:        "usr-friend",
		Username:  "gamer_bro",
		Role:      domain.RoleMember,
		CreatedAt: time.Now().UTC(),
	}
	if err := db.CreateAdminAccount(ctx, &friend); err != nil {
		t.Fatal(err)
	}

	// Create organization
	org := domain.Organization{
		ID:        "org-squad-http",
		Name:      "Dream Team",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.CreateOrganization(ctx, &org, owner.ID); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		store:      db,
		provider:   mustRegistry(t),
		apiMetrics: metrics.NewRegistry(),
		cfg:        config.Config{DataDir: t.TempDir()},
	}

	router := chi.NewRouter()
	h.Register(router)

	// Helper to attach authenticated context
	withAuth := func(req *http.Request, acc domain.AdminAccount) *http.Request {
		return req.WithContext(context.WithValue(req.Context(), authAccountContextKey, acc))
	}

	var inviteToken string
	var inviteID string

	// 1. Owner creates invitation
	t.Run("owner creates invitation", func(t *testing.T) {
		body, _ := json.Marshal(map[string]any{
			"role":       "member",
			"maxUses":    3,
			"expireDays": 7,
		})
		req := httptest.NewRequest("POST", "/api/organizations/"+org.ID+"/invitations", bytes.NewReader(body))
		req = withAuth(req, owner)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}

		var inv domain.OrganizationInvitation
		if err := json.Unmarshal(rec.Body.Bytes(), &inv); err != nil {
			t.Fatal(err)
		}
		if inv.Token == "" || inv.OrganizationID != org.ID {
			t.Fatalf("unexpected invitation: %+v", inv)
		}
		inviteToken = inv.Token
		inviteID = inv.ID
	})

	// 2. Anonymous visits invitation info
	t.Run("public gets invitation summary", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/invitations/"+inviteToken, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var summary domain.InvitationSummary
		if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
			t.Fatal(err)
		}
		if summary.OrganizationName != "Dream Team" || summary.InviterName != "squad_leader" {
			t.Fatalf("unexpected summary: %+v", summary)
		}
	})

	// 3. Friend accepts invitation
	t.Run("friend accepts invitation", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/invitations/"+inviteToken+"/accept", nil)
		req = withAuth(req, friend)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var result map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["status"] != "accepted" {
			t.Fatalf("expected status accepted, got %v", result["status"])
		}
	})

	// 4. Friend can now list organization members
	t.Run("friend lists organization members", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/organizations/"+org.ID+"/members", nil)
		req = withAuth(req, friend)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var members []organizationMemberDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &members); err != nil {
			t.Fatal(err)
		}
		if len(members) != 2 {
			t.Fatalf("expected 2 members (owner + friend), got %d", len(members))
		}
	})

	// 5. Owner revokes invitation
	t.Run("owner revokes invitation", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/organizations/"+org.ID+"/invitations/"+inviteID, nil)
		req = withAuth(req, owner)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}

		// Trying to accept revoked invitation should now fail
		stranger := domain.AdminAccount{ID: "usr-stranger", Username: "stranger"}
		reqAccept := httptest.NewRequest("POST", "/api/invitations/"+inviteToken+"/accept", nil)
		reqAccept = withAuth(reqAccept, stranger)
		recAccept := httptest.NewRecorder()
		router.ServeHTTP(recAccept, reqAccept)

		if recAccept.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for revoked invitation, got %d", recAccept.Code)
		}
	})
}
