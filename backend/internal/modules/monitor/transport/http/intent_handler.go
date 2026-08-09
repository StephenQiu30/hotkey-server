package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const maximumIntentRequestBytes = 64 * 1024

type intentHTTPService interface {
	ReadDraft(context.Context, monitorapplication.ReadCurrentIntentDraftQuery) (monitorapplication.ReadCurrentIntentDraftResult, error)
	PutDraft(context.Context, monitorapplication.PutCurrentIntentDraftCommand) (monitorapplication.PutCurrentIntentDraftResult, error)
	ReviewExpansionCandidate(context.Context, monitorapplication.ReviewCurrentExpansionCandidateCommand) (monitorapplication.ReviewExpansionCandidateResult, error)
	SubmitExpansionRun(context.Context, monitorapplication.SubmitCurrentExpansionRunCommand) (monitorapplication.SubmitExpansionRunResult, error)
	SubmitPreviewRun(context.Context, monitorapplication.SubmitCurrentPreviewRunCommand) (monitorapplication.SubmitPreviewRunResult, error)
	ReadExpansionRun(context.Context, monitorapplication.ReadIntentExpansionRunQuery) (monitorapplication.ReadExpansionRunResult, error)
	ReadPreviewRun(context.Context, monitorapplication.ReadIntentPreviewRunQuery) (monitorapplication.ReadPreviewRunResult, error)
}

type IntentHandler struct{ service intentHTTPService }

func NewIntentHandler(service intentHTTPService) *IntentHandler {
	return &IntentHandler{service: service}
}

// GetDraft reads only the intent attached to the current configuration draft.
// An uninitialized intent returns the stable monitor-intent not-found code.
// @Summary Get the current monitor intent draft
// @Tags monitor-intent
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Success 200 {object} MonitorResult[IntentDraftResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft [get]
func (handler *IntentHandler) GetDraft(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, err := intentActorAndMonitor(c)
	if err != nil {
		return err
	}
	result, err := handler.service.ReadDraft(c.Request.Context(), monitorapplication.ReadCurrentIntentDraftQuery{Actor: actor, MonitorID: monitorID})
	if err != nil {
		return err
	}
	setIntentDraftHeaders(c, result.Draft.ResourceVersion)
	httptransport.OK(c, intentDraftResponseDTO(result.Draft))
	return nil
}

// PutDraft atomically initializes or replaces the semantic intent. Creation
// requires If-None-Match: * and expected_resource_version=0; replacement
// requires one exact strong If-Match: "vN" matching the request body.
// @Summary Initialize or replace the current monitor intent draft
// @Tags monitor-intent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param If-Match header string false "strong current resource ETag, required for replacement"
// @Param If-None-Match header string false "*, required for initialization"
// @Param request body ReplaceIntentDraftRequestDTO true "complete semantic intent replacement"
// @Success 200 {object} MonitorResult[IntentDraftResponseDTO]
// @Success 201 {object} MonitorResult[IntentDraftResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/intent [put]
func (handler *IntentHandler) PutDraft(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, err := intentActorAndMonitor(c)
	if err != nil {
		return err
	}
	var request ReplaceIntentDraftRequestDTO
	if err := bindIntentJSON(c, &request); err != nil {
		return err
	}
	if err := requireIntentDraftCondition(c, request.ExpectedResourceVersion); err != nil {
		return err
	}
	objective, clauses, entities, examples := intentDraftRequestCommand(request)
	result, err := handler.service.PutDraft(c.Request.Context(), monitorapplication.PutCurrentIntentDraftCommand{
		Actor: actor, MonitorID: monitorID, ExpectedResourceVersion: request.ExpectedResourceVersion,
		Objective: objective, Clauses: clauses, Entities: entities, Examples: examples,
	})
	if err != nil {
		return err
	}
	setIntentDraftHeaders(c, result.Draft.ResourceVersion)
	response := intentDraftResponseDTO(result.Draft)
	if result.Created {
		httptransport.Created(c, response)
	} else {
		httptransport.OK(c, response)
	}
	return nil
}

// SubmitExpansionRun reserves a version-bound expansion only when a real
// production processor is available. No objective, prompt, or body enters the
// durable job or the 202 response.
// @Summary Submit a monitor intent expansion run
// @Tags monitor-intent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param If-Match header string true "strong current resource ETag"
// @Param Idempotency-Key header string true "bounded idempotency key"
// @Param request body SubmitIntentExpansionRunRequestDTO true "expansion request"
// @Success 202 {object} MonitorResult[IntentRunAcceptedResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/expansion-runs [post]
func (handler *IntentHandler) SubmitExpansionRun(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, err := intentActorAndMonitor(c)
	if err != nil {
		return err
	}
	var request SubmitIntentExpansionRunRequestDTO
	if err := bindIntentJSON(c, &request); err != nil {
		return err
	}
	key, err := requireIntentActionHeaders(c, request.ExpectedResourceVersion)
	if err != nil {
		return err
	}
	result, err := handler.service.SubmitExpansionRun(c.Request.Context(), monitorapplication.SubmitCurrentExpansionRunCommand{
		Actor: actor, MonitorID: monitorID, ExpectedResourceVersion: request.ExpectedResourceVersion,
		IdempotencyKey: key, ExpansionProfile: request.ExpansionProfile,
	})
	if err != nil {
		return err
	}
	response := intentRunAcceptedResponseDTO(result.Run, result.Reused)
	c.Header("Location", response.StatusURL)
	c.Header("Cache-Control", "private, no-store")
	httptransport.Accepted(c, response)
	return nil
}

// GetExpansionRun returns lifecycle state and safe candidate provenance. It
// cannot represent objective text, provider prompt content, or document body.
// @Summary Get a monitor intent expansion run
// @Tags monitor-intent
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param run_id path int true "expansion run ID"
// @Success 200 {object} MonitorResult[IntentExpansionRunStatusResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/expansion-runs/{run_id} [get]
func (handler *IntentHandler) GetExpansionRun(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, runID, err := intentRunPath(c)
	if err != nil {
		return err
	}
	result, err := handler.service.ReadExpansionRun(c.Request.Context(), monitorapplication.ReadIntentExpansionRunQuery{Actor: actor, MonitorID: monitorID, RunID: runID})
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "private, no-store")
	httptransport.OK(c, intentExpansionRunStatusResponseDTO(result.Expansion))
	return nil
}

// ReviewExpansionCandidate applies one admin decision with resource CAS and
// an idempotency receipt. Application re-authorizes durable user state before
// looking up an existing receipt.
// @Summary Review a monitor intent expansion candidate
// @Tags monitor-intent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param candidate_id path string true "expansion candidate ID"
// @Param If-Match header string true "strong current resource ETag"
// @Param Idempotency-Key header string true "bounded idempotency key"
// @Param request body ReviewIntentExpansionCandidateRequestDTO true "candidate decision"
// @Success 200 {object} MonitorResult[IntentDraftResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/expansion-candidates/{candidate_id}/decision [post]
func (handler *IntentHandler) ReviewExpansionCandidate(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, err := intentActorAndMonitor(c)
	if err != nil {
		return err
	}
	candidateID, err := intentCandidateID(c)
	if err != nil {
		return err
	}
	var request ReviewIntentExpansionCandidateRequestDTO
	if err := bindIntentJSON(c, &request); err != nil {
		return err
	}
	key, err := requireIntentActionHeaders(c, request.ExpectedResourceVersion)
	if err != nil {
		return err
	}
	result, err := handler.service.ReviewExpansionCandidate(c.Request.Context(), monitorapplication.ReviewCurrentExpansionCandidateCommand{
		Actor: actor, MonitorID: monitorID, CandidateID: candidateID,
		ExpectedResourceVersion: request.ExpectedResourceVersion, Decision: request.Decision,
		Note: request.Note, IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	setIntentDraftHeaders(c, result.Draft.ResourceVersion)
	httptransport.OK(c, intentDraftResponseDTO(result.Draft))
	return nil
}

// SubmitPreviewRun reserves a version-bound preview only when a real preview
// evaluator is production-ready.
// @Summary Submit a monitor intent preview run
// @Tags monitor-intent
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param If-Match header string true "strong current resource ETag"
// @Param Idempotency-Key header string true "bounded idempotency key"
// @Param request body SubmitIntentPreviewRunRequestDTO true "preview request"
// @Success 202 {object} MonitorResult[IntentRunAcceptedResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 409 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/preview-runs [post]
func (handler *IntentHandler) SubmitPreviewRun(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, err := intentActorAndMonitor(c)
	if err != nil {
		return err
	}
	var request SubmitIntentPreviewRunRequestDTO
	if err := bindIntentJSON(c, &request); err != nil {
		return err
	}
	key, err := requireIntentActionHeaders(c, request.ExpectedResourceVersion)
	if err != nil {
		return err
	}
	result, err := handler.service.SubmitPreviewRun(c.Request.Context(), monitorapplication.SubmitCurrentPreviewRunCommand{
		Actor: actor, MonitorID: monitorID, ExpectedResourceVersion: request.ExpectedResourceVersion,
		IdempotencyKey: key, EvaluatorProfile: request.EvaluatorProfile, SampleLimit: request.SampleLimit,
	})
	if err != nil {
		return err
	}
	response := intentRunAcceptedResponseDTO(result.Run, result.Reused)
	c.Header("Location", response.StatusURL)
	c.Header("Cache-Control", "private, no-store")
	httptransport.Accepted(c, response)
	return nil
}

// GetPreviewRun returns lifecycle state and bounded explainability samples. It
// never includes the recalled document body or any model prompt.
// @Summary Get a monitor intent preview run
// @Tags monitor-intent
// @Produce json
// @Security BearerAuth
// @Param id path int true "monitor ID"
// @Param run_id path int true "preview run ID"
// @Success 200 {object} MonitorResult[IntentPreviewRunStatusResponseDTO]
// @Failure 400 {object} MonitorResult[EmptyResponse]
// @Failure 401 {object} MonitorResult[EmptyResponse]
// @Failure 403 {object} MonitorResult[EmptyResponse]
// @Failure 404 {object} MonitorResult[EmptyResponse]
// @Failure 503 {object} MonitorResult[EmptyResponse]
// @Router /api/v1/monitors/{id}/draft/preview-runs/{run_id} [get]
func (handler *IntentHandler) GetPreviewRun(c *gin.Context) error {
	httptransport.SetModule(c, "monitor")
	actor, monitorID, runID, err := intentRunPath(c)
	if err != nil {
		return err
	}
	result, err := handler.service.ReadPreviewRun(c.Request.Context(), monitorapplication.ReadIntentPreviewRunQuery{Actor: actor, MonitorID: monitorID, RunID: runID})
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "private, no-store")
	httptransport.OK(c, intentPreviewRunStatusResponseDTO(result.Preview))
	return nil
}

func bindIntentJSON(c *gin.Context, destination any) error {
	if c == nil || c.Request == nil || destination == nil {
		return invalidRequest(fmt.Errorf("intent request is invalid"))
	}
	c.Request.Body = stdhttp.MaxBytesReader(c.Writer, c.Request.Body, maximumIntentRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalidRequest(err)
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return invalidRequest(fmt.Errorf("intent request contains trailing data"))
	}
	if err := binding.Validator.ValidateStruct(destination); err != nil {
		return invalidRequest(err)
	}
	return nil
}

func intentActorAndMonitor(c *gin.Context) (monitorapplication.IntentActorDTO, int64, error) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return monitorapplication.IntentActorDTO{}, 0, sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	}
	id, err := monitorID(c)
	if err != nil {
		return monitorapplication.IntentActorDTO{}, 0, err
	}
	return monitorapplication.IntentActorDTO{UserID: subject.UserID}, id, nil
}

func intentRunPath(c *gin.Context) (monitorapplication.IntentActorDTO, int64, int64, error) {
	actor, monitorID, err := intentActorAndMonitor(c)
	if err != nil {
		return monitorapplication.IntentActorDTO{}, 0, 0, err
	}
	runID, err := positivePathID(c, "run_id")
	if err != nil {
		return monitorapplication.IntentActorDTO{}, 0, 0, err
	}
	return actor, monitorID, runID, nil
}

func intentCandidateID(c *gin.Context) (string, error) {
	value := c.Param("candidate_id")
	if value == "" || value != strings.TrimSpace(value) || len([]byte(value)) > 128 || strings.ContainsAny(value, "\x00\r\n") {
		return "", invalidRequest(fmt.Errorf("invalid candidate_id"))
	}
	return value, nil
}

func requireIntentDraftCondition(c *gin.Context, expected int64) error {
	if expected < 0 {
		return invalidRequest(fmt.Errorf("expected_resource_version is invalid"))
	}
	ifMatch := c.Request.Header.Values("If-Match")
	ifNoneMatch := c.Request.Header.Values("If-None-Match")
	if expected == 0 {
		if len(ifMatch) != 0 || len(ifNoneMatch) != 1 || ifNoneMatch[0] != "*" {
			return invalidRequest(fmt.Errorf("initialization requires If-None-Match: *"))
		}
		return nil
	}
	if len(ifNoneMatch) != 0 || len(ifMatch) != 1 {
		return invalidRequest(fmt.Errorf("replacement requires one strong If-Match"))
	}
	version, err := parseIntentResourceETag(ifMatch[0])
	if err != nil || version != expected {
		return invalidRequest(fmt.Errorf("If-Match and expected_resource_version must match"))
	}
	return nil
}

func requireIntentActionHeaders(c *gin.Context, expected int64) (string, error) {
	if expected <= 0 || len(c.Request.Header.Values("If-None-Match")) != 0 {
		return "", invalidRequest(fmt.Errorf("positive expected_resource_version and If-Match are required"))
	}
	values := c.Request.Header.Values("If-Match")
	if len(values) != 1 {
		return "", invalidRequest(fmt.Errorf("one strong If-Match is required"))
	}
	version, err := parseIntentResourceETag(values[0])
	if err != nil || version != expected {
		return "", invalidRequest(fmt.Errorf("If-Match and expected_resource_version must match"))
	}
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !validIntentIdempotencyHeader(keys[0]) {
		return "", invalidRequest(fmt.Errorf("one valid Idempotency-Key is required"))
	}
	return keys[0], nil
}

func parseIntentResourceETag(value string) (int64, error) {
	if len(value) < 4 || value[0] != '"' || value[1] != 'v' || value[len(value)-1] != '"' {
		return 0, fmt.Errorf("resource ETag must be strong")
	}
	digits := value[2 : len(value)-1]
	if digits == "" || digits[0] == '0' {
		return 0, fmt.Errorf("resource ETag version is invalid")
	}
	for _, character := range digits {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("resource ETag version is invalid")
		}
	}
	version, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("resource ETag version is invalid")
	}
	return version, nil
}

func validIntentIdempotencyHeader(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len([]byte(value)) <= 128 && !strings.ContainsAny(value, "\x00\r\n")
}

func setIntentDraftHeaders(c *gin.Context, version int64) {
	c.Header("ETag", fmt.Sprintf(`"v%d"`, version))
	c.Header("Cache-Control", "private, no-store")
}
