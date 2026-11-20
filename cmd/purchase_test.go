package cmd

import (
	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/majd/ipatool/v2/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Purchase command", func() {
	It("accepts a platform flag with macOS help", func() {
		cmd := purchaseCmd()
		platform, err := cmd.Flags().GetString("platform")

		Expect(err).ToNot(HaveOccurred())
		Expect(platform).To(BeEmpty())
		Expect(cmd.Flag("platform").Usage).To(ContainSubstring("macos"))
	})

	DescribeTable("purchasing by app ID", func(flag string) {
		store := &fakePurchaseAppStore{}
		previousDependencies := dependencies
		DeferCleanup(func() { dependencies = previousDependencies })
		dependencies.Logger = log.NewLogger(log.Args{})

		cmd := purchaseCmdWithAppStore(func() appstore.AppStore { return store })
		cmd.SetArgs([]string{flag, "123456789", "--platform", "macos"})

		Expect(cmd.Execute()).To(Succeed())
		Expect(store.lookupCalls).To(BeZero())
		Expect(store.purchaseInput.App).To(Equal(appstore.App{ID: 123456789}))
		Expect(store.purchaseInput.Platform).To(Equal(appstore.PlatformMacOS))
	},
		Entry("accepts the long flag", "--app-id"),
		Entry("accepts the short flag", "-i"),
	)

	It("requires an app ID or bundle identifier before accessing the account", func() {
		cmd := purchaseCmdWithAppStore(func() appstore.AppStore {
			Fail("must not access the app store without a target")

			return nil
		})
		cmd.SetArgs([]string{})

		Expect(cmd.Execute()).To(MatchError("either the app ID or the bundle identifier must be specified"))
	})

	It("lets the bundle identifier override the app ID", func() {
		store := &fakePurchaseAppStore{}
		previousDependencies := dependencies
		DeferCleanup(func() { dependencies = previousDependencies })
		dependencies.Logger = log.NewLogger(log.Args{})

		cmd := purchaseCmdWithAppStore(func() appstore.AppStore { return store })
		cmd.SetArgs([]string{"-i", "123456789", "-b", "com.example.app"})

		Expect(cmd.Execute()).To(Succeed())
		Expect(store.lookupCalls).To(Equal(1))
		Expect(store.purchaseInput.App).To(Equal(appstore.App{ID: 42, BundleID: "com.example.app"}))
	})

	It("reads the account once and reuses the refreshed account on retry", func() {
		store := &fakePurchaseAppStore{
			loginAccount:   appstore.Account{Email: "user@example.com", PasswordToken: "refreshed-token", StoreFront: "143441"},
			purchaseErrors: []error{appstore.ErrPasswordTokenExpired},
		}
		previousDependencies := dependencies
		DeferCleanup(func() { dependencies = previousDependencies })
		dependencies.Logger = log.NewLogger(log.Args{})

		cmd := purchaseCmdWithAppStore(func() appstore.AppStore { return store })
		cmd.SetArgs([]string{"-i", "123456789"})

		Expect(cmd.Execute()).To(Succeed())
		Expect(store.accountInfoCalls).To(Equal(1))
		Expect(store.loginInputs).To(Equal([]appstore.LoginInput{{Email: "user@example.com", Password: "password"}}))
		Expect(store.purchaseInput.Account).To(Equal(store.loginAccount))
	})

	It("propagates macOS to lookup and purchase", func() {
		store := &fakePurchaseAppStore{}
		previousDependencies := dependencies
		DeferCleanup(func() { dependencies = previousDependencies })
		dependencies.Logger = log.NewLogger(log.Args{})

		cmd := purchaseCmdWithAppStore(func() appstore.AppStore { return store })
		cmd.SetArgs([]string{"--bundle-identifier", "com.example.mac", "--platform", "macos"})

		Expect(cmd.Execute()).To(Succeed())
		Expect(store.lookupInput.BundleID).To(Equal("com.example.mac"))
		Expect(store.lookupInput.Platform).To(Equal(appstore.PlatformMacOS))
		Expect(store.purchaseInput.Platform).To(Equal(appstore.PlatformMacOS))
		Expect(store.purchaseInput.App).To(Equal(appstore.App{ID: 42, BundleID: "com.example.mac"}))
	})
})

type fakePurchaseAppStore struct {
	accountInfoCalls int
	loginAccount     appstore.Account
	loginInputs      []appstore.LoginInput
	lookupCalls      int
	lookupInput      appstore.LookupInput
	purchaseErrors   []error
	purchaseInput    appstore.PurchaseInput
}

func (f *fakePurchaseAppStore) Login(input appstore.LoginInput) (appstore.LoginOutput, error) {
	f.loginInputs = append(f.loginInputs, input)

	return appstore.LoginOutput{Account: f.loginAccount}, nil
}

func (f *fakePurchaseAppStore) AccountInfo() (appstore.AccountInfoOutput, error) {
	f.accountInfoCalls++

	return appstore.AccountInfoOutput{Account: appstore.Account{Email: "user@example.com", Password: "password", StoreFront: "143441"}}, nil
}

func (*fakePurchaseAppStore) Revoke() error { return nil }

func (f *fakePurchaseAppStore) Lookup(input appstore.LookupInput) (appstore.LookupOutput, error) {
	f.lookupCalls++
	f.lookupInput = input

	return appstore.LookupOutput{App: appstore.App{ID: 42, BundleID: input.BundleID}}, nil
}

func (*fakePurchaseAppStore) Search(input appstore.SearchInput) (appstore.SearchOutput, error) {
	return appstore.SearchOutput{}, nil
}

func (*fakePurchaseAppStore) OwnedApps(input appstore.OwnedAppsInput) (appstore.OwnedAppsOutput, error) {
	return appstore.OwnedAppsOutput{}, nil
}

func (f *fakePurchaseAppStore) Purchase(input appstore.PurchaseInput) error {
	f.purchaseInput = input

	if len(f.purchaseErrors) == 0 {
		return nil
	}

	err := f.purchaseErrors[0]
	f.purchaseErrors = f.purchaseErrors[1:]

	return err
}

func (*fakePurchaseAppStore) Download(input appstore.DownloadInput) (appstore.DownloadOutput, error) {
	return appstore.DownloadOutput{}, nil
}

func (*fakePurchaseAppStore) ReplicateSinf(input appstore.ReplicateSinfInput) error { return nil }

func (*fakePurchaseAppStore) ListVersions(input appstore.ListVersionsInput) (appstore.ListVersionsOutput, error) {
	return appstore.ListVersionsOutput{}, nil
}

func (*fakePurchaseAppStore) GetVersionMetadata(input appstore.GetVersionMetadataInput) (appstore.GetVersionMetadataOutput, error) {
	return appstore.GetVersionMetadataOutput{}, nil
}

func (*fakePurchaseAppStore) Bag(input appstore.BagInput) (appstore.BagOutput, error) {
	return appstore.BagOutput{}, nil
}
