package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

type fakePrewarmDriver struct {
	runtime.Driver
	ready bool
	err   error
}

func (f *fakePrewarmDriver) Handler() string { return runtime.HandlerDocker }
func (f *fakePrewarmDriver) Version() string { return "test" }
func (f *fakePrewarmDriver) PrePullImage(_ context.Context, _ string) (bool, error) {
	return f.ready, f.err
}

func TestPrePullSingleImageRecordsOnlyReady(t *testing.T) {
	cases := []struct {
		name      string
		ready     bool
		err       error
		wantErr   bool
		wantWarms int
	}{
		{name: "not ready", ready: false, wantErr: true, wantWarms: 0},
		{name: "pull error", ready: false, err: errors.New("denied"), wantErr: true, wantWarms: 0},
		{name: "ready", ready: true, wantErr: false, wantWarms: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := runtime.NewDefaultRegistry(t.TempDir())
			reg.RegisterDriver(&fakePrewarmDriver{ready: tc.ready, err: tc.err})
			ag := &Agent{drivers: reg}
			err := ag.prePullSingleImage(context.Background(), "gamepanel/runtime:latest")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if got := len(ag.snapshotWarmImages()); got != tc.wantWarms {
				t.Fatalf("warmImages=%d want %d", got, tc.wantWarms)
			}
		})
	}
}
