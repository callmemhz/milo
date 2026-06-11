package store

import (
	"context"
	"time"

	"github.com/callmemhz/milo/internal/store/sqlcgen"
)

type Link = sqlcgen.Link

// AppLink is a link seen from the app side, joined with the linked addon.
type AppLink = sqlcgen.ListLinksForAppRow

// AddonLink is a link seen from the addon side, joined with the linked app.
type AddonLink = sqlcgen.ListLinksForAddonRow

func (s *Store) CreateLink(ctx context.Context, appID, addonID int64, alias string) (Link, error) {
	return s.Q.CreateLink(ctx, sqlcgen.CreateLinkParams{
		AppID:     appID,
		AddonID:   addonID,
		Alias:     alias,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) GetLink(ctx context.Context, appID, addonID int64) (Link, error) {
	return s.Q.GetLink(ctx, sqlcgen.GetLinkParams{AppID: appID, AddonID: addonID})
}

func (s *Store) DeleteLink(ctx context.Context, appID, addonID int64) error {
	return s.Q.DeleteLink(ctx, sqlcgen.DeleteLinkParams{AppID: appID, AddonID: addonID})
}

func (s *Store) DeleteLinksForApp(ctx context.Context, appID int64) error {
	return s.Q.DeleteLinksForApp(ctx, appID)
}

func (s *Store) DeleteLinksForAddon(ctx context.Context, addonID int64) error {
	return s.Q.DeleteLinksForAddon(ctx, addonID)
}

func (s *Store) ListLinksForApp(ctx context.Context, appID int64) ([]AppLink, error) {
	return s.Q.ListLinksForApp(ctx, appID)
}

func (s *Store) ListLinksForAddon(ctx context.Context, addonID int64) ([]AddonLink, error) {
	return s.Q.ListLinksForAddon(ctx, addonID)
}
