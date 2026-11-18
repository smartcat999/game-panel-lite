package v1

import (
	"context"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// SessionInterface has methods to work with Session resources.
type SessionInterface interface {
	Login(ctx context.Context, identifier, password string) (*iamv1.Session, error)
	IssueNodeToken(ctx context.Context, nodeName string) (*iamv1.Session, error)
	Create(ctx context.Context, session *iamv1.Session) (*iamv1.Session, error)
	Get(ctx context.Context, name string) (*iamv1.Session, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.Session], error)
	Delete(ctx context.Context, name string) error
}

type sessions struct {
	rest rest.Interface
}

func newSessions(r rest.Interface) *sessions {
	return &sessions{rest: r}
}

func (c *sessions) Login(ctx context.Context, identifier, password string) (*iamv1.Session, error) {
	req := &iamv1.Session{
		Identifier: identifier,
		Password:   password,
	}
	return c.Create(ctx, req)
}

func (c *sessions) IssueNodeToken(ctx context.Context, nodeName string) (*iamv1.Session, error) {
	req := &iamv1.Session{
		Kind:       "Node",
		Identifier: nodeName,
	}
	return c.Create(ctx, req)
}

func (c *sessions) Create(ctx context.Context, session *iamv1.Session) (*iamv1.Session, error) {
	var out iamv1.Session
	if err := c.rest.Create(ctx, session, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *sessions) Get(ctx context.Context, name string) (*iamv1.Session, error) {
	var out iamv1.Session
	if err := c.rest.Get(ctx, "sessions", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *sessions) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[iamv1.Session], error) {
	var out metav1.List[iamv1.Session]
	if err := c.rest.List(ctx, "sessions", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *sessions) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "sessions", "", name)
}
