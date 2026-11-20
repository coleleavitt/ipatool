package appstore

import (
	"fmt"
	gohttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/majd/ipatool/v2/pkg/http"
)

type CatalogVersionHistoryInput struct {
	Account  Account
	App      App
	Platform Platform
}

type CatalogVersion struct {
	ExternalVersionID string
	DisplayVersion    string
	ReleaseDate       time.Time
}

type CatalogVersionHistoryOutput struct {
	Versions []CatalogVersion
}

type catalogVersionHistoryItem struct {
	ExternalVersionID platformVersionExternalID `json:"externalVersionId,omitempty"`
	VersionDisplay    string                    `json:"versionDisplay,omitempty"`
	ReleaseTimestamp  string                    `json:"releaseTimestamp,omitempty"`
}

// CatalogVersionHistory reads the public App Store catalog, which lists only the
// most recent versions (currently 25). Only iPhone and iPad apps have a history
// there; other platforms return no versions.
func (t *appstore) CatalogVersionHistory(input CatalogVersionHistoryInput) (CatalogVersionHistoryOutput, error) {
	platform := input.Platform
	if platform == "" {
		platform = PlatformIPhone
	}

	if platform != PlatformIPhone && platform != PlatformIPad {
		return CatalogVersionHistoryOutput{}, nil
	}

	countryCode, err := t.countryCodeFromStoreFront(input.Account.StoreFront)
	if err != nil {
		return CatalogVersionHistoryOutput{}, fmt.Errorf("failed to resolve the country code: %w", err)
	}

	res, err := t.platformClient.Send(http.Request{
		URL: fmt.Sprintf("https://apps.apple.com/api/apps/v1/catalog/%s/apps/%d?platform=%s&extend=versionHistory",
			strings.ToLower(countryCode), input.App.ID, platform),
		Method:         http.MethodGET,
		ResponseFormat: http.ResponseFormatJSON,
	})
	if err != nil {
		return CatalogVersionHistoryOutput{}, fmt.Errorf("catalog version history request failed: %w", err)
	}

	if res.StatusCode != gohttp.StatusOK {
		return CatalogVersionHistoryOutput{}, fmt.Errorf("catalog version history returned HTTP %d", res.StatusCode)
	}

	if len(res.Data.Data) != 1 || res.Data.Data[0].ID != strconv.FormatInt(input.App.ID, 10) {
		return CatalogVersionHistoryOutput{}, fmt.Errorf("catalog version history returned a different app")
	}

	history := res.Data.Data[0].Attributes.PlatformAttributes["ios"].VersionHistory
	versions := make([]CatalogVersion, 0, len(history))

	for _, item := range history {
		releaseDate, err := time.Parse(time.RFC3339, item.ReleaseTimestamp)
		if err != nil {
			return CatalogVersionHistoryOutput{}, fmt.Errorf("failed to parse release timestamp %q: %w", item.ReleaseTimestamp, err)
		}

		versions = append(versions, CatalogVersion{
			ExternalVersionID: string(item.ExternalVersionID),
			DisplayVersion:    item.VersionDisplay,
			ReleaseDate:       releaseDate,
		})
	}

	return CatalogVersionHistoryOutput{Versions: versions}, nil
}
