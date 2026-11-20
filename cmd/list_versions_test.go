package cmd

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/majd/ipatool/v2/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("List Versions command", func() {
	DescribeTable("passes the selected platform to lookup and version listing", func(args []string, expected appstore.Platform, lookup bool) {
		store := &fakeListVersionsAppStore{}
		previous := dependencies
		DeferCleanup(func() { dependencies = previous })
		dependencies.AppStore = store
		dependencies.Logger = log.NewLogger(log.Args{})
		cmd := ListVersionsCmd()
		cmd.SetArgs(args)
		Expect(cmd.Execute()).To(Succeed())
		Expect(store.input.Platform).To(Equal(expected))
		Expect(store.input.App.ID).To(Equal(int64(42)))
		Expect(store.lookupCalled).To(Equal(lookup))
		if lookup {
			Expect(store.lookupInput.Platform).To(Equal(expected))
		}
	},
		Entry("Mac bundle ID", []string{"-b", "com.example.mac", "--platform", "macos"}, appstore.PlatformMacOS, true),
		Entry("Mac app ID alias", []string{"-i", "42", "--platform", "mac"}, appstore.PlatformMacOS, false),
		Entry("default platform", []string{"-i", "42"}, appstore.Platform(""), false),
		Entry("iOS alias", []string{"-i", "42", "--platform", "ios"}, appstore.PlatformIPhone, false),
	)

	It("rejects unknown as a platform before accessing the account", func() {
		cmd := ListVersionsCmd()
		cmd.SetArgs([]string{"-i", "42", "--platform", "unknown"})
		Expect(cmd.Execute()).To(MatchError(`invalid platform "unknown"`))
	})

	Describe("--with-metadata", func() {
		var store *fakeListVersionsMetadataAppStore

		BeforeEach(func() {
			store = &fakeListVersionsMetadataAppStore{ids: []string{"1", "2", "3", "4", "5"}}
			previous := dependencies
			DeferCleanup(func() { dependencies = previous })
			dependencies.AppStore = store
			dependencies.Logger = log.NewLogger(log.Args{})
		})

		It("fetches every version's metadata in order, within the concurrency limit", func() {
			versions, err := fetchVersionMetadata(context.Background(), appstore.Account{}, appstore.App{ID: 42}, "", store.ids, 2)

			Expect(err).NotTo(HaveOccurred())
			Expect(versions).To(HaveLen(5))
			for i, v := range versions {
				Expect(v.ExternalVersionID).To(Equal(store.ids[i]))
				Expect(v.DisplayVersion).To(Equal("v" + store.ids[i]))
			}
			Expect(store.maxInFlight).To(Equal(2))
		})

		It("returns the first failure", func() {
			store.failID = "3"

			cmd := ListVersionsCmd()
			cmd.SetArgs([]string{"-i", "42", "--with-metadata", "--concurrency", "1"})

			Expect(cmd.Execute()).To(MatchError(ContainSubstring("failed to get metadata for version 3")))
		})
	})
})

type fakeListVersionsMetadataAppStore struct {
	appstore.AppStore
	ids         []string
	failID      string
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
}

func (*fakeListVersionsMetadataAppStore) AccountInfo() (appstore.AccountInfoOutput, error) {
	return appstore.AccountInfoOutput{}, nil
}

func (s *fakeListVersionsMetadataAppStore) ListVersions(appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
	return appstore.ListVersionsOutput{ExternalVersionIdentifiers: s.ids}, nil
}

func (s *fakeListVersionsMetadataAppStore) GetVersionMetadata(input appstore.GetVersionMetadataInput) (appstore.GetVersionMetadataOutput, error) {
	s.mu.Lock()
	s.inFlight++
	s.maxInFlight = max(s.maxInFlight, s.inFlight)
	s.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()

	if input.VersionID == s.failID {
		return appstore.GetVersionMetadataOutput{}, errors.New("boom")
	}

	return appstore.GetVersionMetadataOutput{DisplayVersion: "v" + input.VersionID}, nil
}

type fakeListVersionsAppStore struct {
	appstore.AppStore
	input        appstore.ListVersionsInput
	lookupInput  appstore.LookupInput
	lookupCalled bool
}

func (*fakeListVersionsAppStore) AccountInfo() (appstore.AccountInfoOutput, error) {
	return appstore.AccountInfoOutput{}, nil
}
func (s *fakeListVersionsAppStore) Lookup(input appstore.LookupInput) (appstore.LookupOutput, error) {
	s.lookupCalled = true
	s.lookupInput = input

	return appstore.LookupOutput{App: appstore.App{ID: 42, BundleID: input.BundleID}}, nil
}
func (s *fakeListVersionsAppStore) ListVersions(input appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
	s.input = input

	return appstore.ListVersionsOutput{ExternalVersionIdentifiers: []string{"123"}}, nil
}
