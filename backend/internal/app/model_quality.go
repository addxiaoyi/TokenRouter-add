package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
	"github.com/TokenFlux/TokenRouter/internal/modelquality"
	qualityhttp "github.com/TokenFlux/TokenRouter/internal/modelquality/httpapi"
	qualitypostgres "github.com/TokenFlux/TokenRouter/internal/modelquality/postgres"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// model_quality.go binds scored assessments to current administrator permissions
// and the existing provider test execution path. Eligible native text protocols
// carry an explicit output budget and strict completion requirements. Runtime
// provider credentials remain inside application assembly and never enter reports.
//
// Each queued job captures a private configuration fingerprint. Before each
// question, current identity and provider state must still permit that selection;
// changed credentials or routing end the run instead of mixing configurations.
// The worker is stopped before provider transports and storage close. Public
// progress includes only fixed question outcomes, with raw answers discarded
// after grading and detailed execution errors retained by existing diagnostics.

// qualityBackend adapts existing identity and provider services to fixed assessments.
type qualityBackend struct {
	users     *identitypostgres.UserStore     // current account permissions
	providers *providerpostgres.ProviderStore // configured provider records
	tests     *provider.TestService           // native provider test execution
}

// qualitySink retains bounded visible text and explicit completion for one question.
type qualitySink struct {
	text     strings.Builder // collected visible answer text
	complete bool            // explicit successful completion event
	failed   bool            // observed provider error event
}

// qualityModels returns finite configured names supported by scored text execution.
// API-key OpenAI, Anthropic, Gemini, Kimi, DeepSeek, and Zhipu adapters support
// the required output cap and completion checks. Other execution families remain
// available through ordinary provider testing without claiming comparable scores.
func qualityModels(record *provider.Record) []string {
	if record == nil || !record.IsActive() || record.Type != provider.ProviderTypeAPIKey {
		return nil
	}
	if !slices.Contains([]string{provider.PlatformOpenAI, provider.PlatformAnthropic, provider.PlatformGemini, provider.PlatformKimi, provider.PlatformDeepseek, provider.PlatformZhipu}, record.Platform) {
		return nil
	}
	rules := provider.PrepareModelRules(record, provideradapter.ModelDefaults(), provideradapter.ModelRules(record))
	models := []string{}
	for _, name := range rules.ConfiguredModels() {
		lower := strings.ToLower(name)
		media := false
		for _, marker := range []string{"image", "video", "tts", "whisper", "embedding", "rerank", "dall-e", "sora", "flux", "suno", "veo"} {
			if strings.Contains(lower, marker) {
				media = true
				break
			}
		}
		if !media && qualityRoute(record, name).UpstreamProtocol != "" {
			models = append(models, name)
		}
	}
	return models
}

// qualityRoute selects an enabled native text protocol supported by assessment adapters.
// The explicit route prevents standalone probe defaults from selecting Responses
// for a provider configured only for Chat. Model-specific protocol restrictions
// remain enforced by the same snapshot validation used during test execution.
func qualityRoute(record *provider.Record, model string) provider.TestProtocolRoute {
	for _, protocol := range []capability.ProtocolID{capability.ProtocolOpenAIChatCompletions, capability.ProtocolOpenAIResponses, capability.ProtocolAnthropicMessages, capability.ProtocolGeminiGenerateContent} {
		route := provider.TestProtocolRoute{ClientProtocol: protocol, UpstreamProtocol: protocol}
		if route.SupportsProvider(record.RoutingSnapshot(), model) {
			return route
		}
	}
	return provider.TestProtocolRoute{}
}

// Targets lists configured text models without returning provider credentials.
// Pagination includes all active provider records, and filtering uses the same
// eligibility function used again when the administrator submits a selection.
func (b qualityBackend) Targets(ctx context.Context) ([]modelquality.Target, error) {
	targets := []modelquality.Target{}
	for page := 1; ; page++ {
		records, total, err := b.providers.ListWithFilters(ctx, pagination.PaginationParams{Page: page, PageSize: 100}, "", "", "active", "", 0, "")
		if err != nil {
			return nil, err
		}
		for i := range records {
			if models := qualityModels(&records[i]); len(models) > 0 {
				targets = append(targets, modelquality.Target{ID: records[i].ID, Name: records[i].Name, Models: models})
			}
		}
		if total == nil || page >= total.Pages {
			break
		}
	}
	return targets, nil
}

// Fingerprint verifies active administrator authority and hashes private execution settings.
// The hash remains private. Runtime status timestamps are excluded so observations
// cannot invalidate a suite, while credentials, route policy, and proxy selection
// retain their identity across every question in a comparable run.
func (b qualityBackend) Fingerprint(ctx context.Context, operator, providerID int64, model string) (string, error) {
	user, err := b.users.GetByID(ctx, operator)
	if err != nil {
		return "", err
	}
	if user == nil || !user.IsActive() || user.Role != "admin" {
		return "", apperror.Forbidden("QUALITY_ACCESS", "An active administrator is required")
	}
	record, err := b.providers.GetByID(ctx, providerID)
	if err != nil {
		return "", err
	}
	if !slices.Contains(qualityModels(record), model) {
		return "", apperror.BadRequest("QUALITY_MODEL", "Select an enabled API-key text model with complete response support")
	}
	policy := map[string]any{}
	for _, key := range []string{"openai_text_route_mode", "api_protocol", "model_attributes", "header_overrides"} {
		if value, ok := record.Extra[key]; ok {
			policy[key] = value
		}
	}
	raw, err := json.Marshal([]any{record.Platform, record.Type, record.Credentials, policy, record.ProxyID})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// Answer runs a native provider test with strict completion and bounded output.
// The service revalidates the captured fingerprint immediately before this call.
// Raw output is returned only for grading; operational errors expose no answer
// or private upstream diagnostics through the public report projection.
func (b qualityBackend) Answer(ctx context.Context, job modelquality.Job, prompt string) (string, error) {
	record, err := b.providers.GetByID(ctx, job.ProviderID)
	if err != nil {
		return "", err
	}
	route := qualityRoute(record, job.Report.Model)
	if route.UpstreamProtocol == "" {
		return "", errors.New("assessment protocol unavailable")
	}
	ctx = provider.WithTestConstraints(ctx, provider.TestConstraints{OutputTokens: modelquality.MaxOutputTokens, Complete: true})
	kind := provider.ProviderTestTypeText
	sink := &qualitySink{}
	err = b.tests.Test(ctx, provider.TestRequest{ProviderID: job.ProviderID, Model: job.Report.Model, Prompt: prompt, Type: &kind, ProtocolRoute: route, OperatorUserID: &job.OperatorID}, sink)
	if err != nil {
		return "", err
	}
	if sink.failed || !sink.complete {
		return "", errors.New("assessment did not complete")
	}
	// Configuration can change while a question is running. Verify the captured
	// identity again before grading so the report cannot claim a mixed baseline.
	fingerprint, err := b.Fingerprint(ctx, job.OperatorID, job.ProviderID, job.Report.Model)
	if err != nil {
		return "", err
	}
	if fingerprint != job.Fingerprint {
		return "", errors.New("assessment configuration changed")
	}
	return sink.text.String(), nil
}

// Begin accepts test preparation without publishing HTTP response headers.
func (s *qualitySink) Begin(context.Context, bool) error { return nil }

// Emit retains only bounded visible answer text and explicit terminal events.
// Exceeding the answer cap cancels upstream work through TestService's guarded
// sink. Provider status and private diagnostics never enter the retained answer.
func (s *qualitySink) Emit(_ context.Context, event provider.TestEvent) error {
	switch event.Type {
	case "content":
		if s.text.Len()+len(event.Text) > 65536 {
			return errors.New("assessment answer exceeds limit")
		}
		if _, err := s.text.WriteString(event.Text); err != nil {
			return err
		}
	case "error":
		s.failed = true
	case "test_complete":
		s.complete = event.Success
	}
	return nil
}

// provideQualityHTTP assembles persistent assessments and registers their bounded worker.
// Shutdown cancels ongoing questions and waits for final progress publication
// before storage and provider transports close. Startup itself performs no tests.
func provideQualityHTTP(db *sql.DB, users *identitypostgres.UserStore, providers *providerpostgres.ProviderStore, tests *provider.TestService, manager *lifecycle.Manager) *qualityhttp.Handler {
	service := modelquality.NewService(qualitypostgres.NewStore(db), qualityBackend{users: users, providers: providers, tests: tests})
	var cancel context.CancelFunc
	var done chan struct{}
	manager.Register(lifecycle.Hook{Name: "ModelQuality", StartOrder: 981, StopOrder: shutdownStageProducers, Start: func(context.Context) error {
		ctx, stop := context.WithCancel(context.Background())
		cancel = stop
		done = make(chan struct{})
		// Register completion before starting the worker so shutdown observes every
		// question and its final persistence attempt before shared dependencies close.
		go func() { defer close(done); service.Run(ctx) }()
		return nil
	}, Stop: func(ctx context.Context) error {
		if cancel == nil {
			return nil
		}
		cancel()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}})
	return qualityhttp.NewHandler(service)
}
