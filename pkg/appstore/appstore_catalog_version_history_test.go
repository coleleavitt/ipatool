package appstore

import (
	"time"

	"github.com/majd/ipatool/v2/pkg/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const versionHistoryCatalogResponse = `{"data":[{"id":"42","type":"apps","attributes":{"platformAttributes":{"ios":{"versionHistory":[` +
	`{"externalVersionId":1002,"versionDisplay":"2.0.0","releaseTimestamp":"2026-11-18T20:52:13Z"},` +
	`{"externalVersionId":"1001","versionDisplay":"1.0.0","releaseTimestamp":"2026-11-01T08:00:00Z"}]}}}}]}`

var _ = Describe("Catalog version history", func() {
	var client *http.MockClient[platformVersionLookupResult]
	var store *appstore
	account := Account{StoreFront: "143443-2,34"}

	BeforeEach(func() {
		client = http.NewMockClient[platformVersionLookupResult](gomock.NewController(GinkgoT()))
		store = &appstore{platformClient: client, storefrontCountryCodes: testStorefrontCountryCodes()}
	})

	It("maps external version identifiers to display versions in one request", func() {
		client.EXPECT().Send(http.Request{
			URL:            "https://apps.apple.com/api/apps/v1/catalog/de/apps/42?platform=iphone&extend=versionHistory",
			Method:         http.MethodGET,
			ResponseFormat: http.ResponseFormatJSON,
		}).Return(http.Result[platformVersionLookupResult]{StatusCode: 200, Data: decodeCatalogResponse(versionHistoryCatalogResponse)}, nil)

		out, err := store.CatalogVersionHistory(CatalogVersionHistoryInput{Account: account, App: App{ID: 42}})

		Expect(err).ToNot(HaveOccurred())
		Expect(out.Versions).To(Equal([]CatalogVersion{
			{ExternalVersionID: "1002", DisplayVersion: "2.0.0", ReleaseDate: time.Date(2026, 11, 18, 20, 52, 13, 0, time.UTC)},
			{ExternalVersionID: "1001", DisplayVersion: "1.0.0", ReleaseDate: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)},
		}))
	})

	It("rejects a response for a different app", func() {
		client.EXPECT().Send(gomock.Any()).Return(http.Result[platformVersionLookupResult]{
			StatusCode: 200,
			Data:       decodeCatalogResponse(`{"data":[{"id":"7","type":"apps"}]}`),
		}, nil)

		_, err := store.CatalogVersionHistory(CatalogVersionHistoryInput{Account: account, App: App{ID: 42}})
		Expect(err).To(MatchError(ContainSubstring("different app")))
	})

	It("returns no versions for platforms without a catalog history", func() {
		out, err := store.CatalogVersionHistory(CatalogVersionHistoryInput{Account: account, App: App{ID: 42}, Platform: PlatformMacOS})

		Expect(err).ToNot(HaveOccurred())
		Expect(out.Versions).To(BeEmpty())
	})
})
