package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/spf13/cobra"
)

// nolint:wrapcheck
func ListVersionsCmd() *cobra.Command {
	var (
		appID         int64
		bundleID      string
		platformValue string
		withMetadata  bool
		concurrency   int
	)

	cmd := &cobra.Command{
		Use:   "list-versions",
		Short: "List the available versions of an App Store app",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appID == 0 && bundleID == "" {
				return errors.New("either the app ID or the bundle identifier must be specified")
			}

			if concurrency < 1 {
				return errors.New("concurrency must be at least 1")
			}

			platform, err := appstore.ParsePlatform(platformValue)
			if err != nil {
				return err
			}

			var lastErr error
			var acc appstore.Account

			return retry.Do(func() error {
				infoResult, err := dependencies.AppStore.AccountInfo()
				if err != nil {
					return err
				}

				acc = infoResult.Account

				if errors.Is(lastErr, appstore.ErrPasswordTokenExpired) {
					loginResult, err := dependencies.AppStore.Login(appstore.LoginInput{
						Email:    acc.Email,
						Password: acc.Password,
					})
					if err != nil {
						return err
					}

					acc = loginResult.Account
				}

				app := appstore.App{ID: appID}
				if bundleID != "" {
					lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{Account: acc, BundleID: bundleID, Platform: platform})
					if err != nil {
						return err
					}

					app = lookupResult.App
				}

				out, err := dependencies.AppStore.ListVersions(appstore.ListVersionsInput{Context: cmd.Context(), Account: acc, App: app, Platform: platform})
				if err != nil {
					return err
				}

				event := dependencies.Logger.Log().
					Interface("externalVersionIdentifiers", out.ExternalVersionIdentifiers).
					Str("bundleID", app.BundleID)

				if withMetadata {
					versions, err := listVersionMetadata(cmd.Context(), acc, app, platform, out.ExternalVersionIdentifiers, concurrency)
					if err != nil {
						return err
					}

					event = event.Interface("versions", versions)
				}

				event.Bool("success", true).Send()

				return nil
			},
				retry.LastErrorOnly(true),
				retry.DelayType(retry.FixedDelay),
				retry.Delay(time.Millisecond),
				retry.Attempts(2),
				retry.RetryIf(func(err error) bool {
					lastErr = err

					return errors.Is(err, appstore.ErrPasswordTokenExpired)
				}),
			)
		},
	}

	cmd.Flags().Int64VarP(&appID, "app-id", "i", 0, "ID of the target app (required)")
	cmd.Flags().StringVarP(&bundleID, "bundle-identifier", "b", "", "The bundle identifier of the target app (overrides the app ID)")

	cmd.Flags().StringVar(&platformValue, "platform", "", "Platform to list versions for: iphone (iOS), ipad (iPadOS), appletv (tvOS), watchos, visionos, or macos")
	cmd.Flags().BoolVar(&withMetadata, "with-metadata", false, "Also fetch the display version and release date of every version (recent versions come from the public catalog in one request; older versions are cached locally after the first fetch)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Number of version metadata requests to run at once (with --with-metadata)")

	return cmd
}

type versionMetadata struct {
	ExternalVersionID string    `json:"externalVersionID"`
	DisplayVersion    string    `json:"displayVersion"`
	ReleaseDate       time.Time `json:"releaseDate"`
	// Source is "catalog" (public App Store catalog, release date as shown on the
	// store page) or "store" (per-version store request, release date from the package).
	Source string `json:"source"`
}

// listVersionMetadata takes the recent versions from the public catalog in one
// request, older versions seen before from the local cache, and fetches only the
// rest one by one from the store.
func listVersionMetadata(
	ctx context.Context,
	acc appstore.Account,
	app appstore.App,
	platform appstore.Platform,
	versionIDs []string,
	concurrency int,
) ([]versionMetadata, error) {
	known := map[string]versionMetadata{}

	cachePath := versionMetadataCachePath()

	cache, err := loadVersionMetadataCache(cachePath)
	if err != nil {
		dependencies.Logger.Verbose().Err(err).Msg("ignoring version metadata cache")
	}

	catalog, err := dependencies.AppStore.CatalogVersionHistory(appstore.CatalogVersionHistoryInput{Account: acc, App: app, Platform: platform})
	if err != nil {
		dependencies.Logger.Verbose().Err(err).Msg("catalog version history unavailable; fetching every version from the store")
	}

	for _, v := range catalog.Versions {
		known[v.ExternalVersionID] = versionMetadata{
			ExternalVersionID: v.ExternalVersionID,
			DisplayVersion:    v.DisplayVersion,
			ReleaseDate:       v.ReleaseDate,
			Source:            "catalog",
		}
	}

	var missing []string

	for _, id := range versionIDs {
		if _, ok := known[id]; ok {
			continue
		}

		if v, ok := cache.get(app.ID, id); ok {
			known[id] = v

			continue
		}

		missing = append(missing, id)
	}

	fetched, fetchErr := fetchVersionMetadata(ctx, acc, app, platform, missing, concurrency)

	for _, v := range fetched {
		if v.DisplayVersion != "" {
			known[v.ExternalVersionID] = v
		}
	}

	for _, v := range known {
		cache.put(app.ID, v)
	}

	if err := cache.save(cachePath); err != nil {
		dependencies.Logger.Verbose().Err(err).Msg("could not save version metadata cache")
	}

	if fetchErr != nil {
		return nil, fetchErr
	}

	results := make([]versionMetadata, len(versionIDs))
	for i, id := range versionIDs {
		results[i] = known[id]
	}

	return results, nil
}

func fetchVersionMetadata(
	ctx context.Context,
	acc appstore.Account,
	app appstore.App,
	platform appstore.Platform,
	versionIDs []string,
	concurrency int,
) ([]versionMetadata, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]versionMetadata, len(versionIDs))
	sem := make(chan struct{}, concurrency)

	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)

	for i, id := range versionIDs {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}

		if ctx.Err() != nil {
			break
		}

		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			out, err := dependencies.AppStore.GetVersionMetadata(appstore.GetVersionMetadataInput{
				Context:   ctx,
				Account:   acc,
				App:       app,
				VersionID: id,
				Platform:  platform,
			})
			if err != nil {
				errOnce.Do(func() {
					firstErr = fmt.Errorf("failed to get metadata for version %s: %w", id, err)
					cancel()
				})

				return
			}

			results[i] = versionMetadata{ExternalVersionID: id, DisplayVersion: out.DisplayVersion, ReleaseDate: out.ReleaseDate, Source: "store"}
		}()
	}

	wg.Wait()

	// Partial results are returned with the error so the caller can cache them.
	if firstErr != nil {
		return results, firstErr
	}

	return results, ctx.Err()
}
