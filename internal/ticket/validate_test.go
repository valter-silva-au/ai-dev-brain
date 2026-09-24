package ticket

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

func TestPolicyCheckToleratesMalformedTicketsAndDoesNotMutate(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Valid policy ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	badRoot := filepath.Join(scope.TicketsRoot(), "TASK-99999-malformed")
	if err := os.MkdirAll(badRoot, 0o755); err != nil {
		t.Fatalf("create malformed ticket: %v", err)
	}
	before, err := os.ReadDir(scope.TicketsRoot())
	if err != nil {
		t.Fatalf("read scope before policy check: %v", err)
	}

	result, err := service.PolicyCheck(context.Background(), PolicyCheckRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceWarn,
	})
	if err != nil {
		t.Fatalf("policy check: %v", err)
	}
	if result.Outcome != capability.OutcomeAttention ||
		len(result.Data.Tickets) != 2 {
		t.Fatalf("policy check result = %#v", result)
	}
	valid := policyTicketByPath(t, result.Data.Tickets, created.Ticket.Path)
	if !valid.Report.Conformant {
		t.Fatalf("valid ticket report = %#v", valid.Report)
	}
	malformed := policyTicketByPath(t, result.Data.Tickets, badRoot)
	assertProfileFindingClass(
		t,
		malformed.Report,
		profile.ConformanceInvalidMetadata,
	)

	after, err := os.ReadDir(scope.TicketsRoot())
	if err != nil {
		t.Fatalf("read scope after policy check: %v", err)
	}
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("policy check mutated scope: before=%v after=%v", before, after)
	}
}

func TestPolicyReconcilePreviewApplyIsSafeJournaledAndIdempotent(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	oldCatalog := reconciliationTemplates(t, "old generated summary\n")
	createService := testLifecycleService(t, Options{
		TemplateResolver: oldCatalog,
	})
	created := applyCreate(t, createService, CreateRequest{
		Scope:     scope,
		Title:     "Reconcile safely",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	layout := created.Ticket.Layout
	contextPath := filepath.Join(layout.Root(), "context.md")
	generatedPath := filepath.Join(layout.Root(), "summary.md")
	artifactsPath := filepath.Join(layout.Root(), "artifacts")
	customContext := []byte("authored customization must survive\n")
	if err := os.WriteFile(contextPath, customContext, 0o644); err != nil {
		t.Fatalf("customize authored context: %v", err)
	}
	if err := os.Remove(artifactsPath); err != nil {
		t.Fatalf("remove structural directory: %v", err)
	}
	checkpointsBefore := append(
		[]AppendOnlyCheckpoint(nil),
		readTicketManifest(t, layout).AppendOnlyCheckpoints...,
	)

	newCatalog := reconciliationTemplates(t, "new generated summary\n")
	service := testLifecycleService(t, Options{
		TemplateResolver: newCatalog,
	})
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		ActorID:       "task-26",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000026",
	}

	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview reconcile: %v", err)
	}
	if preview.Outcome != capability.OutcomePlanned ||
		preview.Data.OperationID != request.OperationID {
		t.Fatalf("preview reconcile = %#v", preview)
	}
	assertCapabilityEffect(t, preview.Effects, "create-directory", artifactsPath)
	assertCapabilityEffect(t, preview.Effects, "replace-generated", generatedPath)
	if string(readFile(t, generatedPath)) != "old generated summary\n" {
		t.Fatal("preview replaced generated content")
	}
	if _, err := os.Stat(artifactsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview created structural directory: %v", err)
	}

	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	applied, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("apply reconcile: %v", err)
	}
	if applied.Outcome != capability.OutcomeApplied {
		t.Fatalf("apply reconcile = %#v", applied)
	}
	if string(readFile(t, generatedPath)) != "new generated summary\n" {
		t.Fatal("generated target was not safely replaced")
	}
	if info, err := os.Stat(artifactsPath); err != nil || !info.IsDir() {
		t.Fatalf("structural directory was not repaired: %v", err)
	}
	if got := readFile(t, contextPath); string(got) != string(customContext) {
		t.Fatalf("authored content was clobbered: %q", got)
	}
	manifest := readTicketManifest(t, layout)
	if fmt.Sprint(manifest.AppendOnlyCheckpoints) !=
		fmt.Sprint(checkpointsBefore) {
		t.Fatalf(
			"reconcile advanced append-only checkpoints: before=%#v after=%#v",
			checkpointsBefore,
			manifest.AppendOnlyCheckpoints,
		)
	}
	renderedFile, err := os.Open(layout.RenderManifestPath())
	if err != nil {
		t.Fatalf("open render manifest: %v", err)
	}
	rendered, decodeErr := profile.DecodeRenderManifest(renderedFile)
	closeErr := renderedFile.Close()
	if decodeErr != nil || closeErr != nil {
		t.Fatalf(
			"decode render manifest: decode=%v close=%v",
			decodeErr,
			closeErr,
		)
	}
	if record := renderRecordByRole(t, rendered, "ticket.summary"); record.GeneratedHash !=
		profile.HashContent([]byte("new generated summary\n")) ||
		record.ObservedHash != record.GeneratedHash {
		t.Fatalf("updated render record = %#v", record)
	}
	assertJournalCommitted(t, scope, request.OperationID)

	if err := os.MkdirAll(
		filepath.Join(scope.TicketsRoot(), "TASK-99998-later-damage"),
		0o755,
	); err != nil {
		t.Fatalf("create unrelated later damage: %v", err)
	}
	repeated, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("repeat reconcile: %v", err)
	}
	if repeated.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("repeat reconcile = %#v", repeated)
	}
}

func TestPolicyReconcileRecoversAppliedUnrecordedSafeStep(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Recover reconciliation",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	var interrupted atomic.Bool
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
		AfterFilesystemStage: func(stage string) error {
			if stage == "replace-generated" &&
				interrupted.CompareAndSwap(false, true) {
				return errors.New("injected reconcile interruption")
			}
			return nil
		},
	})
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000027",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	failed, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("interrupted reconcile = %#v, %v", failed, err)
	}
	generatedPath := filepath.Join(created.Ticket.Path, "summary.md")
	if string(readFile(t, generatedPath)) != "new generated summary\n" {
		t.Fatal("injected failure did not occur after the filesystem effect")
	}

	resumed, err := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	}).PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("resume reconcile: %v", err)
	}
	if resumed.Outcome != capability.OutcomeApplied {
		t.Fatalf("resumed reconcile = %#v", resumed)
	}
	assertJournalCommitted(t, scope, request.OperationID)
}

func TestPolicyReconcileRecoveryRejectsReplacementTicketIdentity(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Reject replacement ticket recovery",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	var interrupted atomic.Bool
	service.beforeRootedPublish = func(target string) error {
		if filepath.Base(target) == "summary.md" &&
			interrupted.CompareAndSwap(false, true) {
			return errors.New("interrupt before replacement publication")
		}
		return nil
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000036",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview identity-bound reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	failed, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("interrupted identity-bound reconcile = %#v, %v", failed, err)
	}

	replacement := readTicketManifest(t, created.Ticket.Layout)
	replacement.ID = "550e8400-e29b-41d4-a716-446655440099"
	writeTicketManifest(t, created.Ticket.Layout, replacement)

	resumed, err := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	}).PolicyReconcile(context.Background(), request)
	if err == nil ||
		!strings.Contains(err.Error(), "ticket identity") ||
		resumed.Outcome != capability.OutcomeFailed {
		t.Fatalf("replacement-ticket recovery = %#v, %v", resumed, err)
	}
	generatedPath := filepath.Join(created.Ticket.Path, "summary.md")
	if got := string(readFile(t, generatedPath)); got !=
		"old generated summary\n" {
		t.Fatalf("replacement ticket was mutated: %q", got)
	}
}

func TestPolicyReconcileRecoversTargetDisplacedBeforePublication(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Recover displaced reconciliation target",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	var interrupted atomic.Bool
	service.afterRootedDisplace = func(target string) error {
		if filepath.Base(target) == "summary.md" &&
			interrupted.CompareAndSwap(false, true) {
			return errors.New("interrupt after guarded displacement")
		}
		return nil
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000033",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview displaced reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	failed, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("displaced reconcile = %#v, %v", failed, err)
	}

	generatedPath := filepath.Join(created.Ticket.Path, "summary.md")
	if _, err := os.Stat(generatedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("guarded target was not displaced: %v", err)
	}
	guardPath := filepath.Join(
		created.Ticket.Path,
		reconcilePublicationGuardName(request.OperationID, 1),
	)
	if got := string(readFile(t, guardPath)); got != "old generated summary\n" {
		t.Fatalf("guarded original content = %q", got)
	}

	resumed, err := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	}).PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("resume displaced reconcile: %v", err)
	}
	if resumed.Outcome != capability.OutcomeApplied {
		t.Fatalf("resumed displaced reconcile = %#v", resumed)
	}
	if got := string(readFile(t, generatedPath)); got != "new generated summary\n" {
		t.Fatalf("resumed generated content = %q", got)
	}
	if got := string(readFile(t, guardPath)); got != "old generated summary\n" {
		t.Fatalf("recovery guard content = %q", got)
	}
	assertJournalCommitted(t, scope, request.OperationID)
}

// TestPolicyReconcileReportsUndeletableRecoveryToken pins that a recovery pass
// which cannot clean up after itself says so. The pass hard-links the publication
// guard into a ".recover-<uuid>" token to carry it between two rooted views of one
// ticket; if that link cannot be removed it stays in the ticket directory forever,
// and this is the code an operator reads after a crash — so the removal failure is
// joined onto the cause rather than dropped.
func TestPolicyReconcileReportsUndeletableRecoveryToken(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Report undeletable recovery token",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	var interrupted atomic.Bool
	service.afterRootedDisplace = func(target string) error {
		if filepath.Base(target) == "summary.md" &&
			interrupted.CompareAndSwap(false, true) {
			return errors.New("interrupt after guarded displacement")
		}
		return nil
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000039",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview displaced reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	if failed, err := service.PolicyReconcile(
		context.Background(),
		request,
	); err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("displaced reconcile = %#v, %v", failed, err)
	}

	resumeService := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	var sealed atomic.Bool
	resumeService.beforeRecoveryGuardSettle = func(string) error {
		if !sealed.CompareAndSwap(false, true) {
			return nil
		}
		// The token link already exists; sealing its directory against writes
		// makes the cleanup removal fail with EACCES.
		if err := os.Chmod(created.Ticket.Path, 0o500); err != nil {
			return err
		}
		t.Cleanup(func() {
			_ = os.Chmod(created.Ticket.Path, 0o755)
		})
		return errors.New("interrupt before recovery guard settle")
	}
	resumed, err := resumeService.PolicyReconcile(
		context.Background(),
		request,
	)
	if err == nil || resumed.Outcome != capability.OutcomeFailed {
		t.Fatalf("sealed recovery = %#v, %v", resumed, err)
	}
	if !sealed.Load() {
		t.Fatal("recovery guard settle hook did not run")
	}
	if !strings.Contains(
		err.Error(),
		"remove reconciliation recovery token",
	) {
		t.Fatalf("error does not report the leaked token: %v", err)
	}
	if !strings.Contains(
		err.Error(),
		"interrupt before recovery guard settle",
	) {
		t.Fatalf("error dropped the original cause: %v", err)
	}
}

func TestPolicyReconcileRecoveryDoesNotFollowRenamedTicketRoot(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Reject detached recovery",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	var interrupted atomic.Bool
	service.afterRootedDisplace = func(target string) error {
		if filepath.Base(target) == "summary.md" &&
			interrupted.CompareAndSwap(false, true) {
			return errors.New("interrupt after guarded displacement")
		}
		return nil
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000038",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview detached recovery: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	if failed, err := service.PolicyReconcile(
		context.Background(),
		request,
	); err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("interrupt detached recovery = %#v, %v", failed, err)
	}

	detachedRoot := filepath.Join(
		filepath.Dir(scope.TicketsRoot()),
		"detached-"+filepath.Base(created.Ticket.Path),
	)
	replacement := created.Ticket.Manifest
	replacement.ID = "550e8400-e29b-41d4-a716-446655440098"
	resumeService := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	var moved atomic.Bool
	resumeService.beforeRecoveryGuardSettle = func(string) error {
		if !moved.CompareAndSwap(false, true) {
			return nil
		}
		if err := os.Rename(created.Ticket.Path, detachedRoot); err != nil {
			return err
		}
		writeTicketManifest(t, created.Ticket.Layout, replacement)
		if err := os.WriteFile(
			filepath.Join(
				created.Ticket.Path,
				reconcilePublicationGuardName(request.OperationID, 1),
			),
			[]byte("old generated summary\n"),
			0o644,
		); err != nil {
			return err
		}
		entries, err := os.ReadDir(detachedRoot)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.Contains(entry.Name(), ".recover-") {
				continue
			}
			return os.Link(
				filepath.Join(detachedRoot, entry.Name()),
				filepath.Join(created.Ticket.Path, entry.Name()),
			)
		}
		return errors.New("recovery capability token was not found")
	}
	if resumed, err := resumeService.PolicyReconcile(
		context.Background(),
		request,
	); err == nil || resumed.Outcome != capability.OutcomeFailed {
		t.Fatalf("renamed-root recovery = %#v, %v", resumed, err)
	}
	if !moved.Load() {
		t.Fatal("ticket root was not renamed during recovery")
	}
	generatedPath := filepath.Join(detachedRoot, "summary.md")
	if _, err := os.Stat(generatedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery mutated detached ticket target: %v", err)
	}
	replacementTarget := filepath.Join(created.Ticket.Path, "summary.md")
	if _, err := os.Stat(replacementTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery mutated replacement ticket target: %v", err)
	}
	guardPath := filepath.Join(
		detachedRoot,
		reconcilePublicationGuardName(request.OperationID, 1),
	)
	if got := string(readFile(t, guardPath)); got !=
		"old generated summary\n" {
		t.Fatalf("detached recovery guard changed: %q", got)
	}
}

func TestPolicyReconcileDirectoryRepairDoesNotRecreateRenamedTicketRoot(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{}), CreateRequest{
		Scope:     scope,
		Title:     "Reject renamed directory repair",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	artifactsPath := filepath.Join(created.Ticket.Path, "artifacts")
	if err := os.Remove(artifactsPath); err != nil {
		t.Fatalf("remove structural directory: %v", err)
	}
	detachedRoot := filepath.Join(
		filepath.Dir(scope.TicketsRoot()),
		"detached-"+filepath.Base(created.Ticket.Path),
	)
	service := testLifecycleService(t, Options{})
	replacement := created.Ticket.Manifest
	replacement.ID = "550e8400-e29b-41d4-a716-446655440097"
	var moved atomic.Bool
	service.beforeRootedPublish = func(target string) error {
		if filepath.Base(target) != "artifacts" ||
			!moved.CompareAndSwap(false, true) {
			return nil
		}
		if err := os.Rename(created.Ticket.Path, detachedRoot); err != nil {
			return err
		}
		writeTicketManifest(t, created.Ticket.Layout, replacement)
		entries, err := os.ReadDir(detachedRoot)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".artifacts.tmp-") {
				continue
			}
			return os.Mkdir(
				filepath.Join(created.Ticket.Path, entry.Name()),
				0o755,
			)
		}
		return errors.New("directory staging capability was not found")
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000040",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview directory-only reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	applied, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || applied.Outcome != capability.OutcomeFailed {
		t.Fatalf("renamed directory repair = %#v, %v", applied, err)
	}
	if !moved.Load() {
		t.Fatal("ticket root was not renamed during directory repair")
	}
	if got := readTicketManifest(t, created.Ticket.Layout).ID; got !=
		replacement.ID {
		t.Fatalf("replacement ticket identity = %q", got)
	}
	if _, err := os.Stat(
		filepath.Join(created.Ticket.Path, "artifacts"),
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory repair mutated replacement ticket root: %v", err)
	}
	if _, err := os.Stat(
		filepath.Join(detachedRoot, "artifacts"),
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory repair mutated detached ticket root: %v", err)
	}
}

func TestPolicyReconcileDoesNotFollowRenamedTicketRootDuringPublication(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Reject renamed ticket publication",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	detachedRoot := filepath.Join(
		filepath.Dir(scope.TicketsRoot()),
		"detached-"+filepath.Base(created.Ticket.Path),
	)
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	replacement := created.Ticket.Manifest
	replacement.ID = "550e8400-e29b-41d4-a716-446655440096"
	var moved atomic.Bool
	service.beforeRootedPublish = func(target string) error {
		if filepath.Base(target) != "summary.md" ||
			!moved.CompareAndSwap(false, true) {
			return nil
		}
		if err := os.Rename(created.Ticket.Path, detachedRoot); err != nil {
			return err
		}
		writeTicketManifest(t, created.Ticket.Layout, replacement)
		entries, err := os.ReadDir(detachedRoot)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".summary.md.tmp-") {
				continue
			}
			return os.Link(
				filepath.Join(detachedRoot, entry.Name()),
				filepath.Join(created.Ticket.Path, entry.Name()),
			)
		}
		return errors.New("file staging capability was not found")
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000037",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview path-bound reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	applied, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || applied.Outcome != capability.OutcomeFailed {
		t.Fatalf("renamed-root reconcile = %#v, %v", applied, err)
	}
	if !moved.Load() {
		t.Fatal("ticket root was not renamed during publication")
	}
	generatedPath := filepath.Join(detachedRoot, "summary.md")
	if got := string(readFile(t, generatedPath)); got !=
		"old generated summary\n" {
		t.Fatalf("detached ticket root was mutated: %q", got)
	}
	if _, err := os.Stat(
		filepath.Join(created.Ticket.Path, "summary.md"),
	); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement ticket root was mutated: %v", err)
	}
}

func TestPolicyReconcileRequiresCanonicalOperationID(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	_, err := testLifecycleService(t, Options{}).PolicyReconcile(
		context.Background(),
		PolicyReconcileRequest{
			Scope:         scope,
			ActiveProfile: testBuiltinProfile(t),
			Mode:          profile.ConformanceRepairSafe,
			ActorType:     "agent",
			Tool:          "adb-cli",
			OperationID:   "../escape",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("unsafe operation id error = %v", err)
	}
}

func TestPolicyCheckContinuesPastUnreadableSymlinkArtifact(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	first := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Symlink policy ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	second := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Healthy policy sibling",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	contextPath := filepath.Join(first.Ticket.Path, "context.md")
	if err := os.Remove(contextPath); err != nil {
		t.Fatalf("remove context before symlink: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outside, contextPath); err != nil {
		t.Skipf("create artifact symlink: %v", err)
	}

	result, err := service.PolicyCheck(context.Background(), PolicyCheckRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceWarn,
	})
	if err != nil {
		t.Fatalf("policy check symlink artifact: %v", err)
	}
	assertProfileFindingClass(
		t,
		policyTicketByPath(
			t,
			result.Data.Tickets,
			first.Ticket.Path,
		).Report,
		profile.ConformanceContainedPathError,
	)
	if sibling := policyTicketByPath(
		t,
		result.Data.Tickets,
		second.Ticket.Path,
	); !sibling.Report.Conformant {
		t.Fatalf("healthy sibling report = %#v", sibling.Report)
	}
}

func TestPolicyReconcileStaleProfileIsReviewOnly(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	current := reconciliationProfile(t)
	active := current
	active.Version = "v2"
	active.Artifacts[len(active.Artifacts)-1].Template.Version = "v2"
	templates := profile.BuiltinTemplates()
	templates = append(
		templates,
		profile.Template{
			ID: "ticket/summary", Version: "v1",
			SourceScope: "builtin",
			Content:     []byte("old generated summary\n"),
		},
		profile.Template{
			ID: "ticket/summary", Version: "v2",
			SourceScope: "builtin",
			Content:     []byte("new generated summary\n"),
		},
	)
	catalog, err := profile.NewTemplateCatalog(templates)
	if err != nil {
		t.Fatalf("build versioned template catalog: %v", err)
	}
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: catalog,
	}), CreateRequest{
		Scope:     scope,
		Title:     "Stale profile ticket",
		Type:      "feat",
		Profile:   current,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	if err := os.Remove(filepath.Join(created.Ticket.Path, "artifacts")); err != nil {
		t.Fatalf("remove stale-profile structural directory: %v", err)
	}
	service := testLifecycleService(t, Options{
		TemplateResolver: catalog,
		ProfileResolver: ProfileResolverFunc(
			func(reference profile.ProfileReference) (profile.Profile, error) {
				if reference.ID == current.ID &&
					reference.Version == current.Version {
					return current, nil
				}
				return profile.Profile{}, errors.New("profile not found")
			},
		),
	})
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: active,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000029",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview stale-profile reconcile: %v", err)
	}
	ticket := policyTicketByPath(
		t,
		preview.Data.Tickets,
		created.Ticket.Path,
	)
	assertProfileFindingClass(
		t,
		ticket.Report,
		profile.ConformanceStaleProfile,
	)
	for _, effect := range ticket.Plan.Effects {
		if effect.Safe || len(effect.Content) != 0 {
			t.Fatalf("stale-profile effect is auto-repairable: %#v", effect)
		}
	}
	if len(preview.Effects) != 0 {
		t.Fatalf("stale-profile capability effects = %#v", preview.Effects)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	applied, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("apply stale-profile reconcile: %v", err)
	}
	if applied.Outcome != capability.OutcomeUnchanged {
		t.Fatalf("stale-profile apply = %#v", applied)
	}
	renderedFile, err := os.Open(created.Ticket.Layout.RenderManifestPath())
	if err != nil {
		t.Fatalf("open stale-profile render manifest: %v", err)
	}
	rendered, decodeErr := profile.DecodeRenderManifest(renderedFile)
	closeErr := renderedFile.Close()
	if decodeErr != nil || closeErr != nil {
		t.Fatalf("read stale-profile render manifest: %v, %v", decodeErr, closeErr)
	}
	if rendered.Profile.ID != current.ID ||
		rendered.Profile.Version != current.Version {
		t.Fatalf("stale-profile provenance changed: %#v", rendered.Profile)
	}
}

func TestPolicyReconcileRecoversEmptyStructuralFileCreation(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	createProfile := reconciliationProfile(t)
	activeProfile := createProfile
	activeProfile.Artifacts = append(activeProfile.Artifacts, profile.Artifact{
		Role:      "ticket.marker",
		Path:      "marker",
		Kind:      profile.ArtifactFile,
		Authority: profile.AuthorityStructural,
		Required:  true,
		Search:    profile.SearchNone,
		Retention: profile.RetentionDurable,
		Git:       profile.GitTracked,
	})
	if err := profile.ValidateProfile(activeProfile); err != nil {
		t.Fatalf("validate structural recovery profile: %v", err)
	}
	catalog := reconciliationTemplates(t, "generated summary\n")
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: catalog,
	}), CreateRequest{
		Scope:     scope,
		Title:     "Structural recovery ticket",
		Type:      "feat",
		Profile:   createProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	var interrupted atomic.Bool
	service := testLifecycleService(t, Options{
		TemplateResolver: catalog,
		AfterFilesystemStage: func(stage string) error {
			if stage == "create-file" &&
				interrupted.CompareAndSwap(false, true) {
				return errors.New("interrupt structural file creation")
			}
			return nil
		},
	})
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000030",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview structural file reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	failed, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || failed.Outcome != capability.OutcomeFailed {
		t.Fatalf("interrupted structural reconcile = %#v, %v", failed, err)
	}
	markerPath := filepath.Join(created.Ticket.Path, "marker")
	if content := readFile(t, markerPath); len(content) != 0 {
		t.Fatalf("structural marker content = %q", content)
	}
	resumed, err := testLifecycleService(t, Options{
		TemplateResolver: catalog,
	}).PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("resume structural reconcile: %v", err)
	}
	if resumed.Outcome != capability.OutcomeApplied {
		t.Fatalf("resumed structural reconcile = %#v", resumed)
	}
	assertJournalCommitted(t, scope, request.OperationID)
}

func TestPolicyReconcileConcurrentApplyCommitsOnce(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Concurrent reconcile",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000031",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview concurrent reconcile: %v", err)
	}
	request.PlanHash = preview.Data.PlanHash
	request.Apply = true

	type response struct {
		result capability.Result[PolicyReconcileData]
		err    error
	}
	responses := make(chan response, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, applyErr := service.PolicyReconcile(
				context.Background(),
				request,
			)
			responses <- response{result: result, err: applyErr}
		}()
	}
	wait.Wait()
	close(responses)
	outcomes := map[capability.Outcome]int{}
	for response := range responses {
		if response.err != nil {
			t.Fatalf("concurrent reconcile: %v", response.err)
		}
		outcomes[response.result.Outcome]++
	}
	if outcomes[capability.OutcomeApplied] != 1 ||
		outcomes[capability.OutcomeUnchanged] != 1 {
		t.Fatalf("concurrent outcomes = %#v", outcomes)
	}
	assertJournalCommitted(t, scope, request.OperationID)
}

func TestRootedPublicationRejectsParentSwapAfterPreflight(t *testing.T) {
	t.Parallel()

	ticketRootPath := t.TempDir()
	managedPath := filepath.Join(ticketRootPath, "managed")
	if err := os.Mkdir(managedPath, 0o755); err != nil {
		t.Fatalf("create managed directory: %v", err)
	}
	targetPath := filepath.Join(managedPath, "generated.md")
	before := []byte("before\n")
	after := []byte("after\n")
	if err := os.WriteFile(targetPath, before, 0o644); err != nil {
		t.Fatalf("write managed target: %v", err)
	}
	root, err := os.OpenRoot(ticketRootPath)
	if err != nil {
		t.Fatalf("open rooted ticket: %v", err)
	}
	defer root.Close()
	step := reconcileStep{
		journal: journal.Step{
			Ordinal:    1,
			Action:     "replace-generated",
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
		ticketRoot: ticketRootPath,
		ticketID:   "550e8400-e29b-41d4-a716-446655440000",
		relative:   filepath.Join("managed", "generated.md"),
		content:    after,
	}
	roots := &openedReconcileRoots{
		tickets: map[string]*os.Root{ticketRootPath: root},
		parents: map[string]*os.Root{},
	}
	defer roots.Close()
	if err := preflightReconcileSteps(roots, []reconcileStep{step}); err != nil {
		t.Fatalf("preflight rooted publication: %v", err)
	}
	originalManaged := filepath.Join(ticketRootPath, "managed-original")
	if err := os.Rename(managedPath, originalManaged); err != nil {
		t.Fatalf("move managed directory: %v", err)
	}
	outsideRoot := t.TempDir()
	outsideTarget := filepath.Join(outsideRoot, "generated.md")
	if err := os.WriteFile(outsideTarget, before, 0o644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outsideRoot, managedPath); err != nil {
		t.Skipf("replace managed parent with symlink: %v", err)
	}
	parent, _, err := roots.parentForStep(step)
	if err != nil {
		t.Fatalf("read held rooted parent: %v", err)
	}
	if err := roots.validateParentForStep(step, parent); err == nil {
		t.Fatal("rooted publication accepted a swapped symlink parent")
	}
	if got := string(readFile(t, outsideTarget)); got != string(before) {
		t.Fatalf("outside target was modified: %q", got)
	}
}

func TestRootedPublicationPreservesConcurrentTargetBeforeRename(
	t *testing.T,
) {
	t.Parallel()

	ticketRootPath := t.TempDir()
	targetPath := filepath.Join(ticketRootPath, "generated.md")
	before := []byte("before\n")
	after := []byte("after\n")
	concurrent := []byte("concurrent authored change\n")
	if err := os.WriteFile(targetPath, before, 0o644); err != nil {
		t.Fatalf("write managed target: %v", err)
	}
	root, err := os.OpenRoot(ticketRootPath)
	if err != nil {
		t.Fatalf("open rooted ticket: %v", err)
	}
	defer root.Close()
	step := reconcileStep{
		journal: journal.Step{
			Ordinal:    1,
			Action:     "replace-generated",
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
		ticketRoot: ticketRootPath,
		ticketID:   "550e8400-e29b-41d4-a716-446655440000",
		relative:   "generated.md",
		content:    after,
	}
	service := testLifecycleService(t, Options{})
	service.beforeRootedPublish = func(string) error {
		return os.WriteFile(targetPath, concurrent, 0o644)
	}

	err = service.publishRootedFile(
		root,
		"generated.md",
		"550e8400-e29b-41d4-a716-000000000032",
		step,
		true,
	)
	if err == nil {
		t.Fatal("rooted publication overwrote a concurrent target change")
	}
	if got := readFile(t, targetPath); string(got) != string(concurrent) {
		t.Fatalf("concurrent target was clobbered: %q", got)
	}
}

// TestRootedPublicationReportsUndeletableStagingFile pins that a failed
// publication which also cannot remove its staging file reports both. The staging
// file is a durable ".generated.md.tmp-<uuid>" inside the ticket directory, and
// the failure is the only moment anyone will look; the removal error is therefore
// joined onto the cause instead of being dropped.
//
// The join is deliberately one-directional: it never manufactures a failure out of
// a successful publication, because this function also returns nil with the
// staging file still present on the crash-recovery path.
func TestRootedPublicationReportsUndeletableStagingFile(t *testing.T) {
	t.Parallel()

	ticketRootPath := t.TempDir()
	targetPath := filepath.Join(ticketRootPath, "generated.md")
	before := []byte("before\n")
	after := []byte("after\n")
	if err := os.WriteFile(targetPath, before, 0o644); err != nil {
		t.Fatalf("write managed target: %v", err)
	}
	root, err := os.OpenRoot(ticketRootPath)
	if err != nil {
		t.Fatalf("open rooted ticket: %v", err)
	}
	defer root.Close()
	step := reconcileStep{
		journal: journal.Step{
			Ordinal:    1,
			Action:     "replace-generated",
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
		ticketRoot: ticketRootPath,
		ticketID:   "550e8400-e29b-41d4-a716-446655440000",
		relative:   "generated.md",
		content:    after,
	}
	service := testLifecycleService(t, Options{})
	service.beforeRootedPublish = func(string) error {
		// The staging file already exists; sealing its directory against writes
		// makes the deferred removal fail with EACCES.
		if err := os.Chmod(ticketRootPath, 0o500); err != nil {
			return err
		}
		t.Cleanup(func() {
			_ = os.Chmod(ticketRootPath, 0o755)
		})
		return errors.New("interrupt before rooted publish")
	}

	err = service.publishRootedFile(
		root,
		"generated.md",
		"550e8400-e29b-41d4-a716-000000000040",
		step,
		true,
	)
	if err == nil {
		t.Fatal("publication error = nil, want the interrupted publish")
	}
	if !strings.Contains(err.Error(), "interrupt before rooted publish") {
		t.Fatalf("error dropped the original cause: %v", err)
	}
	if !strings.Contains(
		err.Error(),
		"remove reconciliation staging file",
	) {
		t.Fatalf("error does not report the leaked staging file: %v", err)
	}
}

func TestRootedPublicationPreservesLateWritesThroughDisplacedDescriptor(
	t *testing.T,
) {
	t.Parallel()

	ticketRootPath := t.TempDir()
	targetPath := filepath.Join(ticketRootPath, "generated.md")
	before := []byte("before\n")
	after := []byte("after\n")
	late := []byte("late descriptor write\n")
	if err := os.WriteFile(targetPath, before, 0o644); err != nil {
		t.Fatalf("write managed target: %v", err)
	}
	writer, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("open displaced writer: %v", err)
	}
	defer writer.Close()
	root, err := os.OpenRoot(ticketRootPath)
	if err != nil {
		t.Fatalf("open rooted ticket: %v", err)
	}
	defer root.Close()
	operationID := "550e8400-e29b-41d4-a716-000000000034"
	step := reconcileStep{
		journal: journal.Step{
			Ordinal:    1,
			Action:     "replace-generated",
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
		ticketRoot: ticketRootPath,
		ticketID:   "550e8400-e29b-41d4-a716-446655440000",
		relative:   "generated.md",
		content:    after,
	}
	service := testLifecycleService(t, Options{})
	service.beforeGuardRelease = func(string) error {
		if _, err := writer.Write(late); err != nil {
			return err
		}
		return writer.Sync()
	}

	if err := service.publishRootedFile(
		root,
		"generated.md",
		operationID,
		step,
		true,
	); err != nil {
		t.Fatalf("publish with late descriptor write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close displaced writer: %v", err)
	}
	if got := string(readFile(t, targetPath)); got != string(after) {
		t.Fatalf("published target = %q", got)
	}
	guardPath := filepath.Join(
		ticketRootPath,
		reconcilePublicationGuardName(operationID, step.journal.Ordinal),
	)
	if got := string(readFile(t, guardPath)); got != string(before)+string(late) {
		t.Fatalf("late descriptor content was not preserved: %q", got)
	}
}

func TestRootedPublicationDoesNotExposeSwappedTemporarySource(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	targetPath := filepath.Join(rootPath, "generated.md")
	before := []byte("before\n")
	after := []byte("after\n")
	substituted := []byte("substituted temporary content\n")
	if err := os.WriteFile(targetPath, before, 0o644); err != nil {
		t.Fatalf("write managed target: %v", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("open rooted parent: %v", err)
	}
	defer root.Close()
	operationID := "550e8400-e29b-41d4-a716-000000000039"
	step := reconcileStep{
		journal: journal.Step{
			Ordinal:    1,
			Action:     "replace-generated",
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
		ticketRoot: rootPath,
		ticketID:   "550e8400-e29b-41d4-a716-446655440000",
		relative:   "generated.md",
		content:    after,
	}
	service := testLifecycleService(t, Options{})
	service.afterRootedDisplace = func(string) error {
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".generated.md.tmp-") {
				continue
			}
			temporaryPath := filepath.Join(rootPath, entry.Name())
			if err := os.Rename(
				temporaryPath,
				temporaryPath+".expected",
			); err != nil {
				return err
			}
			return os.WriteFile(temporaryPath, substituted, 0o644)
		}
		return errors.New("temporary publication source was not found")
	}

	err = service.publishRootedFile(
		root,
		"generated.md",
		operationID,
		step,
		true,
	)
	if err != nil {
		t.Fatalf("rooted publication with swapped staging source: %v", err)
	}
	if got := string(readFile(t, targetPath)); got != string(after) {
		t.Fatalf("canonical target exposed substituted content: %q", got)
	}
	guardPath := filepath.Join(
		rootPath,
		reconcilePublicationGuardName(operationID, step.journal.Ordinal),
	)
	if got := string(readFile(t, guardPath)); got != string(before) {
		t.Fatalf("recovery guard was not preserved: %q", got)
	}
}

func TestRootedContentFailureCleansOnlyItsCreatedTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		replace    bool
		wantExists bool
		want       string
	}{
		{
			name: "partial target is removed",
		},
		{
			name:       "concurrent replacement is preserved",
			replace:    true,
			wantExists: true,
			want:       "concurrent replacement\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rootPath := t.TempDir()
			root, err := os.OpenRoot(rootPath)
			if err != nil {
				t.Fatalf("open rooted parent: %v", err)
			}
			defer root.Close()
			targetPath := filepath.Join(rootPath, "generated.md")
			err = publishRootedContentExclusiveWithWrite(
				root,
				"generated.md",
				[]byte("desired content\n"),
				journal.Digest([]byte("desired content\n")),
				func(target *os.File, _ []byte) (int, error) {
					written, err := target.Write([]byte("partial"))
					if err != nil {
						return written, err
					}
					if !test.replace {
						return written, errors.New("injected partial write")
					}
					if err := target.Sync(); err != nil {
						return written, err
					}
					if err := os.Rename(
						targetPath,
						targetPath+".partial",
					); err != nil {
						return written, err
					}
					if err := os.WriteFile(
						targetPath,
						[]byte(test.want),
						0o644,
					); err != nil {
						return written, err
					}
					return written, errors.New(
						"injected write failure after replacement",
					)
				},
			)
			if err == nil {
				t.Fatal("expected injected rooted write failure")
			}
			content, readErr := os.ReadFile(targetPath)
			if !test.wantExists {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("partial target remains: %v", readErr)
				}
				return
			}
			if readErr != nil {
				t.Fatalf("read concurrent replacement: %v", readErr)
			}
			if got := string(content); got != test.want {
				t.Fatalf("concurrent replacement = %q", got)
			}
		})
	}
}

func TestRootedPublicationAtomicallyReservesRecoveryGuard(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	targetPath := filepath.Join(rootPath, "generated.md")
	before := []byte("before\n")
	after := []byte("after\n")
	if err := os.WriteFile(targetPath, before, 0o644); err != nil {
		t.Fatalf("write managed target: %v", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("open rooted parent: %v", err)
	}
	defer root.Close()
	operationID := "550e8400-e29b-41d4-a716-000000000035"
	step := reconcileStep{
		journal: journal.Step{
			Ordinal:    1,
			Action:     "replace-generated",
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
		ticketRoot: rootPath,
		ticketID:   "550e8400-e29b-41d4-a716-446655440000",
		relative:   "generated.md",
		content:    after,
	}
	var reservationBlocked bool
	service := testLifecycleService(t, Options{})
	service.beforeGuardMove = func(guardPath string) error {
		file, err := os.OpenFile(
			guardPath,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0o644,
		)
		if errors.Is(err, os.ErrExist) {
			reservationBlocked = true
			return nil
		}
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.WriteString("concurrent guard\n")
		return err
	}

	if err := service.publishRootedFile(
		root,
		"generated.md",
		operationID,
		step,
		true,
	); err != nil {
		t.Fatalf("publish with guard race: %v", err)
	}
	if !reservationBlocked {
		t.Fatal("concurrent guard creation was not atomically rejected")
	}
	if got := string(readFile(t, targetPath)); got != string(after) {
		t.Fatalf("published target = %q", got)
	}
}

func TestSettleRootedPublicationGuardRetainsDistinctConcurrentVersions(
	t *testing.T,
) {
	t.Parallel()

	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("open rooted parent: %v", err)
	}
	defer root.Close()
	before := []byte("before\n")
	after := []byte("after\n")
	concurrent := []byte("concurrent replacement\n")
	guardName := ".aidb-reconcile-guard.previous"
	if err := os.WriteFile(
		filepath.Join(rootPath, guardName),
		before,
		0o644,
	); err != nil {
		t.Fatalf("write recovery guard: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(rootPath, "generated.md"),
		concurrent,
		0o644,
	); err != nil {
		t.Fatalf("write concurrent target: %v", err)
	}
	settled, err := settleRootedPublicationGuard(
		root,
		"generated.md",
		guardName,
		journal.Step{
			Target:     filepath.Join(rootPath, "generated.md"),
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest(after),
		},
	)
	if err == nil || settled {
		t.Fatalf("distinct concurrent versions settled = %t, %v", settled, err)
	}
	if got := string(readFile(t, filepath.Join(rootPath, guardName))); got !=
		string(before) {
		t.Fatalf("recovery guard was discarded: %q", got)
	}
	if got := string(readFile(t, filepath.Join(rootPath, "generated.md"))); got !=
		string(concurrent) {
		t.Fatalf("concurrent target was changed: %q", got)
	}
}

func TestSettleRootedPublicationGuardRejectsUnexpectedMissingGuardContent(
	t *testing.T,
) {
	t.Parallel()

	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("open rooted parent: %v", err)
	}
	defer root.Close()
	guardName := ".aidb-reconcile-guard.unexpected"
	guardPath := filepath.Join(rootPath, guardName)
	targetPath := filepath.Join(rootPath, "generated.md")
	unexpected := []byte("unplanned guard content\n")
	if err := os.WriteFile(guardPath, unexpected, 0o644); err != nil {
		t.Fatalf("write unexpected recovery guard: %v", err)
	}

	settled, err := settleRootedPublicationGuard(
		root,
		"generated.md",
		guardName,
		journal.Step{
			Target:     targetPath,
			BeforeHash: journal.Digest([]byte("before\n")),
			AfterHash:  journal.Digest([]byte("after\n")),
		},
	)
	if err == nil || settled {
		t.Fatalf("unexpected guard settled = %t, %v", settled, err)
	}
	if _, err := os.Stat(targetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected guard created canonical target: %v", err)
	}
	if got := string(readFile(t, guardPath)); got != string(unexpected) {
		t.Fatalf("unexpected guard content changed: %q", got)
	}
}

func TestSettleRootedPublicationGuardRetainsGuardAfterMissingBaseRecovery(
	t *testing.T,
) {
	t.Parallel()

	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatalf("open rooted parent: %v", err)
	}
	defer root.Close()
	before := []byte("before\n")
	guardName := ".aidb-reconcile-guard.previous"
	guardPath := filepath.Join(rootPath, guardName)
	targetPath := filepath.Join(rootPath, "generated.md")
	if err := os.WriteFile(guardPath, before, 0o644); err != nil {
		t.Fatalf("write recovery guard: %v", err)
	}
	settled, err := settleRootedPublicationGuard(
		root,
		"generated.md",
		guardName,
		journal.Step{
			Target:     targetPath,
			BeforeHash: journal.Digest(before),
			AfterHash:  journal.Digest([]byte("after\n")),
		},
	)
	if err != nil || settled {
		t.Fatalf("missing-base recovery settled = %t, %v", settled, err)
	}
	guardInfo, err := os.Stat(guardPath)
	if err != nil {
		t.Fatalf("stat retained recovery guard: %v", err)
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat recovered target: %v", err)
	}
	if os.SameFile(guardInfo, targetInfo) {
		t.Fatal("recovered target unexpectedly reuses the mutable guard inode")
	}
	if got := string(readFile(t, targetPath)); got != string(before) {
		t.Fatalf("recovered target content = %q", got)
	}
	if got := string(readFile(t, guardPath)); got != string(before) {
		t.Fatalf("retained recovery guard content = %q", got)
	}
}

func TestPolicyReconcileRejectsChangedPreviewWithoutPartialRepair(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := reconciliationProfile(t)
	service := testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"new generated summary\n",
		),
	})
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: reconciliationTemplates(
			t,
			"old generated summary\n",
		),
	}), CreateRequest{
		Scope:     scope,
		Title:     "Reject changed preview",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	artifactsPath := filepath.Join(created.Ticket.Path, "artifacts")
	generatedPath := filepath.Join(created.Ticket.Path, "summary.md")
	if err := os.Remove(artifactsPath); err != nil {
		t.Fatalf("remove structural directory: %v", err)
	}
	request := PolicyReconcileRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceRepairSafe,
		ActorType:     "agent",
		Tool:          "adb-cli",
		OperationID:   "550e8400-e29b-41d4-a716-000000000028",
	}
	preview, err := service.PolicyReconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("preview reconcile: %v", err)
	}
	if err := os.WriteFile(
		generatedPath,
		[]byte("unexpected generated customization\n"),
		0o644,
	); err != nil {
		t.Fatalf("change generated target after preview: %v", err)
	}

	request.PlanHash = preview.Data.PlanHash
	request.Apply = true
	result, err := service.PolicyReconcile(context.Background(), request)
	if err == nil || result.Outcome != capability.OutcomeConflict {
		t.Fatalf("changed-preview reconcile = %#v, %v", result, err)
	}
	if _, statErr := os.Stat(artifactsPath); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Fatalf("partial structural repair occurred: %v", statErr)
	}
	if got := string(readFile(t, generatedPath)); got != "unexpected generated customization\n" {
		t.Fatalf("changed generated target was clobbered: %q", got)
	}
}

func TestPolicyCheckReportsMalformedMetadataAndMissingAppendOnlyArtifact(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Malformed policy metadata",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	layout := created.Ticket.Layout
	if err := os.WriteFile(
		layout.RenderManifestPath(),
		[]byte("not: [valid"),
		0o644,
	); err != nil {
		t.Fatalf("corrupt render manifest: %v", err)
	}
	if err := os.WriteFile(
		layout.TombstonesPath(),
		[]byte("schema_version: wrong\n"),
		0o644,
	); err != nil {
		t.Fatalf("corrupt tombstones: %v", err)
	}
	if err := os.Remove(filepath.Join(layout.Root(), "notes.md")); err != nil {
		t.Fatalf("remove append-only artifact: %v", err)
	}

	result, err := service.PolicyCheck(context.Background(), PolicyCheckRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceWarn,
	})
	if err != nil {
		t.Fatalf("policy check malformed metadata: %v", err)
	}
	ticket := policyTicketByPath(t, result.Data.Tickets, layout.Root())
	if !ticket.Report.Blocked {
		t.Fatalf("malformed metadata report = %#v", ticket.Report)
	}
	assertProfileFindingCode(
		t,
		ticket.Report,
		"ticket.render_manifest.invalid",
	)
	assertProfileFindingCode(t, ticket.Report, "ticket.tombstones.invalid")
	assertProfileFindingClass(
		t,
		ticket.Report,
		profile.ConformanceMissingRequired,
	)
}

func TestPolicyCheckClassifiesUnsafeMetadataPathsAsContained(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	created := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Unsafe metadata paths",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	outsideRoot := t.TempDir()
	for _, metadata := range []struct {
		path string
		code string
	}{
		{
			path: created.Ticket.Layout.RenderManifestPath(),
			code: "ticket.render_manifest.invalid",
		},
		{
			path: created.Ticket.Layout.TombstonesPath(),
			code: "ticket.tombstones.invalid",
		},
	} {
		if err := os.Remove(metadata.path); err != nil {
			t.Fatalf("remove metadata target %q: %v", metadata.path, err)
		}
		outside := filepath.Join(outsideRoot, filepath.Base(metadata.path))
		if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
			t.Fatalf("write outside metadata %q: %v", outside, err)
		}
		if err := os.Symlink(outside, metadata.path); err != nil {
			t.Skipf("create metadata symlink %q: %v", metadata.path, err)
		}
	}

	result, err := service.PolicyCheck(context.Background(), PolicyCheckRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceWarn,
	})
	if err != nil {
		t.Fatalf("policy check unsafe metadata: %v", err)
	}
	report := policyTicketByPath(
		t,
		result.Data.Tickets,
		created.Ticket.Path,
	).Report
	for _, code := range []string{
		"ticket.render_manifest.invalid",
		"ticket.tombstones.invalid",
	} {
		assertProfileFindingCodeClass(
			t,
			report,
			code,
			profile.ConformanceContainedPathError,
		)
	}
}

func TestPolicyCheckClassifiesEmbeddedEscapingMetadataPathsAsContained(
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name   string
		code   string
		mutate func(t *testing.T, created MutationData)
	}{
		{
			name: "render manifest",
			code: "ticket.render_manifest.invalid",
			mutate: func(t *testing.T, created MutationData) {
				t.Helper()
				path := created.Ticket.Layout.RenderManifestPath()
				content := readFile(t, path)
				escaped := bytes.Replace(
					content,
					[]byte("path: context.md"),
					[]byte("path: ../outside.md"),
					1,
				)
				if bytes.Equal(content, escaped) {
					t.Fatal("render manifest context path was not found")
				}
				if err := os.WriteFile(path, escaped, 0o644); err != nil {
					t.Fatalf("write escaping render manifest: %v", err)
				}
			},
		},
		{
			name: "render manifest Windows drive path",
			code: "ticket.render_manifest.invalid",
			mutate: func(t *testing.T, created MutationData) {
				t.Helper()
				path := created.Ticket.Layout.RenderManifestPath()
				content := readFile(t, path)
				escaped := bytes.Replace(
					content,
					[]byte("path: context.md"),
					[]byte("path: C:/outside.md"),
					1,
				)
				if bytes.Equal(content, escaped) {
					t.Fatal("render manifest context path was not found")
				}
				if err := os.WriteFile(path, escaped, 0o644); err != nil {
					t.Fatalf("write Windows drive render manifest: %v", err)
				}
			},
		},
		{
			name: "render manifest Windows rooted path",
			code: "ticket.render_manifest.invalid",
			mutate: func(t *testing.T, created MutationData) {
				t.Helper()
				path := created.Ticket.Layout.RenderManifestPath()
				content := readFile(t, path)
				escaped := bytes.Replace(
					content,
					[]byte("path: context.md"),
					[]byte(`path: C:\outside.md`),
					1,
				)
				if bytes.Equal(content, escaped) {
					t.Fatal("render manifest context path was not found")
				}
				if err := os.WriteFile(path, escaped, 0o644); err != nil {
					t.Fatalf("write Windows rooted render manifest: %v", err)
				}
			},
		},
		{
			name: "render manifest Windows parent traversal",
			code: "ticket.render_manifest.invalid",
			mutate: func(t *testing.T, created MutationData) {
				t.Helper()
				path := created.Ticket.Layout.RenderManifestPath()
				content := readFile(t, path)
				escaped := bytes.Replace(
					content,
					[]byte("path: context.md"),
					[]byte(`path: ..\outside.md`),
					1,
				)
				if bytes.Equal(content, escaped) {
					t.Fatal("render manifest context path was not found")
				}
				if err := os.WriteFile(path, escaped, 0o644); err != nil {
					t.Fatalf(
						"write Windows traversal render manifest: %v",
						err,
					)
				}
			},
		},
		{
			name: "tombstones",
			code: "ticket.tombstones.invalid",
			mutate: func(t *testing.T, created MutationData) {
				t.Helper()
				content := []byte(`schema_version: aidb.tombstones/v1
tombstones:
  - actor: human
    reason: retained for review
    scope: ticket:test
    role: ticket.optional
    path: ../outside.md
    removed_at: "2026-09-10T12:00:00Z"
`)
				if err := os.WriteFile(
					created.Ticket.Layout.TombstonesPath(),
					content,
					0o644,
				); err != nil {
					t.Fatalf("write escaping tombstones: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			scope, _ := lifecycleOrganizationScope(t)
			activeProfile := testBuiltinProfile(t)
			service := testLifecycleService(t, Options{})
			created := applyCreate(t, service, CreateRequest{
				Scope:     scope,
				Title:     "Embedded escaping metadata path",
				Type:      "feat",
				Profile:   activeProfile,
				ActorType: "human",
				Tool:      "adb-cli",
			})
			test.mutate(t, created)

			result, err := service.PolicyCheck(
				context.Background(),
				PolicyCheckRequest{
					Scope:         scope,
					ActiveProfile: activeProfile,
					Mode:          profile.ConformanceWarn,
				},
			)
			if err != nil {
				t.Fatalf("policy check escaping metadata: %v", err)
			}
			report := policyTicketByPath(
				t,
				result.Data.Tickets,
				created.Ticket.Path,
			).Report
			assertProfileFindingCodeClass(
				t,
				report,
				test.code,
				profile.ConformanceContainedPathError,
			)
		})
	}
}

func TestPolicyCheckKeepsUnsafeRenderPathWhenFallbackRenderFails(
	t *testing.T,
) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	current := reconciliationProfile(t)
	active := current
	active.Version = "v2"
	active.Artifacts = append([]profile.Artifact(nil), current.Artifacts...)
	for index := range active.Artifacts {
		if active.Artifacts[index].Role == "ticket.summary" {
			reference := *active.Artifacts[index].Template
			reference.Version = "v2"
			active.Artifacts[index].Template = &reference
		}
	}
	createCatalog := reconciliationTemplates(t, "old generated summary\n")
	created := applyCreate(t, testLifecycleService(t, Options{
		TemplateResolver: createCatalog,
	}), CreateRequest{
		Scope:     scope,
		Title:     "Unsafe render path with failed fallback",
		Type:      "feat",
		Profile:   current,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	templates := profile.BuiltinTemplates()
	templates = append(templates, profile.Template{
		ID:          "ticket/summary",
		Version:     "v2",
		SourceScope: "builtin",
		Content:     []byte("new generated summary\n"),
	})
	activeCatalog, err := profile.NewTemplateCatalog(templates)
	if err != nil {
		t.Fatalf("build active-only template catalog: %v", err)
	}
	renderPath := created.Ticket.Layout.RenderManifestPath()
	if err := os.Remove(renderPath); err != nil {
		t.Fatalf("remove render manifest: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "rendered.yaml")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside render manifest: %v", err)
	}
	if err := os.Symlink(outside, renderPath); err != nil {
		t.Skipf("create unsafe render-manifest symlink: %v", err)
	}
	service := testLifecycleService(t, Options{
		TemplateResolver: activeCatalog,
		ProfileResolver: ProfileResolverFunc(
			func(reference profile.ProfileReference) (profile.Profile, error) {
				if reference.ID == current.ID &&
					reference.Version == current.Version {
					return current, nil
				}
				return profile.Profile{}, errors.New("profile not found")
			},
		),
	})

	result, err := service.PolicyCheck(context.Background(), PolicyCheckRequest{
		Scope:         scope,
		ActiveProfile: active,
		Mode:          profile.ConformanceWarn,
	})
	if err != nil {
		t.Fatalf("policy check unsafe failed fallback: %v", err)
	}
	report := policyTicketByPath(
		t,
		result.Data.Tickets,
		created.Ticket.Path,
	).Report
	assertProfileFindingCodeClass(
		t,
		report,
		"ticket.render_manifest.invalid",
		profile.ConformanceContainedPathError,
	)
	assertProfileFindingCodeClass(
		t,
		report,
		"ticket.current_profile.render_failed",
		profile.ConformanceConfigurationError,
	)
}

func TestPolicyCheckReportsCatalogCollisionsPerTicket(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	activeProfile := testBuiltinProfile(t)
	service := testLifecycleService(t, Options{})
	first := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "First collision ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	second := applyCreate(t, service, CreateRequest{
		Scope:     scope,
		Title:     "Second collision ticket",
		Type:      "feat",
		Profile:   activeProfile,
		ActorType: "human",
		Tool:      "adb-cli",
	})
	secondManifest := readTicketManifest(t, second.Ticket.Layout)
	secondManifest.ID = first.Ticket.Manifest.ID
	writeTicketManifest(t, second.Ticket.Layout, secondManifest)

	result, err := service.PolicyCheck(context.Background(), PolicyCheckRequest{
		Scope:         scope,
		ActiveProfile: activeProfile,
		Mode:          profile.ConformanceWarn,
	})
	if err != nil {
		t.Fatalf("policy check catalog collision: %v", err)
	}
	if len(result.Data.Tickets) != 2 {
		t.Fatalf("collision tickets = %#v", result.Data.Tickets)
	}
	for _, ticket := range result.Data.Tickets {
		if !ticket.Report.Blocked {
			t.Fatalf("collision report = %#v", ticket.Report)
		}
		assertProfileFindingCode(
			t,
			ticket.Report,
			"ticket.catalog.collision",
		)
	}
}

func reconciliationProfile(t *testing.T) profile.Profile {
	t.Helper()

	value := testBuiltinProfile(t)
	value.ID = "ticket/reconciliation"
	value.Artifacts = append(value.Artifacts, profile.Artifact{
		Role:      "ticket.summary",
		Path:      "summary.md",
		Kind:      profile.ArtifactFile,
		Authority: profile.AuthorityGenerated,
		Required:  true,
		Search:    profile.SearchLexical,
		Retention: profile.RetentionDurable,
		Git:       profile.GitTracked,
		Template: &profile.TemplateReference{
			ID:      "ticket/summary",
			Version: "v1",
		},
	})
	if err := profile.ValidateProfile(value); err != nil {
		t.Fatalf("validate reconciliation profile: %v", err)
	}
	return value
}

func reconciliationTemplates(
	t *testing.T,
	generated string,
) *profile.TemplateCatalog {
	t.Helper()

	templates := profile.BuiltinTemplates()
	templates = append(templates, profile.Template{
		ID:          "ticket/summary",
		Version:     "v1",
		SourceScope: "builtin",
		Content:     []byte(generated),
	})
	catalog, err := profile.NewTemplateCatalog(templates)
	if err != nil {
		t.Fatalf("build reconciliation templates: %v", err)
	}
	return catalog
}

func policyTicketByPath(
	t *testing.T,
	tickets []PolicyTicket,
	path string,
) PolicyTicket {
	t.Helper()
	for _, ticket := range tickets {
		if filepath.Clean(ticket.Path) == filepath.Clean(path) {
			return ticket
		}
	}
	t.Fatalf("policy ticket %q missing from %#v", path, tickets)
	return PolicyTicket{}
}

func assertProfileFindingClass(
	t *testing.T,
	report profile.ConformanceReport,
	class profile.ConformanceClass,
) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.Class == class {
			return
		}
	}
	t.Fatalf("finding class %q missing from %#v", class, report.Findings)
}

func assertProfileFindingCode(
	t *testing.T,
	report profile.ConformanceReport,
	code string,
) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.Code == code {
			return
		}
	}
	t.Fatalf("finding code %q missing from %#v", code, report.Findings)
}

func assertProfileFindingCodeClass(
	t *testing.T,
	report profile.ConformanceReport,
	code string,
	class profile.ConformanceClass,
) {
	t.Helper()
	for _, finding := range report.Findings {
		if finding.Code == code && finding.Class == class {
			return
		}
	}
	t.Fatalf(
		"finding %q with class %q missing from %#v",
		code,
		class,
		report.Findings,
	)
}

func assertCapabilityEffect(
	t *testing.T,
	effects []capability.Effect,
	action string,
	target string,
) {
	t.Helper()
	for _, effect := range effects {
		if effect.Action == action &&
			filepath.Clean(effect.Target) == filepath.Clean(target) {
			return
		}
	}
	t.Fatalf("effect %s %q missing from %#v", action, target, effects)
}

func renderRecordByRole(
	t *testing.T,
	manifest profile.RenderManifest,
	role string,
) profile.RenderedRecord {
	t.Helper()
	for _, record := range manifest.Artifacts {
		if record.Role == role {
			return record
		}
	}
	t.Fatalf("render record %q missing from %#v", role, manifest.Artifacts)
	return profile.RenderedRecord{}
}

func TestPolicyReconcileRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()

	scope, _ := lifecycleOrganizationScope(t)
	_, err := testLifecycleService(t, Options{}).PolicyReconcile(
		context.Background(),
		PolicyReconcileRequest{
			Scope:         scope,
			ActiveProfile: testBuiltinProfile(t),
			Mode:          profile.ConformanceWarn,
			ActorType:     "agent",
			Tool:          "adb-cli",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "repair-safe") {
		t.Fatalf("unsupported reconcile mode error = %v", err)
	}
}
