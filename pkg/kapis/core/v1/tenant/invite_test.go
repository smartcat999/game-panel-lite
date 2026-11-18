package tenant

import (
	"context"
	"errors"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
)

type fakeInviteUserOp struct {
	usermodel.Operator
	existing map[string]bool
	created  []string
}

func (f *fakeInviteUserOp) Get(_ context.Context, name string) (*iamv1.User, error) {
	if f.existing[name] {
		return &iamv1.User{}, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeInviteUserOp) Create(_ context.Context, u *iamv1.User) (*iamv1.User, error) {
	f.created = append(f.created, u.Metadata.Name)
	return nil, errors.New("create should not be reached in this test")
}

func TestEnsureInviteAccountModes(t *testing.T) {
	h := &handler{}
	cases := []struct {
		name    string
		mode    corev1.TenantInviteMode
		user    string
		wantErr error
	}{
		{"login existing", corev1.TenantInviteModeLogin, "alice", nil},
		{"login missing never registers", corev1.TenantInviteModeLogin, "alicee", nil},
		{"login by email never registers", corev1.TenantInviteModeLogin, "alice@example.com", nil},
		{"register taken", corev1.TenantInviteModeRegister, "alice", models.ErrAlreadyExists},
		{"legacy existing", "", "alice", nil},
		{"unknown mode", "Magic", "alice", models.ErrBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := &fakeInviteUserOp{existing: map[string]bool{"alice": true}}
			err := h.ensureInviteAccount(context.Background(), op, &corev1.TenantInvite{Mode: tc.mode}, tc.user, "pw")
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
			if len(op.created) != 0 {
				t.Fatalf("account must not be created, got %v", op.created)
			}
		})
	}
}
