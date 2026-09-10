package productionseed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/productioncatalog"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

const (
	WorkspaceID = "ws_ember"
	RegionID    = "reg_asia_east"
)

var seedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type GlobalConfig struct {
	AdminUsername string
	AdminName     string
	AdminPassword string
	FundingKey    []byte
	ProviderKey   []byte
}

type RegionConfig struct {
	PublicAddress string
	PortStart     int
	PortEnd       int
}

func SeedGlobal(ctx context.Context, database *sql.DB, config GlobalConfig) (string, error) {
	if config.AdminUsername == "" || config.AdminName == "" || len(config.AdminPassword) < 12 || len(config.FundingKey) < 32 || len(config.ProviderKey) < 32 {
		return "", errors.New("invalid global seed configuration")
	}
	store := authentication.NewPostgresStore(database)
	credential, err := store.CredentialByLogin(ctx, config.AdminUsername)
	var userID string
	switch {
	case err == nil:
		userID = credential.UserID
	case errors.Is(err, sql.ErrNoRows):
		sessions := authentication.NewSessionService(store, authentication.SessionPolicy{AbsoluteLifetime: 24 * time.Hour, IdleTimeout: time.Hour, ReauthWindow: 10 * time.Minute, SecureCookies: true})
		user, createErr := authentication.NewPasswordService(store, sessions).CreateLocalAccount(ctx, config.AdminUsername, config.AdminName, config.AdminPassword)
		if createErr != nil {
			return "", createErr
		}
		userID = user.ID
	default:
		return "", err
	}
	if err := ensureWorkspace(ctx, database); err != nil {
		return "", err
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO memberships (id,workspace_id,user_id,role,created_at) VALUES ('mbr_ember_owner',$1,$2,'owner',$3) ON CONFLICT (workspace_id,user_id) DO NOTHING`, WorkspaceID, userID, seedTime); err != nil {
		return "", err
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO workspace_selections (user_id,workspace_id,updated_at) VALUES ($1,$2,$3) ON CONFLICT (user_id) DO UPDATE SET workspace_id=EXCLUDED.workspace_id,updated_at=EXCLUDED.updated_at`, userID, WorkspaceID, seedTime); err != nil {
		return "", err
	}
	bindings := authorization.NewPostgresStore(database)
	for _, binding := range []authorization.RoleBinding{
		{ID: "rbd_bootstrap_platform_admin", PrincipalID: authorization.PrincipalID(userID), Role: authorization.RolePlatformAdmin, Scope: authorization.Scope{Type: authorization.ScopePlatform, ID: "platform"}},
		{ID: "rbd_bootstrap_workspace_owner", PrincipalID: authorization.PrincipalID(userID), Role: authorization.RoleWorkspaceOwner, Scope: authorization.Scope{Type: authorization.ScopeWorkspace, ID: WorkspaceID}},
		{ID: "rbd_bootstrap_region_operator", PrincipalID: authorization.PrincipalID(userID), Role: authorization.RoleRegionOperator, Scope: authorization.Scope{Type: authorization.ScopeRegion, ID: RegionID}},
	} {
		if err := bindings.Put(ctx, binding); err != nil {
			return "", err
		}
	}
	if err := ensureRegion(ctx, database); err != nil {
		return "", err
	}
	billingModule := billing.New(billing.NewPostgresStore(database), config.FundingKey)
	if err := billingModule.PublishCatalog(ctx, resourceCatalog()); err != nil {
		return "", err
	}
	if err := billingModule.PublishPriceBook(ctx, priceBook()); err != nil {
		return "", err
	}
	if _, err := billingModule.GrantPromotionalCredit(ctx, WorkspaceID, "initial-hosted-preview-credit", 100000, "initial hosted preview credit"); err != nil {
		return "", err
	}
	if _, _, err := productioncatalog.Publish(ctx, providercontract.NewPostgresStore(database), config.ProviderKey); err != nil {
		return "", err
	}
	return userID, nil
}

func SeedRegion(ctx context.Context, database *sql.DB, config RegionConfig) error {
	if config.PublicAddress == "" || config.PortStart < 1024 || config.PortEnd < config.PortStart || config.PortEnd > 65535 {
		return errors.New("invalid Region seed configuration")
	}
	result, err := database.ExecContext(ctx, `INSERT INTO endpoint_pools (id,region_id,delivery_mode,address,port_start,port_end,stability,active) VALUES ('epp_um773_node_direct',$1,'node-direct',$2,$3,$4,'stable',true) ON CONFLICT (id) DO NOTHING`, RegionID, config.PublicAddress, config.PortStart, config.PortEnd)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	var address string
	var start, end int
	if err := database.QueryRowContext(ctx, `SELECT address,port_start,port_end FROM endpoint_pools WHERE id='epp_um773_node_direct'`).Scan(&address, &start, &end); err != nil {
		return err
	}
	if address != config.PublicAddress || start != config.PortStart || end != config.PortEnd {
		return errors.New("immutable endpoint pool differs from seed configuration")
	}
	return nil
}

func ensureWorkspace(ctx context.Context, database *sql.DB) error {
	result, err := database.ExecContext(ctx, `INSERT INTO workspaces (id,slug,name,created_at) VALUES ($1,'ember','Ember Realms',$2) ON CONFLICT (id) DO NOTHING`, WorkspaceID, seedTime)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	var slug, name string
	if err := database.QueryRowContext(ctx, `SELECT slug,name FROM workspaces WHERE id=$1`, WorkspaceID).Scan(&slug, &name); err != nil {
		return err
	}
	if slug != "ember" || name != "Ember Realms" {
		return errors.New("immutable workspace differs from production seed")
	}
	return nil
}

func ensureRegion(ctx context.Context, database *sql.DB) error {
	result, err := database.ExecContext(ctx, `INSERT INTO regions (id,code,name,available,created_at) VALUES ($1,'asia-east','Asia East',true,$2) ON CONFLICT (id) DO NOTHING`, RegionID, seedTime)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	var code, name string
	var available bool
	if err := database.QueryRowContext(ctx, `SELECT code,name,available FROM regions WHERE id=$1`, RegionID).Scan(&code, &name, &available); err != nil {
		return err
	}
	if code != "asia-east" || name != "Asia East" || !available {
		return errors.New("immutable Region differs from production seed")
	}
	return nil
}

func resourceCatalog() billing.RegionCatalog {
	return billing.RegionCatalog{ID: "rrc_asia_east_v1", RegionID: RegionID, Version: 1, CPU: billing.Range{Minimum: 500, Maximum: 8000, Step: 500}, Memory: billing.Range{Minimum: 1024, Maximum: 16384, Step: 512}, Disk: billing.Range{Minimum: 10, Maximum: 100, Step: 5}, EndpointDeliveryModes: []string{"node-direct"}, Availability: billing.AvailabilityAvailable, EffectiveAt: seedTime, CreatedAt: seedTime}
}

func priceBook() billing.PriceBook {
	return billing.PriceBook{ID: "prb_asia_east_v1", RegionID: RegionID, Revision: 1, Currency: billing.CurrencyCNY, UnitPrices: []billing.UnitPrice{{ResourceKind: billing.ResourceCPU, PriceMinor: 2, UnitQuantity: 1000, Unit: "milli-core-hour"}, {ResourceKind: billing.ResourceMemory, PriceMinor: 1, UnitQuantity: 1024, Unit: "MiB-hour"}, {ResourceKind: billing.ResourceInstanceDisk, PriceMinor: 1, UnitQuantity: 10, Unit: "GiB-hour"}, {ResourceKind: billing.ResourceBackupStorage, PriceMinor: 1, UnitQuantity: 10, Unit: "GiB-hour"}, {ResourceKind: billing.ResourceDedicatedIP, PriceMinor: 0, UnitQuantity: 1, Unit: "address-hour"}}, EffectiveAt: seedTime, CreatedAt: seedTime}
}

func Summary(userID string) string {
	return fmt.Sprintf("user=%s workspace=%s region=%s", userID, WorkspaceID, RegionID)
}
