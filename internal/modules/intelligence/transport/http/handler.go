package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type modelProfileService interface {
	List(context.Context) ([]intelligencedomain.ModelProfile, error)
	Get(context.Context, int64) (intelligencedomain.ModelProfile, error)
	Create(context.Context, intelligencedomain.ModelProfile) (intelligencedomain.ModelProfile, error)
	Update(context.Context, intelligencedomain.ModelProfile, int64) (intelligencedomain.ModelProfile, error)
	SoftDelete(context.Context, int64, int64) (intelligencedomain.ModelProfile, error)
	Restore(context.Context, int64, int64) (intelligencedomain.ModelProfile, error)
}

// Compile-time assertion keeps the application control plane compatible with
// its narrow HTTP boundary without exposing the PostgreSQL repository.
var _ modelProfileService = (*intelligenceapplication.ModelProfileService)(nil)

type Handler struct{ service modelProfileService }

func NewHandler(service modelProfileService) *Handler { return &Handler{service: service} }

// List returns safe profile metadata, including archived state, for admins.
// @Summary List AI model profiles
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ModelProfileResult[ModelProfileListResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/model-profiles [get]
func (handler *Handler) List(c *gin.Context) error {
	httptransport.SetModule(c, "intelligence")
	profiles, err := handler.service.List(c.Request.Context())
	if err != nil {
		return modelProfileError(err)
	}
	httptransport.OK(c, modelProfileListResponse(profiles))
	return nil
}

// Get returns one safe profile projection, never its credential reference.
// @Summary Get an AI model profile
// @Tags ai
// @Produce json
// @Security BearerAuth
// @Param id path int true "model profile ID"
// @Success 200 {object} ModelProfileResult[ModelProfileResponse]
// @Failure 400 {object} ModelProfileResult[EmptyResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/model-profiles/{id} [get]
func (handler *Handler) Get(c *gin.Context) error {
	httptransport.SetModule(c, "intelligence")
	profile, err := handler.profile(c)
	if err != nil {
		return err
	}
	httptransport.OK(c, modelProfileResponse(profile))
	return nil
}

// Create creates a profile with its only write-only credential reference.
// @Summary Create an AI model profile
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateModelProfileRequest true "model profile"
// @Success 201 {object} ModelProfileResult[ModelProfileResponse]
// @Failure 400 {object} ModelProfileResult[EmptyResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/model-profiles [post]
func (handler *Handler) Create(c *gin.Context) error {
	httptransport.SetModule(c, "intelligence")
	var request CreateModelProfileRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		return invalidModelProfileRequest(err)
	}
	profile, err := handler.service.Create(c.Request.Context(), createModelProfile(request))
	if err != nil {
		return modelProfileError(err)
	}
	httptransport.Created(c, modelProfileResponse(profile))
	return nil
}

// Update changes only operational settings under profile version control.
// Immutable fields are rejected with 70000 even when supplied as JSON null.
// @Summary Update AI model profile operational settings
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "model profile ID"
// @Param request body UpdateModelProfileRequest true "operational profile update"
// @Success 200 {object} ModelProfileResult[ModelProfileResponse]
// @Failure 400 {object} ModelProfileResult[EmptyResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 409 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/model-profiles/{id} [patch]
func (handler *Handler) Update(c *gin.Context) error {
	httptransport.SetModule(c, "intelligence")
	id, err := modelProfileID(c)
	if err != nil {
		return err
	}
	request, dailyBudgetSet, err := decodeModelProfileUpdate(c)
	if err != nil {
		return err
	}
	if request.Version <= 0 || !hasProfileOperation(request, dailyBudgetSet) {
		return invalidAIProfile()
	}
	profile, err := handler.service.Get(c.Request.Context(), id)
	if err != nil {
		return modelProfileError(err)
	}
	if profile.Version != request.Version {
		return profileVersionConflict()
	}
	if request.TimeoutSeconds != nil {
		profile.TimeoutSeconds = *request.TimeoutSeconds
	}
	if request.MaxAttempts != nil {
		profile.MaxAttempts = *request.MaxAttempts
	}
	if request.MaxCost != nil {
		profile.MaxCost = *request.MaxCost
	}
	if dailyBudgetSet {
		profile.DailyBudget = request.DailyBudget
	}
	if request.FallbackPriority != nil {
		profile.FallbackPriority = *request.FallbackPriority
	}
	if request.Enabled != nil {
		profile.Enabled = *request.Enabled
	}
	updated, err := handler.service.Update(c.Request.Context(), profile, request.Version)
	if err != nil {
		return modelProfileError(err)
	}
	httptransport.OK(c, modelProfileResponse(updated))
	return nil
}

// Delete soft-deletes a profile without destroying run or vector provenance.
// @Summary Archive an AI model profile
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "model profile ID"
// @Param request body ModelProfileVersionRequest true "expected profile version"
// @Success 200 {object} ModelProfileResult[ModelProfileResponse]
// @Failure 400 {object} ModelProfileResult[EmptyResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 409 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/model-profiles/{id} [delete]
func (handler *Handler) Delete(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.SoftDelete)
}

// Restore returns a soft-deleted profile to its prior enabled setting.
// @Summary Restore an AI model profile
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "model profile ID"
// @Param request body ModelProfileVersionRequest true "expected profile version"
// @Success 200 {object} ModelProfileResult[ModelProfileResponse]
// @Failure 400 {object} ModelProfileResult[EmptyResponse]
// @Failure 401 {object} ModelProfileResult[EmptyResponse]
// @Failure 403 {object} ModelProfileResult[EmptyResponse]
// @Failure 409 {object} ModelProfileResult[EmptyResponse]
// @Failure 503 {object} ModelProfileResult[EmptyResponse]
// @Router /api/v1/ai/model-profiles/{id}/restore [post]
func (handler *Handler) Restore(c *gin.Context) error {
	return handler.lifecycle(c, handler.service.Restore)
}

func (handler *Handler) lifecycle(c *gin.Context, operation func(context.Context, int64, int64) (intelligencedomain.ModelProfile, error)) error {
	httptransport.SetModule(c, "intelligence")
	id, err := modelProfileID(c)
	if err != nil {
		return err
	}
	var request ModelProfileVersionRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		return invalidModelProfileRequest(err)
	}
	if request.Version <= 0 {
		return invalidAIProfile()
	}
	profile, err := operation(c.Request.Context(), id, request.Version)
	if err != nil {
		return modelProfileError(err)
	}
	httptransport.OK(c, modelProfileResponse(profile))
	return nil
}

func (handler *Handler) profile(c *gin.Context) (intelligencedomain.ModelProfile, error) {
	id, err := modelProfileID(c)
	if err != nil {
		return intelligencedomain.ModelProfile{}, err
	}
	profile, err := handler.service.Get(c.Request.Context(), id)
	if err != nil {
		return intelligencedomain.ModelProfile{}, modelProfileError(err)
	}
	return profile, nil
}

func modelProfileID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidModelProfileRequest(fmt.Errorf("invalid model profile id"))
	}
	return id, nil
}

func decodeModelProfileUpdate(c *gin.Context) (UpdateModelProfileRequest, bool, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return UpdateModelProfileRequest{}, false, invalidModelProfileRequest(err)
	}
	var fields map[string]json.RawMessage
	if err := decodeOneJSON(body, &fields, false); err != nil || fields == nil {
		if err == nil {
			err = fmt.Errorf("request must be an object")
		}
		return UpdateModelProfileRequest{}, false, invalidModelProfileRequest(err)
	}
	for _, field := range []string{"task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions"} {
		if _, found := fields[field]; found {
			return UpdateModelProfileRequest{}, false, invalidAIProfile()
		}
	}
	var request UpdateModelProfileRequest
	if err := decodeOneJSON(body, &request, true); err != nil {
		return UpdateModelProfileRequest{}, false, invalidModelProfileRequest(err)
	}
	_, dailyBudgetSet := fields["daily_budget"]
	return request, dailyBudgetSet, nil
}

func decodeStrictJSON(c *gin.Context, target any) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	return decodeOneJSON(body, target, true)
}

func decodeOneJSON(body []byte, target any, strict bool) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func hasProfileOperation(request UpdateModelProfileRequest, dailyBudgetSet bool) bool {
	return request.TimeoutSeconds != nil || request.MaxAttempts != nil || request.MaxCost != nil || dailyBudgetSet || request.FallbackPriority != nil || request.Enabled != nil
}

func invalidModelProfileRequest(cause error) error {
	return sharederrors.Wrap(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "", cause)
}

func invalidAIProfile() error {
	return sharederrors.New(sharederrors.CodeAIModelProfileInvalid, stdhttp.StatusBadRequest, "")
}

func profileVersionConflict() error {
	return sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
}

func modelProfileError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	if code, ok := intelligencedomain.CodeOf(err); ok {
		if definition, found := sharederrors.Lookup(code); found {
			return sharederrors.New(code, definition.HTTPStatus, "")
		}
	}
	return err
}
