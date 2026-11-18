package user

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

type fakeUserDeletionStore struct {
	mu      sync.Mutex
	user    *iamv1.User
	rv      int64
	onGet   func(call int, s *fakeUserDeletionStore)
	getCall int
}

func (s *fakeUserDeletionStore) Get(_ context.Context, _ string) (*iamv1.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user == nil {
		return nil, db.ErrNotFound
	}
	// Snapshot before onGet so the hook models a concurrent write landing between Get and UpdateObject.
	snapshot := cloneTestUser(s.user)
	s.getCall++
	if s.onGet != nil {
		s.onGet(s.getCall, s)
	}
	return snapshot, nil
}

func (s *fakeUserDeletionStore) UpdateObject(_ context.Context, in *iamv1.User) (*iamv1.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user == nil {
		return nil, db.ErrNotFound
	}
	if in.Metadata.ResourceVersion != strconv.FormatInt(s.rv, 10) {
		return nil, db.ErrConflict
	}
	s.rv++
	cp := cloneTestUser(in)
	cp.Metadata.ResourceVersion = strconv.FormatInt(s.rv, 10)
	s.user = cp
	return cloneTestUser(s.user), nil
}

func (s *fakeUserDeletionStore) UpdateStatus(_ context.Context, in *iamv1.User) (*iamv1.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user == nil {
		return nil, db.ErrNotFound
	}
	s.user.Status = in.Status
	return cloneTestUser(s.user), nil
}

type fakeSessionRevoker struct {
	revoked []string
}

func (r *fakeSessionRevoker) RevokeByUser(_ context.Context, username string) error {
	r.revoked = append(r.revoked, username)
	return nil
}

func cloneTestUser(u *iamv1.User) *iamv1.User {
	cp := *u
	cp.Metadata.Finalizers = slices.Clone(u.Metadata.Finalizers)
	cp.Metadata.Annotations = maps.Clone(u.Metadata.Annotations)
	cp.Metadata.Labels = maps.Clone(u.Metadata.Labels)
	return &cp
}

func newTestUser(uid string, deleting bool) *iamv1.User {
	u := &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: "alice", UID: uid, ResourceVersion: "1"},
		Spec:     iamv1.UserSpec{Email: "alice@example.invalid", State: iamv1.UserStateActive},
		Status:   iamv1.UserStatus{Phase: iamv1.UserStateActive},
	}
	if deleting {
		now := time.Now().UTC()
		u.Metadata.DeletionTimestamp = &now
		u.Metadata.Finalizers = []string{iamv1.FinalizerUserCleanup}
		u.Spec.State = iamv1.UserStateDisabled
		u.Status.Phase = iamv1.UserStateDisabled
	}
	return u
}

func TestMarkUserDeletingRetriesCASConflictAndRejectsUIDSwap(t *testing.T) {
	store := &fakeUserDeletionStore{
		user: newTestUser("uid-1", false),
		rv:   1,
		onGet: func(call int, s *fakeUserDeletionStore) {
			if call == 1 {
				setUserAnnotation(s.user, iamv1.AnnotationPersonalTenantUID, "tenant-uid-1")
				s.rv++
				s.user.Metadata.ResourceVersion = strconv.FormatInt(s.rv, 10)
			}
		},
	}
	got, err := markUserDeleting(context.Background(), store, "alice")
	if err != nil {
		t.Fatalf("expected CAS conflict retry to succeed, got %v", err)
	}
	if store.getCall < 2 {
		t.Fatalf("expected at least 2 Get calls across CAS retry, got %d", store.getCall)
	}
	if got.Metadata.DeletionTimestamp == nil || got.Spec.State != iamv1.UserStateDisabled {
		t.Fatalf("expected user marked disabled and deleting: %+v", got)
	}

	swapStore := &fakeUserDeletionStore{
		user: newTestUser("uid-1", false),
		rv:   1,
		onGet: func(call int, s *fakeUserDeletionStore) {
			if call == 1 {
				s.rv++
				s.user.Metadata.UID = "uid-2"
				s.user.Metadata.ResourceVersion = strconv.FormatInt(s.rv, 10)
			}
		},
	}
	if _, err := markUserDeleting(context.Background(), swapStore, "alice"); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict on UID swap, got %v", err)
	}
}

func TestRevokeDeletingUserWithStoreHandlesControllerRaces(t *testing.T) {
	ctx := context.Background()
	revoker := &fakeSessionRevoker{}
	casStore := &fakeUserDeletionStore{
		user: newTestUser("uid-1", true),
		rv:   2,
		onGet: func(call int, s *fakeUserDeletionStore) {
			if call == 1 {
				setUserAnnotation(s.user, iamv1.AnnotationPersonalTenantUID, "tenant-uid-1")
				s.rv++
				s.user.Metadata.ResourceVersion = strconv.FormatInt(s.rv, 10)
			}
		},
	}
	if err := revokeDeletingUserWithStore(ctx, casStore, newTestUser("uid-1", true), revoker); err != nil {
		t.Fatalf("expected retry convergence on concurrent UpdateObject, got %v", err)
	}
	if casStore.getCall < 2 || casStore.user.Metadata.Annotations[iamv1.AnnotationSessionsRevoked] != "uid-1" {
		t.Fatalf("expected CAS retry to persist sessions-revoked annotation, calls=%d", casStore.getCall)
	}

	alreadyStore := &fakeUserDeletionStore{
		user: newTestUser("uid-1", true),
		rv:   3,
	}
	setUserAnnotation(alreadyStore.user, iamv1.AnnotationSessionsRevoked, "uid-1")
	if err := revokeDeletingUserWithStore(ctx, alreadyStore, newTestUser("uid-1", true), revoker); err != nil {
		t.Fatalf("expected nil when concurrent reconciler already recorded revocation, got %v", err)
	}

	deletedStore := &fakeUserDeletionStore{user: nil}
	if err := revokeDeletingUserWithStore(ctx, deletedStore, newTestUser("uid-1", true), revoker); err != nil {
		t.Fatalf("expected nil when user was already finalized after revocation, got %v", err)
	}

	swapStore := &fakeUserDeletionStore{user: newTestUser("uid-2", true), rv: 2}
	if err := revokeDeletingUserWithStore(ctx, swapStore, newTestUser("uid-1", true), revoker); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("expected ErrConflict on UID swap, got %v", err)
	}
}
