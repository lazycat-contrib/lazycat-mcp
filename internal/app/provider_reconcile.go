package app

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	providerReconcileInterval    = 30 * time.Second
	providerReconcileGracePeriod = 30 * time.Second
)

func (a *App) startProviderReconciliation() {
	if a == nil || a.providers == nil || a.resources == nil {
		return
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	a.providerReconcileCancel = cancel
	go func() {
		ticker := time.NewTicker(providerReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.reconcileLazyCatProvidersBestEffort(loopCtx)
			case <-loopCtx.Done():
				return
			}
		}
	}()
}

func (a *App) reconcileLazyCatProvidersBestEffort(ctx context.Context) {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()

	a.upstreamRefreshMu.Lock()
	deleted, err := a.reconcileLazyCatProvidersAt(reconcileCtx, time.Now())
	a.upstreamRefreshMu.Unlock()
	if deleted > 0 {
		a.refreshUpstreamToolsBestEffort(reconcileCtx)
	}
	if err != nil {
		if a.logger != nil {
			a.logger.Warn().Err(err).Msg("skip stale provider cleanup")
		}
		return
	}
}

func (a *App) reconcileLazyCatProviders(ctx context.Context) (int, error) {
	return a.reconcileLazyCatProvidersAt(ctx, time.Now())
}

func (a *App) reconcileLazyCatProvidersAt(ctx context.Context, now time.Time) (int, error) {
	if a == nil || a.providers == nil || a.resources == nil {
		return 0, nil
	}
	index, err := a.resources.ScanForReconcile(ctx)
	if err != nil {
		a.missingProviderSince = make(map[int]time.Time)
		return 0, err
	}
	providers, err := a.providers.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list providers: %w", err)
	}
	if a.missingProviderSince == nil {
		a.missingProviderSince = make(map[int]time.Time)
	}
	seen := make(map[int]bool, len(providers))
	deleted := 0
	for _, provider := range providers {
		seen[provider.ID] = true
		resourceID := strings.TrimSpace(provider.ResourceID)
		if provider.Type != "lazycat" || provider.AppID == "" || provider.AppID == selfPackageID || resourceID == "" {
			delete(a.missingProviderSince, provider.ID)
			continue
		}
		if index.HasProviderResource(provider.AppID, resourceID) {
			delete(a.missingProviderSince, provider.ID)
			continue
		}
		missingSince, observed := a.missingProviderSince[provider.ID]
		if !observed {
			a.missingProviderSince[provider.ID] = now
			continue
		}
		if now.Sub(missingSince) < providerReconcileGracePeriod {
			continue
		}
		if a.logger != nil {
			a.logger.Info().
				Int("provider_id", provider.ID).
				Str("slug", provider.Slug).
				Str("app_id", provider.AppID).
				Str("resource_id", resourceID).
				Str("reason", "resource_missing").
				Msg("auto-deleting stale lazycat provider")
		}
		if err := a.providers.Delete(ctx, provider.ID); err != nil {
			return deleted, fmt.Errorf("delete stale provider %s: %w", provider.Slug, err)
		}
		delete(a.missingProviderSince, provider.ID)
		deleted++
	}
	for providerID := range a.missingProviderSince {
		if !seen[providerID] {
			delete(a.missingProviderSince, providerID)
		}
	}
	return deleted, nil
}
