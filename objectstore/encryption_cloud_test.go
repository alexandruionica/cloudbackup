package objectstore

import (
	"cloudbackup/config"
	"cloudbackup/shared"
	"cloudbackup/testutils"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// The tests in this file exercise the keystore sidecar bootstrap path against real
// cloud buckets. They focus on the *conditional-PUT* race semantics that the existing
// objectstore/encryption_test.go covers only against the in-memory TestNull store —
// real S3/GCS/Azure each have provider-specific preconditions (If-None-Match: */
// IfGenerationMatch=0 / blob exists) and only an end-to-end test against the actual
// service confirms the SDK call shape is right.
//
// All tests in this file skip when their per-provider env vars are unset, just like
// the existing TestAwsS3ValidateUploadDelete etc. tests in the sibling files.
//
// Each test uses a per-run prefix so that concurrent CI lanes don't clash on the
// sidecar object. The cloud-bucket cleanup of the leftover sidecar object is best-effort
// — see clean_object_stores_after_tests.py for the global cleanup step.

const cloudEncryptionTestPass = "cloud-encryption-test-pass-do-not-reuse-this"

// initStoreFromBackupConfig is a small factory: given a backupConfig (already
// populated by getAndSet*ConfigFromEnv) it brings up an ObjectStore for the
// chosen target via GetObjectStores. Used by both the AWS, GCS and Azure cloud
// encryption tests so we don't duplicate plumbing.
func initStoreFromBackupConfig(t *testing.T, backupConfig shared.ConfigBackup, targetName string) ObjectStore {
	t.Helper()
	state := shared.NewJobsState()
	stores, err := GetObjectStores(context.Background(), backupConfig, state)
	if err != nil {
		t.Fatalf("GetObjectStores: %v", err)
	}
	for _, s := range stores {
		name, _ := s.GetStoreDetails()
		if name == targetName {
			return s
		}
	}
	t.Fatalf("could not find target %q in initialised stores", targetName)
	return nil
}

// parallelBootstrap creates two stores from the same backupConfig and calls
// InitEncryption(AllowBootstrap=true) on both concurrently. Returns a slice of
// errors observed (one per goroutine) plus the KEK keystore UUIDs they each
// resolved. The caller asserts the conflict-resolution invariants.
func parallelBootstrap(t *testing.T, backupConfig shared.ConfigBackup, targetName string) (errs []error, uuids []string) {
	t.Helper()
	const N = 2
	errs = make([]error, N)
	uuids = make([]string, N)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store := initStoreFromBackupConfig(t, backupConfig, targetName)
			<-start
			err := store.InitEncryption(EncryptionInitOptions{AllowBootstrap: true})
			errs[idx] = err
			if err == nil {
				if u, ok := store.(interface{ KeystoreUUID() [16]byte }); ok {
					b := u.KeystoreUUID()
					uuids[idx] = fmt.Sprintf("%x", b[:])
				}
			}
		}(i)
	}
	// Release both goroutines as close together as possible.
	close(start)
	wg.Wait()
	return errs, uuids
}

func TestAwsS3EncryptionParallelBootstrap(t *testing.T) {
	// Skips early via getAndSetAwsS3ConfigFromEnv if CLD_S3_BUCKET isn't set.
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cloud_enc_aws_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)
	rt, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	rt.Config.Backup[0].Target[0].RateLimit = "0"
	rt.Config.Backup[0].Encrypt = true
	rt.Config.Backup[0].EncryptPass = cloudEncryptionTestPass + "-aws-" + fmt.Sprint(time.Now().UnixNano())
	const targetName = "aws_enc_1"
	getAndSetAwsS3ConfigFromEnv(rt, t, targetName)
	backupConfig := rt.Config.Backup[0]

	errs, uuids := parallelBootstrap(t, backupConfig, targetName)
	for i, e := range errs {
		if e != nil {
			t.Fatalf("InitEncryption goroutine %d returned error: %v", i, e)
		}
	}
	if uuids[0] == "" || uuids[1] == "" {
		t.Fatalf("expected non-empty KeystoreUUID for both goroutines: %v", uuids)
	}
	if uuids[0] != uuids[1] {
		t.Fatalf("KeystoreUUID mismatch — race resolution failed: %q vs %q", uuids[0], uuids[1])
	}
}

func TestGCPEncryptionParallelBootstrap(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cloud_enc_gcp_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)
	rt, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	rt.Config.Backup[0].Target[0].RateLimit = "0"
	rt.Config.Backup[0].Encrypt = true
	rt.Config.Backup[0].EncryptPass = cloudEncryptionTestPass + "-gcp-" + fmt.Sprint(time.Now().UnixNano())
	const targetName = "gcp_enc_1"
	getAndSetGcpStorageConfigFromEnv(rt, t, targetName)
	backupConfig := rt.Config.Backup[0]

	errs, uuids := parallelBootstrap(t, backupConfig, targetName)
	for i, e := range errs {
		if e != nil {
			t.Fatalf("InitEncryption goroutine %d returned error: %v", i, e)
		}
	}
	if uuids[0] != uuids[1] {
		t.Fatalf("KeystoreUUID mismatch — race resolution failed: %q vs %q", uuids[0], uuids[1])
	}
}

func TestAzureBlobEncryptionParallelBootstrap(t *testing.T) {
	cfgpath, pathsToDelete := testutils.SetupMockConfigAndTmpPaths(t, "unittest_cloud_enc_azure_")
	defer testutils.DeleteTestFilesAndDirs(pathsToDelete)
	rt, err := config.Load(cfgpath, false, &sync.RWMutex{})
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	rt.Config.Backup[0].Target[0].RateLimit = "0"
	rt.Config.Backup[0].Encrypt = true
	rt.Config.Backup[0].EncryptPass = cloudEncryptionTestPass + "-azure-" + fmt.Sprint(time.Now().UnixNano())
	const targetName = "azure_enc_1"
	getAndSetAzureBlobStorageConfigFromEnv(rt, t, targetName)
	backupConfig := rt.Config.Backup[0]

	errs, uuids := parallelBootstrap(t, backupConfig, targetName)
	for i, e := range errs {
		if e != nil {
			t.Fatalf("InitEncryption goroutine %d returned error: %v", i, e)
		}
	}
	if uuids[0] != uuids[1] {
		t.Fatalf("KeystoreUUID mismatch — race resolution failed: %q vs %q", uuids[0], uuids[1])
	}
}
