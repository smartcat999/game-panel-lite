package http

import (
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestFriendInviteAndPublicHostFlow(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("invite-server", cfg.DataDir)
	server.GameKey = domain.GameTerraria
	server.ProviderKey = domain.ProviderTerrariaVanilla
	server.Port = 7777
	server.HostPort = 17777
	createTestServer(t, db, server)

	joinInfo := httptest.NewRecorder()
	router.ServeHTTP(joinInfo, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/invite-server/join-info", nil))
	if joinInfo.Code != stdhttp.StatusOK {
		t.Fatalf("expected join-info 200, got %d: %s", joinInfo.Code, joinInfo.Body.String())
	}
	var info domain.ServerJoinInfo
	if err := json.Unmarshal(joinInfo.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Port != 17777 || !strings.Contains(info.InviteText, "127.0.0.1:17777") {
		t.Fatalf("expected default join info, got %+v", info)
	}

	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/settings/public-host", bytes.NewBufferString(`{"publicHost":"play.example.com"}`)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected public host update 200, got %d: %s", update.Code, update.Body.String())
	}

	joinInfoAfter := httptest.NewRecorder()
	router.ServeHTTP(joinInfoAfter, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/invite-server/join-info", nil))
	var infoAfter domain.ServerJoinInfo
	if err := json.Unmarshal(joinInfoAfter.Body.Bytes(), &infoAfter); err != nil {
		t.Fatal(err)
	}
	if infoAfter.Address != "play.example.com" || !strings.Contains(infoAfter.InviteText, "play.example.com:17777") {
		t.Fatalf("expected public host in join info, got %+v", infoAfter)
	}

	settings := httptest.NewRecorder()
	router.ServeHTTP(settings, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	var settingsResp map[string]string
	if err := json.Unmarshal(settings.Body.Bytes(), &settingsResp); err != nil {
		t.Fatal(err)
	}
	if settingsResp["publicHost"] != "play.example.com" {
		t.Fatalf("expected settings to expose public host, got %+v", settingsResp)
	}
}

func TestSettingsLocaleCanBeStoredInBackend(t *testing.T) {
	router, _, _ := newTestRouter(t)

	readDefault := httptest.NewRecorder()
	router.ServeHTTP(readDefault, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	if readDefault.Code != stdhttp.StatusOK {
		t.Fatalf("expected settings read 200, got %d: %s", readDefault.Code, readDefault.Body.String())
	}
	var defaults map[string]string
	if err := json.Unmarshal(readDefault.Body.Bytes(), &defaults); err != nil {
		t.Fatal(err)
	}
	if defaults["locale"] != "zh" {
		t.Fatalf("expected default locale zh, got %+v", defaults)
	}

	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/settings/locale", bytes.NewBufferString(`{"locale":"en"}`)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected locale update 200, got %d: %s", update.Code, update.Body.String())
	}

	readUpdated := httptest.NewRecorder()
	router.ServeHTTP(readUpdated, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	var updated map[string]string
	if err := json.Unmarshal(readUpdated.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated["locale"] != "en" {
		t.Fatalf("expected saved locale en, got %+v", updated)
	}

	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(stdhttp.MethodPut, "/api/settings/locale", bytes.NewBufferString(`{"locale":"fr"}`)))
	if invalid.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected invalid locale 400, got %d: %s", invalid.Code, invalid.Body.String())
	}
}

func TestShareableServerPageFlow(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("share-server", cfg.DataDir)
	server.GameKey = domain.GameTerraria
	server.ProviderKey = domain.ProviderTerrariaVanilla
	server.Password = "secret"
	server.Config.Password = "secret"
	server.HostPort = 17778
	createTestServer(t, db, server)

	enable := httptest.NewRecorder()
	router.ServeHTTP(enable, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/share-server/share", bytes.NewBufferString(`{"includePassword":false}`)))
	if enable.Code != stdhttp.StatusOK {
		t.Fatalf("expected share enable 200, got %d: %s", enable.Code, enable.Body.String())
	}
	var share serverShareResponse
	if err := json.Unmarshal(enable.Body.Bytes(), &share); err != nil {
		t.Fatal(err)
	}
	if !share.Enabled || share.Token == "" || share.SharePath != "/share/"+share.Token {
		t.Fatalf("expected enabled share response, got %+v", share)
	}
	getShare := httptest.NewRecorder()
	router.ServeHTTP(getShare, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/share-server/share", nil))
	if getShare.Code != stdhttp.StatusOK {
		t.Fatalf("expected share status 200, got %d: %s", getShare.Code, getShare.Body.String())
	}
	var shareStatus serverShareResponse
	if err := json.Unmarshal(getShare.Body.Bytes(), &shareStatus); err != nil {
		t.Fatal(err)
	}
	if shareStatus.Token != share.Token {
		t.Fatalf("expected share status to return token %s, got %+v", share.Token, shareStatus)
	}

	public := httptest.NewRecorder()
	router.ServeHTTP(public, httptest.NewRequest(stdhttp.MethodGet, "/api/public/servers/"+share.Token, nil))
	if public.Code != stdhttp.StatusOK {
		t.Fatalf("expected public share 200, got %d: %s", public.Code, public.Body.String())
	}
	var publicResp publicServerShareResponse
	if err := json.Unmarshal(public.Body.Bytes(), &publicResp); err != nil {
		t.Fatal(err)
	}
	if publicResp.Name != server.Name || publicResp.JoinInfo.Port != 17778 {
		t.Fatalf("expected public join info, got %+v", publicResp)
	}
	if publicResp.JoinInfo.Password != "" || strings.Contains(publicResp.JoinInfo.InviteText, "secret") {
		t.Fatalf("expected public share to hide password, got %+v", publicResp.JoinInfo)
	}

	enableWithPassword := httptest.NewRecorder()
	router.ServeHTTP(enableWithPassword, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/share-server/share", bytes.NewBufferString(`{"includePassword":true}`)))
	if enableWithPassword.Code != stdhttp.StatusOK {
		t.Fatalf("expected share update 200, got %d: %s", enableWithPassword.Code, enableWithPassword.Body.String())
	}
	var updatedShare serverShareResponse
	if err := json.Unmarshal(enableWithPassword.Body.Bytes(), &updatedShare); err != nil {
		t.Fatal(err)
	}
	if updatedShare.Token != share.Token || !updatedShare.IncludePassword {
		t.Fatalf("expected same share token with password enabled, got %+v", updatedShare)
	}

	publicWithPassword := httptest.NewRecorder()
	router.ServeHTTP(publicWithPassword, httptest.NewRequest(stdhttp.MethodGet, "/api/public/servers/"+share.Token, nil))
	var publicWithPasswordResp publicServerShareResponse
	if err := json.Unmarshal(publicWithPassword.Body.Bytes(), &publicWithPasswordResp); err != nil {
		t.Fatal(err)
	}
	if publicWithPasswordResp.JoinInfo.Password != "secret" {
		t.Fatalf("expected public share to include password after opt-in, got %+v", publicWithPasswordResp.JoinInfo)
	}

	disable := httptest.NewRecorder()
	router.ServeHTTP(disable, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/share-server/share", nil))
	if disable.Code != stdhttp.StatusOK {
		t.Fatalf("expected share disable 200, got %d: %s", disable.Code, disable.Body.String())
	}
	getDisabledShare := httptest.NewRecorder()
	router.ServeHTTP(getDisabledShare, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/share-server/share", nil))
	var disabledStatus serverShareResponse
	if err := json.Unmarshal(getDisabledShare.Body.Bytes(), &disabledStatus); err != nil {
		t.Fatal(err)
	}
	if disabledStatus.Enabled {
		t.Fatalf("expected disabled share status, got %+v", disabledStatus)
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(stdhttp.MethodGet, "/api/public/servers/"+share.Token, nil))
	if missing.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected disabled share 404, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestSettingsEndpointReadsConfiguredDockerHost(t *testing.T) {
	router, _, _ := newTestRouter(t)

	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(stdhttp.MethodGet, "/api/settings", nil))
	if read.Code != stdhttp.StatusOK {
		t.Fatalf("expected settings read 200, got %d: %s", read.Code, read.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["dockerHost"] != "unix:///initial.sock" {
		t.Fatalf("expected initial docker host, got %q", got["dockerHost"])
	}

	body := bytes.NewBufferString(`{"dockerHost":"unix:///updated.sock"}`)
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/settings", body))
	if update.Code != stdhttp.StatusMethodNotAllowed {
		t.Fatalf("expected settings update to be unavailable, got %d: %s", update.Code, update.Body.String())
	}
}
