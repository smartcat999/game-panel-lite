package client

import (
	"context"
	"errors"
	"fmt"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// Apply declaratively applies an object: GET -> PUT (if 200 with matching ResourceVersion) | POST (if 404).
func (c *Client) Apply(ctx context.Context, obj metav1.Object) (*ApplyResult, error) {
	if obj == nil {
		return nil, errors.New("cannot apply nil object")
	}

	info, err := ResolveObject(obj)
	if err != nil {
		return nil, err
	}

	meta := obj.GetObjectMeta()
	if info.TenantScoped && meta.Tenant == "" {
		meta.Tenant = c.cfg.Tenant
	}
	if meta.Name == "" {
		return nil, fmt.Errorf("metadata.name is required for %s", info.Kind)
	}

	existing := info.NewObject()
	err = c.Get(ctx, info.Plural, meta.Tenant, meta.Name, existing)
	if err != nil {
		if IsNotFound(err) {
			created := info.NewObject()
			if err := c.Create(ctx, obj, created); err != nil {
				return nil, err
			}
			return &ApplyResult{
				Action:   ActionCreated,
				Resource: info,
				Object:   created,
			}, nil
		}
		return nil, err
	}

	// Update existing resource with current ResourceVersion
	meta.ResourceVersion = existing.GetObjectMeta().ResourceVersion
	updated := info.NewObject()
	if err := c.Update(ctx, obj, updated); err != nil {
		return nil, err
	}

	return &ApplyResult{
		Action:   ActionConfigured,
		Resource: info,
		Object:   updated,
	}, nil
}
