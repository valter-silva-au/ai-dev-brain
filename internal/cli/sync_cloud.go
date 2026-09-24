package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration/cloudsync"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// defaultCloudRegion is the personal-account region for the archive
// (Perth-adjacent). Overridable with --region or ADB_CLOUD_REGION.
const defaultCloudRegion = "ap-southeast-2"

// cloudEventTypeSyncPushed and friends are string EventTypes emitted to
// the workspace event log. Kept local to this file so this WS's PR
// doesn't collide with WS-F's observability-const additions.
const (
	cloudEventSyncPushed  observability.EventType = "cloud.sync_pushed"
	cloudEventSyncPulled  observability.EventType = "cloud.sync_pulled"
	cloudEventSyncStatus  observability.EventType = "cloud.sync_status"
	cloudEventSyncDestroy observability.EventType = "cloud.sync_destroyed"
)

// newSyncCloudCmd builds `adb sync cloud {push|pull|status|destroy}` —
// the S3 archive plane (WS-G). Bucket + region come from flags/env
// (ADB_CLOUD_BUCKET / ADB_CLOUD_REGION, default ap-southeast-2). Auth
// is the local AWS profile chain; no credentials are stored.
func newSyncCloudCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: "S3-backed cloud archive of the workspace (KB)",
		Long: `Ship the allowlisted KB content (raw/, scripts/, skills/, tickets/
minus communications/, wiki/, plus root config) to a versioned SSE-KMS
S3 bucket and pull it back. Never uploads .env/.omnictx/communications
or any adb-machinery. Fail-closed on gitleaks findings.

Bucket + region come from --bucket / --region flags or from
ADB_CLOUD_BUCKET / ADB_CLOUD_REGION env vars.`,
	}
	cmd.AddCommand(
		newSyncCloudPushCmd(),
		newSyncCloudPullCmd(),
		newSyncCloudStatusCmd(),
		newSyncCloudDestroyCmd(),
	)
	return cmd
}

// resolveBucketRegion reads --bucket/--region overrides, falling back to
// ADB_CLOUD_BUCKET/ADB_CLOUD_REGION env vars, then the region default.
// Bucket is required for anything but --dry-run push.
func resolveBucketRegion(bucketFlag, regionFlag string) (bucket, region string) {
	bucket = bucketFlag
	if bucket == "" {
		bucket = os.Getenv("ADB_CLOUD_BUCKET")
	}
	region = regionFlag
	if region == "" {
		region = os.Getenv("ADB_CLOUD_REGION")
	}
	if region == "" {
		region = defaultCloudRegion
	}
	return
}

// buildS3Store constructs a live S3-backed ObjectStore. Guarded so
// unit-testable subcommands (push --dry-run) never hit AWS.
func buildS3Store(cmd *cobra.Command, bucket, region string) (cloudsync.ObjectStore, error) {
	if bucket == "" {
		return nil, fmt.Errorf("--bucket (or ADB_CLOUD_BUCKET) is required")
	}
	return cloudsync.NewS3Store(cmd.Context(), bucket, region)
}

// logCloudEvent emits an event via App.EventLog when available; never
// fatal. Kept as a small helper so every subcommand can call it in one
// line at the tail.
func logCloudEvent(evt observability.EventType, data map[string]interface{}) {
	if App == nil || App.EventLog == nil {
		return
	}
	App.EventLog.Log(evt, data)
}

func newSyncCloudPushCmd() *cobra.Command {
	var (
		bucket string
		region string
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload the allowlisted KB set (fails closed on secret finding)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			b, r := resolveBucketRegion(bucket, region)
			cfg := cloudsync.Config{
				BasePath: App.BasePath,
				Bucket:   b,
				Region:   r,
				DryRun:   dryRun,
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(),
					"sync cloud push --dry-run: reporting the plan (no scanner, no upload)\n")
			} else {
				store, err := buildS3Store(cmd, b, r)
				if err != nil {
					return err
				}
				cfg.Store = store
				cfg.Leak = defaultCloudLeakRunner
			}
			if err := cloudsync.Push(cmd.Context(), cfg); err != nil {
				return err
			}
			logCloudEvent(cloudEventSyncPushed, map[string]interface{}{
				"bucket": b, "region": r, "dry_run": dryRun,
			})
			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "sync cloud push --dry-run: OK (would push)")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "sync cloud push: uploaded to s3://%s (%s)\n", b, r)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name (or ADB_CLOUD_BUCKET)")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (or ADB_CLOUD_REGION; default ap-southeast-2)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the plan; no scanner, no upload")
	return cmd
}

func newSyncCloudPullCmd() *cobra.Command {
	var (
		bucket string
		region string
		dest   string
	)
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download the archive into a fresh directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			b, r := resolveBucketRegion(bucket, region)
			if dest == "" {
				return fmt.Errorf("--dest is required (use a fresh directory)")
			}
			store, err := buildS3Store(cmd, b, r)
			if err != nil {
				return err
			}
			cfg := cloudsync.Config{Bucket: b, Region: r, Store: store}
			if err := cloudsync.Pull(cmd.Context(), cfg, dest); err != nil {
				return err
			}
			logCloudEvent(cloudEventSyncPulled, map[string]interface{}{
				"bucket": b, "region": r, "dest": dest,
			})
			fmt.Fprintf(cmd.OutOrStdout(), "sync cloud pull: restored to %s from s3://%s\n", dest, b)
			return nil
		},
	}
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name (or ADB_CLOUD_BUCKET)")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (or ADB_CLOUD_REGION; default ap-southeast-2)")
	cmd.Flags().StringVar(&dest, "dest", "", "Destination directory (must be fresh)")
	return cmd
}

func newSyncCloudStatusCmd() *cobra.Command {
	var (
		bucket string
		region string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show remote object count vs local upload-set count",
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			b, r := resolveBucketRegion(bucket, region)
			store, err := buildS3Store(cmd, b, r)
			if err != nil {
				return err
			}
			cfg := cloudsync.Config{BasePath: App.BasePath, Bucket: b, Region: r, Store: store}
			rep, err := cloudsync.Status(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			logCloudEvent(cloudEventSyncStatus, map[string]interface{}{
				"bucket": b, "remote": rep.RemoteObjects, "local": rep.LocalUploadSet,
			})
			fmt.Fprintf(cmd.OutOrStdout(),
				"sync cloud status: remote=%d objects, local=%d files (would upload)\n",
				rep.RemoteObjects, rep.LocalUploadSet)
			return nil
		},
	}
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name (or ADB_CLOUD_BUCKET)")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (or ADB_CLOUD_REGION; default ap-southeast-2)")
	return cmd
}

func newSyncCloudDestroyCmd() *cobra.Command {
	var (
		bucket  string
		region  string
		confirm bool
	)
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Empty the archive bucket (BUCKET stays; run cdk destroy after)",
		Long: `Deletes every object in the archive bucket. The BUCKET itself is
torn down by 'cdk destroy' — this command exists so a versioned
bucket can be emptied first (a prerequisite for a clean stack teardown).
Requires --confirm.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			if !confirm {
				return fmt.Errorf("refuse to destroy without --confirm")
			}
			b, r := resolveBucketRegion(bucket, region)
			store, err := buildS3Store(cmd, b, r)
			if err != nil {
				return err
			}
			cfg := cloudsync.Config{Bucket: b, Region: r, Store: store}
			if err := cloudsync.Destroy(cmd.Context(), cfg, true); err != nil {
				return err
			}
			logCloudEvent(cloudEventSyncDestroy, map[string]interface{}{
				"bucket": b, "region": r,
			})
			fmt.Fprintf(cmd.OutOrStdout(), "sync cloud destroy: emptied s3://%s (%s)\n", b, r)
			return nil
		},
	}
	cmd.Flags().StringVar(&bucket, "bucket", "", "S3 bucket name (or ADB_CLOUD_BUCKET)")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (or ADB_CLOUD_REGION; default ap-southeast-2)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm destructive action (required)")
	return cmd
}

// defaultCloudLeakRunner is the LeakRunner Push uses in real deploys.
// Points at cloudsync.DefaultLeakRunner (which shells out to `gitleaks`).
var defaultCloudLeakRunner cloudsync.LeakRunner = cloudsync.DefaultLeakRunner
