package keystore

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/accounts"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/cespare/cp"
	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
)

var (
	cachetestDir, _   = filepath.Abs(filepath.Join("testdata", "keystore"))
	cachetestAccounts = []accounts.Account{
		{
			Address: types.StringToAddress("7ef5a6135f1fd6a02593eedc869c6d41d934aef8"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(cachetestDir, "UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8")},
		},
		{
			Address: types.StringToAddress("f466859ead1932d743d622cb74fc058882e8648a"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(cachetestDir, "aaa")},
		},
		{
			Address: types.StringToAddress("289d485d9771714cce91d3393d764e1311907acc"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(cachetestDir, "zzz")},
		},
	}
)

// waitWatcherStart waits up to 1s for the keystore watcher to start.
func waitWatcherStart(ks *KeyStore) bool {
	// On systems where file watch is not supported, just return "ok".
	if !ks.cache.watcher.enabled() {
		return true
	}

	// The watcher should start, and then exit.
	for t0 := time.Now().UTC(); time.Since(t0) < 1*time.Second; time.Sleep(100 * time.Millisecond) {
		if ks.cache.watcherStarted() {
			return true
		}
	}

	return false
}

func waitForAccounts(wantAccounts []accounts.Account, ks *KeyStore) error {
	var list []accounts.Account
	for t0 := time.Now().UTC(); time.Since(t0) < 20*time.Second; time.Sleep(100 * time.Millisecond) {
		list = ks.Accounts()
		if reflect.DeepEqual(list, wantAccounts) {
			// ks should have also received change notifications
			select {
			case <-ks.changes:
			default:
				return errors.New("wasn't notified of new accounts")
			}

			return nil
		}
	}

	return fmt.Errorf("\ngot  %v\nwant %v", list, wantAccounts)
}

func TestWatchNewFile(t *testing.T) {
	t.Parallel()

	dir, ks := tmpKeyStore(t)

	// Ensure the watcher is started before adding any files.
	ks.Accounts()

	if !waitWatcherStart(ks) {
		t.Fatal("keystore watcher didn't start in time")
	}
	// Move in the files.
	wantAccounts := make([]accounts.Account, len(cachetestAccounts))
	for i := range cachetestAccounts {
		wantAccounts[i] = accounts.Account{
			Address: cachetestAccounts[i].Address,
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(dir, filepath.Base(cachetestAccounts[i].URL.Path))},
		}

		if err := cp.CopyFile(wantAccounts[i].URL.Path, cachetestAccounts[i].URL.Path); err != nil {
			t.Fatal(err)
		}
	}

	// ks should see the accounts.
	if err := waitForAccounts(wantAccounts, ks); err != nil {
		t.Error(err)
	}
}

func TestWatchNoDir(t *testing.T) {
	t.Parallel()

	// Create ks but not the directory that it watches.
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("eth-keystore-watchnodir-test-%d-%d", os.Getpid(), rand.Int()))
	ks := NewKeyStore(dir, LightScryptN, LightScryptP, hclog.NewNullLogger())
	list := ks.Accounts()

	if len(list) > 0 {
		t.Error("initial account list not empty:", list)
	}
	// The watcher should start, and then exit.
	if !waitWatcherStart(ks) {
		t.Fatal("keystore watcher didn't start in time")
	}
	// Create the directory and copy a key file into it.
	require.NoError(t, os.MkdirAll(dir, 0700))

	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "aaa")

	if err := cp.CopyFile(file, cachetestAccounts[0].URL.Path); err != nil {
		t.Fatal(err)
	}

	// ks should see the account.
	wantAccounts := []accounts.Account{cachetestAccounts[0]}
	wantAccounts[0].URL = accounts.URL{Scheme: KeyStoreScheme, Path: file}

	for d := 200 * time.Millisecond; d < 8*time.Second; d *= 2 {
		list = ks.Accounts()

		if reflect.DeepEqual(list, wantAccounts) {
			// ks should have also received change notifications
			select {
			case <-ks.changes:
			default:
				t.Fatalf("wasn't notified of new accounts")
			}

			return
		}

		time.Sleep(d)
	}

	t.Errorf("\ngot  %v\nwant %v", list, wantAccounts)
}

func TestCacheInitialReload(t *testing.T) {
	t.Parallel()

	cache, _ := newAccountCache(cachetestDir, hclog.NewNullLogger())
	accounts := cache.accounts()

	if !reflect.DeepEqual(accounts, cachetestAccounts) {
		t.Fatalf("got initial accounts: %swant %s", spew.Sdump(accounts), spew.Sdump(cachetestAccounts))
	}
}

func TestCacheAddDeleteOrder(t *testing.T) {
	t.Parallel()

	cache, _ := newAccountCache("testdata/no-such-dir", hclog.NewNullLogger())
	cache.watcher.running = true // prevent unexpected reloads

	accs := []accounts.Account{
		{
			Address: types.StringToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "-309830980"},
		},
		{
			Address: types.StringToAddress("2cac1adea150210703ba75ed097ddfe24e14f213"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "ggg"},
		},
		{
			Address: types.StringToAddress("8bda78331c916a08481428e4b07c96d3e916d165"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "zzzzzz-the-very-last-one.keyXXX"},
		},
		{
			Address: types.StringToAddress("d49ff4eeb0b2686ed89c0fc0f2b6ea533ddbbd5e"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "SOMETHING.key"},
		},
		{
			Address: types.StringToAddress("7ef5a6135f1fd6a02593eedc869c6d41d934aef8"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "UTC--2016-03-22T12-57-55.920751759Z--7ef5a6135f1fd6a02593eedc869c6d41d934aef8"},
		},
		{
			Address: types.StringToAddress("f466859ead1932d743d622cb74fc058882e8648a"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "aaa"},
		},
		{
			Address: types.StringToAddress("289d485d9771714cce91d3393d764e1311907acc"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: "zzz"},
		},
	}

	for _, a := range accs {
		cache.add(a)
	}
	// Add some of them twice to check that they don't get reinserted.
	cache.add(accs[0])
	cache.add(accs[2])

	// Check that the account list is sorted by filename.
	wantAccounts := make([]accounts.Account, len(accs))

	copy(wantAccounts, accs)

	slices.SortFunc(wantAccounts, byURL)

	list := cache.accounts()

	if !reflect.DeepEqual(list, wantAccounts) {
		t.Fatalf("got accounts: %s\nwant %s", spew.Sdump(accs), spew.Sdump(wantAccounts))
	}

	for _, a := range accs {
		if !cache.hasAddress(a.Address) {
			t.Errorf("expected hasAccount(%x) to return true", a.Address)
		}
	}

	if cache.hasAddress(types.StringToAddress("fd9bd350f08ee3c0c19b85a8e16114a11a60aa4e")) {
		t.Errorf("expected hasAccount(%x) to return false", types.StringToAddress("fd9bd350f08ee3c0c19b85a8e16114a11a60aa4e"))
	}

	// Delete a few keys from the cache.
	for i := 0; i < len(accs); i += 2 {
		cache.delete(wantAccounts[i])
	}

	cache.delete(accounts.Account{Address: types.StringToAddress("fd9bd350f08ee3c0c19b85a8e16114a11a60aa4e"), URL: accounts.URL{Scheme: KeyStoreScheme, Path: "something"}})

	// Check content again after deletion.
	wantAccountsAfterDelete := []accounts.Account{
		wantAccounts[1],
		wantAccounts[3],
		wantAccounts[5],
	}

	list = cache.accounts()
	if !reflect.DeepEqual(list, wantAccountsAfterDelete) {
		t.Fatalf("got accounts after delete: %s\nwant %s", spew.Sdump(list), spew.Sdump(wantAccountsAfterDelete))
	}

	for _, a := range wantAccountsAfterDelete {
		if !cache.hasAddress(a.Address) {
			t.Errorf("expected hasAccount(%x) to return true", a.Address)
		}
	}

	if cache.hasAddress(wantAccounts[0].Address) {
		t.Errorf("expected hasAccount(%x) to return false", wantAccounts[0].Address)
	}
}

func TestCacheFind(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "dir")

	cache, _ := newAccountCache(dir, hclog.NewNullLogger())

	cache.watcher.running = true // prevent unexpected reloads

	accs := []accounts.Account{
		{
			Address: types.StringToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(dir, "a.key")},
		},
		{
			Address: types.StringToAddress("2cac1adea150210703ba75ed097ddfe24e14f213"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(dir, "b.key")},
		},
		{
			Address: types.StringToAddress("d49ff4eeb0b2686ed89c0fc0f2b6ea533ddbbd5e"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(dir, "c.key")},
		},
		{
			Address: types.StringToAddress("d49ff4eeb0b2686ed89c0fc0f2b6ea533ddbbd5e"),
			URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(dir, "c2.key")},
		},
	}

	for _, a := range accs {
		cache.add(a)
	}

	nomatchAccount := accounts.Account{
		Address: types.StringToAddress("f466859ead1932d743d622cb74fc058882e8648a"),
		URL:     accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Join(dir, "something")},
	}

	tests := []struct {
		Query      accounts.Account
		WantResult accounts.Account
		WantError  error
	}{
		// by address
		{Query: accounts.Account{Address: accs[0].Address}, WantResult: accs[0]},
		// by file
		{Query: accounts.Account{URL: accs[0].URL}, WantResult: accs[0]},
		// by basename
		{Query: accounts.Account{URL: accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Base(accs[0].URL.Path)}}, WantResult: accs[0]},
		// by file and address
		{Query: accs[0], WantResult: accs[0]},
		// ambiguous address, tie resolved by file
		{Query: accs[2], WantResult: accs[2]},
		// ambiguous address error
		{
			Query: accounts.Account{Address: accs[2].Address},
			WantError: &accounts.AmbiguousAddrError{
				Addr:    accs[2].Address,
				Matches: []accounts.Account{accs[2], accs[3]},
			},
		},
		// no match error
		{Query: nomatchAccount, WantError: accounts.ErrNoMatch},
		{Query: accounts.Account{URL: nomatchAccount.URL}, WantError: accounts.ErrNoMatch},
		{Query: accounts.Account{URL: accounts.URL{Scheme: KeyStoreScheme, Path: filepath.Base(nomatchAccount.URL.Path)}}, WantError: accounts.ErrNoMatch},
		{Query: accounts.Account{Address: nomatchAccount.Address}, WantError: accounts.ErrNoMatch},
	}

	for i, test := range tests {
		a, err := cache.find(test.Query)
		if !reflect.DeepEqual(err, test.WantError) {
			t.Errorf("test %d: error mismatch for query %v\ngot %q\nwant %q", i, test.Query, err, test.WantError)

			continue
		}

		if a != test.WantResult {
			t.Errorf("test %d: result mismatch for query %v\ngot %v\nwant %v", i, test.Query, a, test.WantResult)

			continue
		}
	}
}

// TestUpdatedKeyfileContents tests that updating the contents of a keystore file
// is noticed by the watcher, and the account cache is updated accordingly
func TestUpdatedKeyfileContents(t *testing.T) {
	t.Skip()
	// t.Parallel()

	// Create a temporary keystore to test with
	dir, err := filepath.Abs(filepath.Join(fmt.Sprintf("eth-keystore-updatedkeyfilecontents-test-%d-%d", os.Getpid(), rand.Int()))) //nolint:gocritic
	require.NoError(t, err)

	ks := NewKeyStore(dir, LightScryptN, LightScryptP, hclog.NewNullLogger())

	list := ks.Accounts()
	if len(list) > 0 {
		t.Error("initial account list not empty:", list)
	}

	if !waitWatcherStart(ks) {
		t.Fatal("keystore watcher didn't start in time")
	}
	// Create the directory and copy a key file into it.
	require.NoError(t, os.MkdirAll(dir, 0700))
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "aaa")

	// Place one of our testfiles in there
	if err := cp.CopyFile(file, cachetestAccounts[0].URL.Path); err != nil {
		t.Fatal(err)
	}

	// ks should see the account.
	wantAccounts := []accounts.Account{cachetestAccounts[0]}
	wantAccounts[0].URL = accounts.URL{Scheme: KeyStoreScheme, Path: file}

	if err := waitForAccounts(wantAccounts, ks); err != nil {
		t.Error(err)

		return
	}
	// needed so that modTime of `file` is different to its current value after forceCopyFile
	require.NoError(t, os.Chtimes(file, time.Now().UTC().Add(-time.Second), time.Now().UTC().Add(-time.Second)))

	// Now replace file contents
	if err := cp.CopyFileOverwrite(file, cachetestAccounts[1].URL.Path); err != nil {
		t.Fatal(err)

		return
	}

	wantAccounts = []accounts.Account{cachetestAccounts[1]}
	wantAccounts[0].URL = accounts.URL{Scheme: KeyStoreScheme, Path: file}

	if err := waitForAccounts(wantAccounts, ks); err != nil {
		t.Errorf("First replacement failed")
		t.Error(err)

		return
	}

	// needed so that modTime of `file` is different to its current value after forceCopyFile
	require.NoError(t, os.Chtimes(file, time.Now().UTC().Add(-time.Second), time.Now().UTC().Add(-time.Second)))

	// Now replace file contents again
	if err := cp.CopyFileOverwrite(file, cachetestAccounts[2].URL.Path); err != nil {
		t.Fatal(err)

		return
	}

	wantAccounts = []accounts.Account{cachetestAccounts[2]}
	wantAccounts[0].URL = accounts.URL{Scheme: KeyStoreScheme, Path: file}

	if err := waitForAccounts(wantAccounts, ks); err != nil {
		t.Errorf("Second replacement failed")
		t.Error(err)

		return
	}

	// needed so that modTime of `file` is different to its current value after os.WriteFile
	require.NoError(t, os.Chtimes(file, time.Now().UTC().Add(-time.Second), time.Now().UTC().Add(-time.Second)))

	// Now replace file contents with crap
	if err := os.WriteFile(file, []byte("foo"), 0600); err != nil {
		t.Fatal(err)

		return
	}

	if err := waitForAccounts([]accounts.Account{}, ks); err != nil {
		t.Errorf("Emptying account file failed")
		t.Error(err)

		return
	}
}

// forceCopyFile is like cp.CopyFile, but doesn't complain if the destination exists.
func forceCopyFile(dst, src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0644)
}