package user

import (
	"context"
	"errors"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

const userResyncBatchSize = 200

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	Revoker      usermodel.SessionRevoker
	mu           sync.Mutex
	resyncOffset int
}

func (c *Controller) Name() string  { return "00-user-personal-tenant" }
func (c *Controller) Topic() string { return mq.TopicUser }

// Resync inspects a bounded page of Users per pass
// and emits reconcile events only for Users whose Personal Tenant or status is out of sync.
func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	c.mu.Lock()
	offset := c.resyncOffset
	c.mu.Unlock()

	users, err := userdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
		Limit:          userResyncBatchSize,
		Offset:         offset,
		SkipTotalCount: true,
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(users.Items) == userResyncBatchSize {
		c.resyncOffset = offset + userResyncBatchSize
	} else {
		c.resyncOffset = 0
	}
	c.mu.Unlock()

	tenantDB := tenantdb.NewDB(reg.Pool())
	var events []mq.Event
	for _, u := range users.Items {
		if needsUserReconcile(ctx, tenantDB, u) {
			ev := mq.NewEvent(mq.EventReconcile, iamv1.GroupVersion, "User", "", u.Metadata.Name, u.Metadata.Generation, u.Metadata.ResourceVersion)
			ev.Topic = mq.TopicUser
			events = append(events, ev)
		}
	}
	return events, nil
}

func needsUserReconcile(ctx context.Context, tenantDB *tenantdb.DB, u *iamv1.User) bool {
	if u == nil {
		return false
	}
	expectedPhase := u.Spec.State
	if expectedPhase == "" {
		expectedPhase = iamv1.UserStateActive
	}
	if u.Metadata.DeletionTimestamp != nil || u.Status.ObservedGeneration < u.Metadata.Generation || u.Status.Phase != expectedPhase {
		return true
	}
	if usermodel.PersonalTenantDisabled(u) {
		return false
	}
	if u.Metadata.Annotations[iamv1.AnnotationPersonalTenantUID] == "" || u.Status.DefaultTenant == "" {
		return true
	}
	if tenantDB != nil {
		if _, err := tenantDB.Get(ctx, u.Metadata.Name); errors.Is(err, db.ErrNotFound) {
			return true
		}
	}
	return false
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	if event.Type == mq.EventDeleted || event.Name == "" {
		return nil
	}
	userDB := userdb.NewDB(reg.Pool())
	u, err := userDB.Get(ctx, event.Name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if uid := event.Attributes["uid"]; uid != "" && uid != u.Metadata.UID {
		return nil
	}
	if u.Metadata.DeletionTimestamp != nil {
		return c.reconcileDeletion(ctx, reg, bus, u)
	}
	if err := usermodel.EnsurePersonalTenantForUID(ctx, reg, u.Metadata.Name, u.Metadata.UID); err != nil {
		return err
	}
	cur, gErr := userDB.Get(ctx, event.Name)
	if gErr != nil {
		if errors.Is(gErr, db.ErrNotFound) {
			return nil
		}
		return gErr
	}
	if cur.Metadata.UID != u.Metadata.UID {
		return db.ErrConflict
	}
	if cur.Metadata.DeletionTimestamp != nil {
		return c.reconcileDeletion(ctx, reg, bus, cur)
	}
	expectedPhase := cur.Spec.State
	if expectedPhase == "" {
		expectedPhase = iamv1.UserStateActive
	}
	disabled := usermodel.PersonalTenantDisabled(cur)
	needsDefaultTenant := !disabled && cur.Status.DefaultTenant == ""
	if cur.Status.ObservedGeneration < cur.Metadata.Generation || needsDefaultTenant || cur.Status.Phase != expectedPhase {
		cur.Status.ObservedGeneration = cur.Metadata.Generation
		if needsDefaultTenant {
			cur.Status.DefaultTenant = cur.Metadata.Name
		}
		cur.Status.Phase = expectedPhase
		if _, uErr := userDB.UpdateStatus(ctx, cur); uErr != nil {
			return uErr
		}
		if bus != nil && !disabled {
			_ = bus.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", u.Metadata.Name, 0, ""))
		}
	}
	return nil
}

func (c *Controller) reconcileDeletion(ctx context.Context, reg *repo.Registry, bus mq.Interface, user *iamv1.User) error {
	revoker := c.Revoker
	if revoker == nil && user.Metadata.Annotations[iamv1.AnnotationSessionsRevoked] != user.Metadata.UID {
		cacheStore := cache.Default()
		if cacheStore == nil {
			var err error
			cacheStore, err = cache.Open(ctx, nil)
			if err != nil {
				return err
			}
		}
		revoker = sessionmodel.NewOperator(reg, cacheStore, nil)
	}
	return usermodel.ReconcileDeletion(ctx, reg, bus, user.Metadata.Name, user.Metadata.UID, revoker)
}
